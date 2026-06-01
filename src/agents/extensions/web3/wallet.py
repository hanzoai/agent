"""Wallet and Web3 integration for agents.

This module provides wallet capabilities for agents to interact with blockchain networks,
enabling on-chain payments, identity, and decentralized coordination.
"""

import secrets
from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any, Optional

from eth_account import Account
from eth_account.hdaccount import generate_mnemonic

# Try to import web3 dependencies
try:
    from eth_typing import Address, HexStr
    from web3 import Web3
    from web3.types import TxParams, Wei

    WEB3_AVAILABLE = True
except ImportError:
    WEB3_AVAILABLE = False
    Web3 = None
    TxParams = dict[str, Any]
    Wei = int
    Address = str
    HexStr = str


@dataclass
class WalletConfig:
    """Configuration for agent wallet."""

    private_key: Optional[str] = None
    mnemonic: Optional[str] = None
    account_index: int = 0
    network_rpc: str = "http://localhost:8545"
    chain_id: int = 31337  # Default to local hardhat/anvil
    gas_limit: int = 3000000
    gas_price_gwei: int = 20

    def __post_init__(self):
        """Validate configuration."""
        if not self.private_key and not self.mnemonic:
            # Generate a new random private key if none provided
            self.private_key = "0x" + secrets.token_hex(32)


@dataclass
class Transaction:
    """Represents a blockchain transaction."""

    hash: str
    from_address: str
    to_address: str
    value: Wei
    gas_used: Optional[int] = None
    status: Optional[bool] = None
    block_number: Optional[int] = None


class WalletInterface(ABC):
    """Abstract interface for wallet implementations."""

    @abstractmethod
    def get_address(self) -> str:
        """Get the wallet's address."""
        pass

    @abstractmethod
    def get_balance(self) -> Wei:
        """Get the wallet's balance in Wei."""
        pass

    @abstractmethod
    def sign_message(self, message: str) -> str:
        """Sign a message with the wallet's private key."""
        pass

    @abstractmethod
    def send_transaction(
        self,
        to: str,
        value: Wei,
        data: Optional[bytes] = None,
        gas_limit: Optional[int] = None,
        gas_price: Optional[Wei] = None,
    ) -> Transaction:
        """Send a transaction."""
        pass

    @abstractmethod
    def call_contract(
        self, contract_address: str, function_signature: str, *args, **kwargs
    ) -> Any:
        """Call a smart contract function."""
        pass


