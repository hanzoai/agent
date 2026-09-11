"""Web3 integration for Hanzo agents.

Provides wallet management, transaction handling, and MPC custody.

``web3_agent`` and ``web3_network`` are not exported: they import modules this
package does not have (``hanzo_agents.router``, ``hanzo_agents.network``).
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
