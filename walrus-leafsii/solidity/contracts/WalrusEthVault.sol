// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

// --- Minimal utility contracts/libraries ---

abstract contract ReentrancyGuard {
    uint256 private constant _NOT_ENTERED = 1;
    uint256 private constant _ENTERED = 2;
    uint256 private _status;

    constructor() {
        _status = _NOT_ENTERED;
    }

    modifier nonReentrant() {
        require(_status != _ENTERED, "REENTRANCY");
        _status = _ENTERED;
        _;
        _status = _NOT_ENTERED;
    }
}

library ECDSA {
    enum RecoverError {
        NoError,
        InvalidSignature,
        InvalidSignatureLength,
        InvalidSignatureS,
        InvalidSignatureV
    }

    function recover(bytes32 hash, bytes memory signature) internal pure returns (address) {
        (address recovered, RecoverError error) = tryRecover(hash, signature);
        _throwError(error);
        return recovered;
    }

    function tryRecover(bytes32 hash, bytes memory signature) internal pure returns (address, RecoverError) {
        if (signature.length == 65) {
            bytes32 r;
            bytes32 s;
            uint8 v;
            assembly {
                r := mload(add(signature, 0x20))
                s := mload(add(signature, 0x40))
                v := byte(0, mload(add(signature, 0x60)))
            }
            return tryRecover(hash, v, r, s);
        }
        return (address(0), RecoverError.InvalidSignatureLength);
    }

    function tryRecover(bytes32 hash, uint8 v, bytes32 r, bytes32 s) internal pure returns (address, RecoverError) {
        if (uint256(s) > 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0) {
            return (address(0), RecoverError.InvalidSignatureS);
        }
        if (v != 27 && v != 28) {
            return (address(0), RecoverError.InvalidSignatureV);
        }

        address signer = ecrecover(hash, v, r, s);
        if (signer == address(0)) {
            return (address(0), RecoverError.InvalidSignature);
        }

        return (signer, RecoverError.NoError);
    }

    function _throwError(RecoverError error) private pure {
        if (error == RecoverError.NoError) {
            return;
        } else if (error == RecoverError.InvalidSignatureLength) {
            revert("ECDSA: invalid signature length");
        } else if (error == RecoverError.InvalidSignatureS) {
            revert("ECDSA: invalid signature 's' value");
        } else if (error == RecoverError.InvalidSignatureV) {
            revert("ECDSA: invalid signature 'v' value");
        } else {
            revert("ECDSA: invalid signature");
        }
    }
}

abstract contract EIP712 {
    bytes32 private immutable _CACHED_DOMAIN_SEPARATOR;
    uint256 private immutable _CACHED_CHAIN_ID;
    address private immutable _CACHED_THIS;
    bytes32 private immutable _HASHED_NAME;
    bytes32 private immutable _HASHED_VERSION;
    bytes32 private immutable _TYPE_HASH = keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");

    constructor(string memory name, string memory version) {
        _HASHED_NAME = keccak256(bytes(name));
        _HASHED_VERSION = keccak256(bytes(version));
        _CACHED_CHAIN_ID = block.chainid;
        _CACHED_DOMAIN_SEPARATOR = _buildDomainSeparator(_TYPE_HASH, _HASHED_NAME, _HASHED_VERSION);
        _CACHED_THIS = address(this);
    }

    function _domainSeparatorV4() internal view returns (bytes32) {
        if (address(this) == _CACHED_THIS && block.chainid == _CACHED_CHAIN_ID) {
            return _CACHED_DOMAIN_SEPARATOR;
        }
        return _buildDomainSeparator(_TYPE_HASH, _HASHED_NAME, _HASHED_VERSION);
    }

    function _buildDomainSeparator(
        bytes32 typeHash,
        bytes32 nameHash,
        bytes32 versionHash
    ) private view returns (bytes32) {
        return keccak256(abi.encode(typeHash, nameHash, versionHash, block.chainid, address(this)));
    }

    function _hashTypedDataV4(bytes32 structHash) internal view returns (bytes32) {
        return keccak256(abi.encodePacked("\x19\x01", _domainSeparatorV4(), structHash));
    }
}