class Web3Wallet(WalletInterface):
    """Web3-based wallet implementation."""

    def __init__(self, config: WalletConfig):
        """Initialize Web3 wallet."""
        if not WEB3_AVAILABLE:
            raise ImportError(
                "Web3 dependencies not available. Install with: pip install web3 eth-account"
            )

        self.config = config
        self.w3 = Web3(Web3.HTTPProvider(config.network_rpc))

        # Initialize account from private key or mnemonic
        if config.private_key:
            self.account = Account.from_key(config.private_key)
        elif config.mnemonic:
            # Derive account from mnemonic at given index
            Account.enable_unaudited_hdwallet_features()
            self.account = Account.from_mnemonic(
                config.mnemonic, account_path=f"m/44'/60'/0'/0/{config.account_index}"
            )
        else:
            raise ValueError("Either private_key or mnemonic must be provided")

        # Ensure connection (best-effort; some providers do not implement net_version)
        try:
            connected = self.w3.is_connected()
        except Exception:
            connected = False
        if not connected:
            raise ConnectionError(f"Cannot connect to {config.network_rpc}")

    def get_address(self) -> str:
        """Get the wallet's address."""
        return self.account.address

    def get_balance(self) -> Wei:
        """Get the wallet's balance in Wei."""
        return self.w3.eth.get_balance(self.account.address)

    def sign_message(self, message: str) -> str:
        """Sign a message using EIP-191 (``personal_sign``).

        This is the same shape :func:`eth_account.Account.sign_message`
        produces, so the resulting signature is recoverable via
        :func:`eth_account.Account.recover_message` against
        :func:`eth_account.messages.encode_defunct`. Callers that want
        a raw-digest sign should use :meth:`sign_digest`.
        """
        from eth_account.messages import encode_defunct

        signable = encode_defunct(text=message)
        signed = self.account.sign_message(signable)
        return signed.signature.hex()

    def sign_digest(self, digest_hex: str) -> str:
        """Sign a pre-computed 32-byte digest directly (no EIP-191 prefix)."""
        digest_hex = digest_hex[2:] if digest_hex.startswith(("0x", "0X")) else digest_hex
        digest_bytes = bytes.fromhex(digest_hex)
        if len(digest_bytes) != 32:
            raise ValueError("digest must be 32 bytes (64 hex chars)")
        signed = self.account.signHash(digest_bytes)
        return signed.signature.hex()

    def send_transaction(
        self,
        to: str,
        value: Wei,
        data: Optional[bytes] = None,
        gas_limit: Optional[int] = None,
        gas_price: Optional[Wei] = None,
    ) -> Transaction:
        """Send a transaction."""
        # Build transaction
        tx: TxParams = {
            "from": self.account.address,
            "to": to,
            "value": value,
            "gas": gas_limit or self.config.gas_limit,
            "gasPrice": gas_price or Web3.to_wei(self.config.gas_price_gwei, "gwei"),
            "nonce": self.w3.eth.get_transaction_count(self.account.address),
            "chainId": self.config.chain_id,
        }

        if data:
            tx["data"] = data

        # Sign transaction
        signed_tx = self.account.sign_transaction(tx)

        # Send transaction
        tx_hash = self.w3.eth.send_raw_transaction(signed_tx.rawTransaction)

        # Wait for receipt
        receipt = self.w3.eth.wait_for_transaction_receipt(tx_hash)

        return Transaction(
            hash=tx_hash.hex(),
            from_address=self.account.address,
            to_address=to,
            value=value,
            gas_used=receipt["gasUsed"],
            status=receipt["status"] == 1,
            block_number=receipt["blockNumber"],
        )

    def call_contract(
        self,
        contract_address: str,
        function_signature: str,
        *args,
        abi: Optional[list[dict[str, Any]]] = None,
        write: bool = False,
        value: Wei = 0,
        gas_limit: Optional[int] = None,
        gas_price: Optional[Wei] = None,
        **_kwargs,
    ) -> Any:
        """Call a smart contract function.

        ``function_signature`` is the function name as it appears in the
        ABI (e.g. ``"balanceOf"``, ``"transfer"``). ``args`` are passed
        positionally to that function.

        ``abi`` is the full contract ABI as a list of dicts. If omitted,
        a minimal one-function ABI is synthesized from
        ``function_signature`` parsed as Solidity-style
        ``"name(type1,type2)"`` with no return value. That fallback is
        only useful for fire-and-forget writes; reads need a real ABI
        so the return type is decoded correctly.

        ``write=False`` (default) performs an ``eth_call`` and returns
        the decoded result. ``write=True`` builds, signs, and submits a
        transaction, returning a :class:`Transaction`.
        """
        if abi is None:
            abi = [_minimal_abi_from_signature(function_signature)]
            fn_name = function_signature.split("(", 1)[0]
        else:
            fn_name = function_signature

        checksum_addr = Web3.to_checksum_address(contract_address)
        contract = self.w3.eth.contract(address=checksum_addr, abi=abi)
        fn = getattr(contract.functions, fn_name)(*args)

        if not write:
            return fn.call({"from": self.account.address})

        # Build a write tx via the contract function, then sign locally.
        tx: TxParams = fn.build_transaction({
            "from": self.account.address,
            "value": value,
            "gas": gas_limit or self.config.gas_limit,
            "gasPrice": gas_price or Web3.to_wei(self.config.gas_price_gwei, "gwei"),
            "nonce": self.w3.eth.get_transaction_count(self.account.address),
            "chainId": self.config.chain_id,
        })
        signed_tx = self.account.sign_transaction(tx)
        tx_hash = self.w3.eth.send_raw_transaction(signed_tx.rawTransaction)
        receipt = self.w3.eth.wait_for_transaction_receipt(tx_hash)
        return Transaction(
            hash=tx_hash.hex(),
            from_address=self.account.address,
            to_address=checksum_addr,
            value=value,
            gas_used=receipt["gasUsed"],
            status=receipt["status"] == 1,
            block_number=receipt["blockNumber"],
        )


