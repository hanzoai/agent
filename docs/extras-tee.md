# TEE extra

```bash
pip install "hanzo-agent[tee]"
```

The extra installs `cryptography` (`>=44,<50`). `agents.extensions.tee` does not import it yet, and the module imports without the extra.

## What it gives you

`agents.extensions.tee` is an interface for running a task in a trusted execution environment and carrying the attestation that came with the result. The only executor that ships is a mock. A real provider is an executor you write.

| Name | What it is |
|---|---|
| `TEEProvider` | `INTEL_SGX` (`"sgx"`), `AMD_SEV` (`"sev"`), `NVIDIA_H100` (`"h100"`), `MOCK` (`"mock"`) |
| `TEEConfig` | `provider` (default `MOCK`), `max_memory_mb` (4096), `max_execution_time_s` (300), `enable_network` (`False`), `allowed_endpoints`, `attestation_server` |
| `AttestationReport` | `provider`, `enclave_id`, `code_hash`, `timestamp`, `quote`, `signature`, `metadata`; `to_dict()` and `verify(expected_code_hash=None)` |
| `ConfidentialAgent` | wraps an agent with a config and an executor |
| `ComputeOffer`, `ComputeRequest`, `ComputeMarketplace` | an in-memory book that matches compute offers to requests |

`TEEExecutor` (the abstract executor) and `MockTEEExecutor` live in `agents.extensions.tee.tee` and are not re-exported from the package.

## Run a task and check its attestation

```python
import hashlib

from agents import Agent
from agents.extensions.tee import ConfidentialAgent, TEEConfig, TEEProvider
from agents.extensions.tee.tee import MockTEEExecutor

code = "result = {'total': sum(inputs['xs'])}"
expected = hashlib.sha256(code.encode()).hexdigest()

agent = ConfidentialAgent(
    Agent(name="analyst"),
    tee_config=TEEConfig(provider=TEEProvider.MOCK),
    tee_executor=MockTEEExecutor(),
)

out = agent.execute_confidential(code, {"xs": [1, 2, 3]}, verify_code_hash=expected)
print(out["success"], out["result"])        # True {'total': 6}
print(agent.verify_computation(out, expected))     # True
print(agent.verify_computation(out, "0" * 64))     # False

out = agent.execute_confidential(code, {"xs": [1, 2, 3]}, verify_code_hash="0" * 64)
print(out["success"], out["warning"])       # True Code hash mismatch
```

- The task is a string of Python. The executor runs it with `inputs` in scope and reads `result` back.
- The result is a dict: `success`, then `result` or `error`, plus the `attestation` as a dict. A task that raises comes back as `success: False` with `error`; it does not raise.
- `code_hash` in the attestation is the SHA-256 of the task string.
- A `verify_code_hash` that does not match adds `warning: "Code hash mismatch"`. The result is still returned.
- `get_attestation_history()` returns the attestations of the successful runs.

## What `verify` checks

`AttestationReport.verify(expected_code_hash)` returns `False` when a hash is given and differs from `code_hash`, and otherwise `True` when `signature` is not empty. It does not check `quote` or `signature` against a vendor root of trust. A `True` from it says the report is well formed and names the code you expected; it is not evidence that hardware produced it.

A real provider overrides `TEEExecutor.verify_remote_attestation` with that check.

## Use a real enclave

Subclass `TEEExecutor` and pass it as `tee_executor`:

- `execute(code, inputs, config)` runs the task in the enclave and returns the dict shown above, including `attestation`.
- `get_attestation()` returns an `AttestationReport` with the provider's quote and signature.
- `verify_remote_attestation(report, expected_code_hash)` validates a report from another party.

`MockTEEExecutor` runs the task with Python's `exec` inside your own process: no isolation, no limits from `TEEConfig`. Use it for tests, and never give it code you did not write.

## Compute marketplace

`ComputeMarketplace` pairs enclave operators with people who need compute. A request matches an offer when the offer's `price_per_second` is at most the request's `max_price_per_second`, the offer's `max_duration` covers `required_duration`, and the request's `required_provider` (if any) is in the offer's `supported_providers`. A match removes both from the book and records the total price.

```python
from agents.extensions.tee import (
    ComputeMarketplace, ComputeOffer, ComputeRequest, TEEProvider,
)

market = ComputeMarketplace()
market.post_offer(ComputeOffer(
    provider_address="0xprovider",
    provider_enclave_id="enclave-1",
    price_per_second=0.0001,
    max_duration=600,
    supported_providers=[TEEProvider.AMD_SEV, TEEProvider.NVIDIA_H100],
))
market.post_request(ComputeRequest(
    requester_address="0xrequester",
    code_hash="ab" * 32,
    max_price_per_second=0.0002,
    required_duration=120,
    required_provider=TEEProvider.NVIDIA_H100,
))

(match,) = market.get_matches()
print(match["provider"], match["requester"], match["total_price"])  # 0xprovider 0xrequester 0.012
```

The book lives in the process. Nothing is paid and nothing is sent anywhere.

## Known gaps

- `create_attestation_verifier_tool()` raises `ModuleNotFoundError: No module named 'agents.extensions.tool'`. Its import is one package level short, and `agents.tool.Tool` is a type alias, not a class to subclass.
- `ConfidentialAgent` rebuilds attestations from the result dict, so entries in its history hold `provider` as the string `"mock"` and `quote` as a hex string, not the enum and bytes.
