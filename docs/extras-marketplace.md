# Marketplace extra

```bash
pip install "hanzo-agent[marketplace]"
```

The extra installs `aiohttp` (`>=3.11.11,<4`) and `redis` (`>=5,<6`). `agents.extensions.marketplace` uses neither. It is an in-memory order book that lives in one process: there is no registry service behind it, no Redis, and no network call. Two agents see each other's offers only if they post to the same `AgentMarketplace` object.

## Model

An agent posts an **offer** (a service it sells, with a price in ETH) or a **request** (a service it wants, with a ceiling price). When an offer and a request fit, they become a **match** and leave the book.

| Name | What it is |
|---|---|
| `ServiceType` | `DATA`, `COMPUTE`, `INFERENCE`, `ANALYSIS`, `CODING`, `RESEARCH`, `CUSTOM` |
| `ServiceOffer` | `id`, `agent_address`, `agent_name`, `service_type`, `description`, `price_eth`, `min_reputation`, `max_duration_hours` (24), `requires_tee`, `metadata`, `expires_at` |
| `ServiceRequest` | `id`, `requester_address`, `requester_name`, `service_type`, `description`, `max_price_eth`, `duration_hours` (1), `min_reputation`, `requires_tee`, `metadata`, `expires_at` |
| `ServiceMatch` | `match_id`, `offer`, `request`, `agreed_price_eth`, `status`, `result`, `attestation`, `escrow_tx` |
| `AgentMarketplace` | the book |

`post_offer(agent, service_type, description, price_eth, **options)` and `post_request(agent, service_type, description, max_price_eth, **options)` take any object with `.name` and `.address` as `agent` (the code types it as `Web3Agent`; an agent with no address is filed as `"mock_address"`). The options are the optional fields above, by name. Both return the new id and try to match straight away.

## How a match is made

An offer fits a request when all of these hold:

- neither has expired (`expires_at` is a Unix time, or `None`);
- the service types are equal, or the request is `CUSTOM`, which fits any offer;
- `price_eth <= max_price_eth`;
- the request's `duration_hours <= max_duration_hours` of the offer;
- the offer has `requires_tee` if the request does;
- the provider's reputation is at least the request's `min_reputation`, and the requester's is at least the offer's.

When several offers fit, the cheapest wins and the agreed price is that offer's price. The marketplace prints `Match created: ...` when it makes one.

## Reputation

Every address starts at `0.5`. `complete_match(match_id, result, attestation=None)` raises the provider by `0.1`. `dispute_match(match_id, reason)` lowers the provider by `0.05`. Scores stay between 0 and 1. A reputation gate above `0.5` therefore keeps out an address that has no history.

## Example

```python
from agents.extensions.marketplace import AgentMarketplace, ServiceType

class Agent:  # anything with .name and .address
    def __init__(self, name, address):
        self.name, self.address = name, address

market = AgentMarketplace()
coder = Agent("coder", "0xcoder")
cheap = Agent("cheap-coder", "0xcheap")
buyer = Agent("buyer", "0xbuyer")

market.post_offer(coder, ServiceType.CODING, "Refactor a module", price_eth=0.05, max_duration_hours=8)
market.post_offer(cheap, ServiceType.CODING, "Refactor a module", price_eth=0.03, max_duration_hours=8)
market.post_request(buyer, ServiceType.CODING, "Refactor my parser", max_price_eth=0.10, duration_hours=2)

(match,) = market.get_matches_for_agent("0xbuyer")
print(match.offer.agent_name, match.agreed_price_eth, match.status)   # cheap-coder 0.03 pending
print(len(market.get_active_offers()), len(market.get_active_requests()))  # 1 0

market.complete_match(match.match_id, {"patch": "..."})
print(match.status, market.get_reputation("0xcheap"))                  # completed 0.6
print(market.get_stats()["total_volume_eth"])                          # 0.03
```

`get_active_offers(service_type=None)` lists offers cheapest first; `get_active_requests(service_type=None)` lists requests by highest ceiling first. The module also creates one shared instance, `marketplace`.

## What it does not do

- No money moves. `price_eth` is bookkeeping, and `escrow_tx` stays `None`.
- Nothing persists. A restart empties the book.
- `attestation` on a match is whatever dict you pass; nothing checks it. To produce one, see the [TEE extra](extras-tee.md).

## Known gaps

`import agents.extensions.marketplace` fails today with `ModuleNotFoundError: No module named 'agents.extensions.marketplace.web3_agent'`. `marketplace.py` imports `Web3Agent` from a sibling module, but the class lives in `agents.extensions.web3.web3_agent`. The package `__init__` also exports `create_marketplace_tools`, which `marketplace.py` does not define. The example above was run against `marketplace.py` as written with only that one import stubbed.