class MockWallet(WalletInterface):
    """Mock wallet for testing without blockchain.

    Backed by a real :class:`eth_account.Account` so EIP-191 signatures
    produced here round-trip through
    :meth:`AgentWallet.verify_signature`. The "mock" aspect is the
    in-memory balance + transaction ledger; signing is real.
    """

    def __init__(self, config: WalletConfig):
        """Initialize mock wallet."""
        self.config = config
        # Back the mock with a real account so signing/recovery works.
        if config.private_key:
            self._account = Account.from_key(config.private_key)
        elif config.mnemonic:
            Account.enable_unaudited_hdwallet_features()
            self._account = Account.from_mnemonic(
                config.mnemonic, account_path=f"m/44'/60'/0'/0/{config.account_index}"
            )
        else:
            self._account = Account.create()
        self.address = self._account.address
        self.balance = Wei(1000000000000000000000)  # 1000 ETH
        self.transactions: list[Transaction] = []

    def get_address(self) -> str:
        """Get the wallet's address."""
        return self.address

    def get_balance(self) -> Wei:
        """Get the wallet's balance in Wei."""
        return self.balance

    def sign_message(self, message: str) -> str:
        """Sign a message using EIP-191 (mock signing uses a real key)."""
        from eth_account.messages import encode_defunct

        signable = encode_defunct(text=message)
        signed = self._account.sign_message(signable)
        return signed.signature.hex()

    def send_transaction(
        self,
        to: str,
        value: Wei,
        data: Optional[bytes] = None,
        gas_limit: Optional[int] = None,
        gas_price: Optional[Wei] = None,
    ) -> Transaction:
        """Send a transaction (mock)."""
        if value > self.balance:
            raise ValueError("Insufficient balance")

        self.balance -= value

        tx = Transaction(
            hash="0x" + secrets.token_hex(32),
            from_address=self.address,
            to_address=to,
            value=value,
            gas_used=21000,
            status=True,
            block_number=len(self.transactions),
        )

        self.transactions.append(tx)
        return tx

    def call_contract(
        self, contract_address: str, function_signature: str, *args, **kwargs
    ) -> Any:
        """Call a smart contract function (mock)."""
        return f"Mock result for {function_signature}"


class AgentWallet:
    """High-level wallet interface for agents."""

    def __init__(
        self,
        config: Optional[WalletConfig] = None,
        *,
        mpc_client: Optional[Any] = None,
    ):
        """Initialize agent wallet.

        ``mpc_client`` is an optional async :class:`MpcClient` instance.
        When set, :meth:`sign_via_mpc` will route signing through the
        threshold-signing cluster. Local signing is always available as
        a fallback via :meth:`sign_message`.
        """
        self.config = config or WalletConfig()

        # Use mock wallet if web3 not available or in test mode
        if WEB3_AVAILABLE and not self.config.network_rpc.startswith("mock://"):
            self.wallet = Web3Wallet(self.config)
        else:
            self.wallet = MockWallet(self.config)

        self._mpc_client = mpc_client

    @property
    def address(self) -> str:
        """Get wallet address."""
        return self.wallet.get_address()

    @property
    def balance(self) -> Wei:
        """Get wallet balance."""
        return self.wallet.get_balance()

    @property
    def has_mpc(self) -> bool:
        """``True`` if an :class:`MpcClient` is wired up for threshold signing."""
        return self._mpc_client is not None

    def send_payment(
        self, to: str, amount_ether: float, memo: Optional[str] = None
    ) -> Transaction:
        """Send a payment to another address.

        Args:
            to: Recipient address
            amount_ether: Amount in Ether (not Wei)
            memo: Optional memo (stored off-chain)

        Returns:
            Transaction object
        """
        amount_wei = Wei(int(amount_ether * 10**18))

        # Log memo if provided (would be stored in agent memory)
        if memo:
            print(f"Payment memo: {memo}")

        return self.wallet.send_transaction(to, amount_wei)

    def sign_message(self, message: str) -> str:
        """Sign a message for authentication.

        Uses the local key. Use :meth:`sign_via_mpc` to route through
        the configured MPC cluster instead.
        """
        return self.wallet.sign_message(message)

    async def sign_via_mpc(self, digest_hex: str, *, key_id: str) -> str:
        """Threshold-sign a digest through the configured MPC client.

        Raises :class:`RuntimeError` if no :class:`MpcClient` was wired
        up at construction time. Returns the hex signature.
        """
        if self._mpc_client is None:
            raise RuntimeError(
                "AgentWallet has no MpcClient configured; pass mpc_client=... at construction"
            )
        result = await self._mpc_client.sign(digest_hex, key_id=key_id)
        return result.signature

    def verify_signature(
        self, message: str, signature: str, expected_address: str
    ) -> bool:
        """Verify ``signature`` was produced by ``expected_address`` over ``message``.

        Uses :func:`eth_account.Account.recover_message` with the standard
        EIP-191 prefix (the same prefix :meth:`sign_message` applies via
        :func:`encode_defunct`). Returns ``False`` rather than raising
        when the signature is malformed, so callers can treat
        verification as a boolean test.
        """
        if not signature or not expected_address:
            return False
        try:
            from eth_account.messages import encode_defunct

            recovered = Account.recover_message(
                encode_defunct(text=message), signature=signature
            )
        except Exception:
            return False
        return recovered.lower() == expected_address.lower()


