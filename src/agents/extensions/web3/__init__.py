"""Web3 integration for Hanzo agents.

Provides wallet management, transaction handling, MPC-signing integration,
and (when the rest of the runtime is wired up) Web3-enabled agents.

``Web3Agent`` and ``Web3Network`` currently depend on symbols
(``Agent.InferenceResult``, ``State``) that have moved during the
``src/agents`` refactor. They are lazily importable but not re-exported
at package level until that surface is restored. This module's wallet
and MPC primitives are stable today.
"""

from .mpc_client import (
    KeygenResult,
    MpcClient,
    MpcConfig,
    MpcError,
    SignResult,
    VerifyResult,
)
from .wallet import (
    AgentWallet,
    MockWallet,
    Transaction,
    WalletConfig,
    Web3Wallet,
    create_wallet_tool,
    derive_agent_wallet,
    generate_shared_mnemonic,
)

__all__ = [
    "AgentWallet",
    "Transaction",
    "WalletConfig",
    "Web3Wallet",
    "MockWallet",
    "create_wallet_tool",
    "derive_agent_wallet",
    "generate_shared_mnemonic",
    "MpcClient",
    "MpcConfig",
    "MpcError",
    "KeygenResult",
    "SignResult",
    "VerifyResult",
]