/// @title Walrus-backed ETH vault for cross-chain collateral
/// @notice Accepts native ETH deposits, tracks share supply, and lets depositors
///         self-redeem with signed vouchers issued from the Sui side.
///         Events give the off-chain monitor enough data to build Walrus
///         checkpoints for fETH/xETH minting.
contract WalrusEthVault is EIP712, ReentrancyGuard {
    using ECDSA for bytes32;

    uint256 public constant INDEX_SCALE = 1e27; // RAY-style scale for assets/share

    address public owner;
    address public monitor; // monitor can bump index based on external yield
    bool public paused;

    uint256 public indexRay; // assets per share
    uint256 public totalShares;

    mapping(address => uint256) public shareBalance;
    mapping(bytes32 => bool) public spentVouchers;

    bytes32 private constant VOUCHER_TYPEHASH = keccak256(
        "Voucher(bytes32 voucherId,address redeemer,string suiOwner,uint256 shares,uint64 nonce,uint64 expiry,uint64 updateId)"
    );

    struct Voucher {
        bytes32 voucherId;   // deterministic hash from Sui side
        address redeemer;    // EVM address receiving funds
        string suiOwner;     // Sui owner (logged for off-chain correlation)
        uint256 shares;      // shares to burn
        uint64 nonce;        // nonce from Sui voucher
        uint64 expiry;       // unix seconds
        uint64 updateId;     // Walrus checkpoint id used when issuing voucher
    }

    event Deposit(
        address indexed sender,
        address indexed recipient,
        uint256 assets,
        uint256 shares,
        string suiOwner
    );

    event VoucherRedeemed(
        bytes32 indexed voucherId,
        address indexed redeemer,
        address indexed recipient,
        uint256 shares,
        uint256 assets,
        uint256 indexRay,
        uint64 updateId
    );

    event Rebase(
        uint64 indexed updateId,
        uint256 newIndexRay,
        uint256 totalAssets,
        bytes32 walrusBlobId
    );

    event VoucherVoided(bytes32 indexed voucherId, address indexed actor);
    event MonitorUpdated(address indexed newMonitor);
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    event Paused();
    event Unpaused();
    event YieldDonation(address indexed from, uint256 amount);

    error NotOwner();
    error NotMonitor();
    error PausedError();
    error InvalidRecipient();
    error ZeroAssets();
    error Slippage();
    error VoucherExpired();
    error VoucherUsed();
    error InvalidSignature();
    error InsufficientShares();
    error InsufficientLiquidity();

    modifier onlyOwner() {
        if (msg.sender != owner) revert NotOwner();
        _;
    }

    modifier onlyMonitor() {
        if (msg.sender != monitor && msg.sender != owner) revert NotMonitor();
        _;
    }

    modifier whenNotPaused() {
        if (paused) revert PausedError();
        _;
    }

    constructor(address initialMonitor) EIP712("WalrusEthVault", "1") {
        owner = msg.sender;
        monitor = initialMonitor == address(0) ? msg.sender : initialMonitor;
        indexRay = INDEX_SCALE; // start 1:1 assets per share
    }

    /// @notice Deposit ETH and mint vault shares to `recipient`.
    /// @param recipient Address that will hold the shares on Ethereum.
    /// @param suiOwner Sui address (as string) to bind deposits for indexing.
    /// @param minShares Protects against rounding slippage on index changes.
    function deposit(
        address recipient,
        string calldata suiOwner,
        uint256 minShares
    ) external payable whenNotPaused nonReentrant returns (uint256 shares) {
        if (recipient == address(0)) revert InvalidRecipient();
        if (msg.value == 0) revert ZeroAssets();

        shares = previewDeposit(msg.value);
        if (shares < minShares) revert Slippage();

        totalShares += shares;
        shareBalance[recipient] += shares;

        emit Deposit(msg.sender, recipient, msg.value, shares, suiOwner);
    }

    /// @notice Redeem a Sui-issued voucher for ETH.
    /// @param voucher Voucher data (burns shares from `voucher.redeemer`).
    /// @param signature EIP-712 signature from `voucher.redeemer` over voucher hash.
    /// @param recipient Optional payout address (defaults to redeemer).
    function redeemVoucher(
        Voucher calldata voucher,
        bytes calldata signature,
        address payable recipient
    ) external whenNotPaused nonReentrant returns (uint256 assets) {
        if (voucher.expiry < block.timestamp) revert VoucherExpired();
        if (spentVouchers[voucher.voucherId]) revert VoucherUsed();

        bytes32 digest = _hashVoucher(voucher);
        address signer = digest.recover(signature);
        if (signer != voucher.redeemer) revert InvalidSignature();

        uint256 shares = voucher.shares;
        if (shareBalance[voucher.redeemer] < shares) revert InsufficientShares();

        spentVouchers[voucher.voucherId] = true;
        shareBalance[voucher.redeemer] -= shares;
        totalShares -= shares;

        assets = previewRedeem(shares);
        if (assets > address(this).balance) revert InsufficientLiquidity();

        address payable target = recipient == address(0)
            ? payable(voucher.redeemer)
            : recipient;

        _sendValue(target, assets);

        emit VoucherRedeemed(
            voucher.voucherId,
            voucher.redeemer,
            target,
            shares,
            assets,
            indexRay,
            voucher.updateId
        );
    }

    /// @notice Bump the share index after external yield (e.g., staking) accrues.
    /// @dev Off-chain monitor calls this when it publishes a new Walrus checkpoint.
    function recordRebase(
        uint64 updateId,
        uint256 newIndexRay,
        bytes32 walrusBlobId
    ) external onlyMonitor {
        if (newIndexRay == 0) revert Slippage();

        indexRay = newIndexRay;

        emit Rebase(updateId, newIndexRay, address(this).balance, walrusBlobId);
    }

    /// @notice Mark a voucher as unusable (e.g., already settled or slashed).
    function voidVoucher(bytes32 voucherId) external onlyOwner {
        spentVouchers[voucherId] = true;
        emit VoucherVoided(voucherId, msg.sender);
    }

    /// @notice Update the monitor address.
    function setMonitor(address newMonitor) external onlyOwner {
        if (newMonitor == address(0)) revert InvalidRecipient();
        monitor = newMonitor;
        emit MonitorUpdated(newMonitor);
    }

    /// @notice Pause deposits and withdrawals.
    function pause() external onlyOwner {
        paused = true;
        emit Paused();
    }

    /// @notice Resume deposits and withdrawals.
    function unpause() external onlyOwner {
        paused = false;
        emit Unpaused();
    }

    /// @notice Transfer contract ownership.
    function transferOwnership(address newOwner) external onlyOwner {
        if (newOwner == address(0)) revert InvalidRecipient();
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    /// @notice View helper matching ERC4626 preview for deposits.
    function previewDeposit(uint256 assets) public view returns (uint256) {
        return (assets * INDEX_SCALE) / indexRay;
    }

    /// @notice View helper matching ERC4626 preview for redeems.
    function previewRedeem(uint256 shares) public view returns (uint256) {
        return (shares * indexRay) / INDEX_SCALE;
    }

    /// @notice Current ETH held by the vault.
    function totalAssets() external view returns (uint256) {
        return address(this).balance;
    }

    /// @notice Expose EIP-712 digest for off-chain signing/testing.
    function hashVoucher(Voucher calldata voucher) external view returns (bytes32) {
        return _hashVoucher(voucher);
    }

    /// @dev Accept plain ETH transfers as yield donations (no shares minted).
    receive() external payable {
        emit YieldDonation(msg.sender, msg.value);
    }

    function _hashVoucher(Voucher memory voucher) internal view returns (bytes32) {
        return _hashTypedDataV4(
            keccak256(
                abi.encode(
                    VOUCHER_TYPEHASH,
                    voucher.voucherId,
                    voucher.redeemer,
                    keccak256(bytes(voucher.suiOwner)),
                    voucher.shares,
                    voucher.nonce,
                    voucher.expiry,
                    voucher.updateId
                )
            )
        );
    }

    function _sendValue(address payable to, uint256 amount) internal {
        (bool ok, ) = to.call{value: amount}("");
        require(ok, "ETH_TRANSFER_FAILED");
    }
}
