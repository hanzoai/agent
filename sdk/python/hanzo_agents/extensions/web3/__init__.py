"""Web3 integration for Hanzo agents.

Provides wallet management, transaction handling, and MPC custody.

``web3_agent`` and ``web3_network`` are NOT exported here. They are written
against an agent model this package does not have: they import
``hanzo_agents.agent.InferenceResult``, ``hanzo_agents.execution_context.State``,
``hanzo_agents.router.Router``/``RouterFn`` and ``hanzo_agents.network``, and
none of those exist. An agent here serves reasoners and skills over HTTP; those
two describe a router stepping a network of agents over shared state, which is
a different design and not one this SDK can be made to satisfy by renaming
imports.

Importing either from this package's ``__init__`` made every module in it
unimportable, including the four that are sound — and took the whole test
suite's collection down with them. Whether this SDK grows a network model, or
those two modules go, is a decision to make rather than a rename to guess at.
"""

from .wallet import (
    AgentWallet,
    Transaction,
    WalletConfig,
    create_wallet_tool,
    derive_agent_wallet,
    generate_shared_mnemonic,
)

__all__ = [
    "AgentWallet",
    "Transaction",
    "WalletConfig",
    "create_wallet_tool",
    "derive_agent_wallet",
    "generate_shared_mnemonic",
]