def _minimal_abi_from_signature(signature: str) -> dict[str, Any]:
    """Build a minimal one-function ABI from a Solidity-style signature.

    ``"transfer(address,uint256)"`` ->

        {"type": "function", "name": "transfer",
         "inputs": [{"type": "address"}, {"type": "uint256"}],
         "outputs": [], "stateMutability": "nonpayable"}
    """
    if "(" not in signature or not signature.endswith(")"):
        # Bare name, no types: treat as no-arg view fn.
        return {
            "type": "function",
            "name": signature,
            "inputs": [],
            "outputs": [],
            "stateMutability": "view",
        }
    name, rest = signature.split("(", 1)
    types_csv = rest[:-1]
    inputs = [{"type": t.strip(), "name": f"arg{i}"}
              for i, t in enumerate(types_csv.split(",")) if t.strip()]
    return {
        "type": "function",
        "name": name,
        "inputs": inputs,
        "outputs": [],
        "stateMutability": "nonpayable",
    }


def generate_shared_mnemonic() -> str:
    """Generate a shared mnemonic for a network of agents."""
    return generate_mnemonic(num_words=12, lang="english")


def derive_agent_wallet(mnemonic: str, agent_index: int, **kwargs) -> AgentWallet:
    """Derive an agent wallet from shared mnemonic.

    Args:
        mnemonic: Shared network mnemonic
        agent_index: Unique index for this agent
        **kwargs: Additional wallet config options

    Returns:
        AgentWallet instance
    """
    config = WalletConfig(mnemonic=mnemonic, account_index=agent_index, **kwargs)
    return AgentWallet(config)


# Tool functions for agent use
def create_wallet_tool():
    """Create a tool that agents can use for wallet operations."""
    from ..tool import Tool

    class WalletTool(Tool):
        """Tool for wallet operations."""

        def __init__(self, wallet: AgentWallet):
            self.wallet = wallet
            self.name = "wallet"
            self.description = "Interact with blockchain wallet"

        async def get_balance(self) -> float:
            """Get wallet balance in Ether."""
            balance_wei = self.wallet.balance
            return float(balance_wei) / 10**18

        async def send_payment(
            self, to_address: str, amount_ether: float, reason: Optional[str] = None
        ) -> str:
            """Send payment to another address.

            Args:
                to_address: Recipient blockchain address
                amount_ether: Amount to send in Ether
                reason: Optional reason for payment

            Returns:
                Transaction hash
            """
            tx = self.wallet.send_payment(to_address, amount_ether, reason)
            return f"Sent {amount_ether} ETH to {to_address}. Tx: {tx.hash}"

        async def get_address(self) -> str:
            """Get this wallet's address."""
            return self.wallet.address

        async def sign_message(self, message: str) -> str:
            """Sign a message for authentication."""
            return self.wallet.sign_message(message)

    return WalletTool
