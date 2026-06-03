# Pointing a Web3 agent at a real MPC cluster

The `Web3Agent` ships with an optional integration to a `hanzoai/mpc`
(or `luxfi/mpc`) threshold-signing cluster via an HTTP bridge. When
enabled, payment-confirmation signatures are cross-verified against
the cluster before the payment is recorded — so a single malicious
party cannot spoof an ack.

## Enable

```python
from hanzo_agents.extensions.web3 import Web3Agent, Web3AgentConfig
from hanzo_agents.extensions.web3.mpc_client import MpcConfig

agent = Web3Agent(
    name="paid-worker",
    description="Does paid work, verifies via MPC",
    web3_config=Web3AgentConfig(
        wallet_enabled=True,
        wallet_config=...,                        # your WalletConfig
        mpc_enabled=True,
        mpc_config=MpcConfig(
            endpoint="https://mpc.example.com",   # HTTP bridge
            timeout_seconds=10.0,
            max_retries=3,
            parties=3,
            threshold=2,
            api_key="...",                        # if cluster requires bearer auth
        ),
        payment_min_confirmations=1,              # raise to 12+ for mainnet
    ),
)
```

`mpc_enabled=False` (the default) keeps the legacy chain-only verification
path. `mpc_config=None` with `mpc_enabled=True` falls back to a
`localhost:8080` bridge — useful for development.

## Wire protocol the bridge must speak

The client expects the HTTP bridge documented in `hanzoai/mpc`
`docs/BRIDGE-INTEGRATION.md`:

| Verb · path           | Request                                                | Response                                       |
|-----------------------|--------------------------------------------------------|------------------------------------------------|
| `POST /v1/keygen`     | `{"parties": n, "threshold": t}`                       | `{"public_key": "<hex>", "key_id": "<id>"}`    |
| `POST /v1/sign`       | `{"key_id": "<id>", "digest": "<hex>"}`                | `{"signature": "<hex>", "scheme": "ecdsa"}`    |
| `POST /v1/verify`     | `{"signature": "<hex>", "digest": "<hex>", "public_key": "<hex>"}` | `{"valid": true|false}`            |

## Failure modes

The client retries 5xx and network failures with exponential backoff
(`backoff_seconds * 2^attempt`), respecting `max_retries`. 4xx responses
are not retried — a malformed request will not be re-sent.

`Web3Agent.verify_payment` is conservative on MPC outage: if the bridge
is unreachable but the chain-side checks pass (status, value, recipient,
confirmations), the payment is still credited. Adjust by overriding
`verify_payment` in a subclass if your threat model requires the MPC
verify to be load-bearing.

## Running the test suite

```
pytest sdk/python/tests/test_web3_mpc_client.py
```

The tests mock `aiohttp.ClientSession`, so no live bridge is needed.
