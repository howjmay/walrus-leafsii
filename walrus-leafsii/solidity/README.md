# Ethereum Vault Contracts

Walrus-backed ETH vault referenced in `CROSSCHAIN_IMPLEMENTATION.md` and `USAGE_GUIDE.md` lives in `solidity/contracts/WalrusEthVault.sol`.

- Accepts native ETH deposits and mints tracked shares (preview helpers mirror ERC4626).
- Emits deposit/redeem/rebase events so the off-chain monitor can build Walrus checkpoints.
- Redeems ETH with EIP-712 vouchers issued from the Sui burn flow (self-custody withdrawals).
- Monitor role can bump `indexRay` when staking yield accrues and anchor new Walrus blob ids.

## Quick Usage

```solidity
WalrusEthVault vault = new WalrusEthVault(monitorAddr);
vault.deposit{value: 1 ether}(msg.sender, "0xSuiOwner", 0); // mint shares at current index

// Voucher hash for signing in the UI / tests
WalrusEthVault.Voucher memory v = WalrusEthVault.Voucher({
    voucherId: keccak256("voucher_1"),
    redeemer: msg.sender,
    suiOwner: "0xSuiOwner",
    shares: vault.previewDeposit(1 ether),
    nonce: 1,
    expiry: uint64(block.timestamp + 1 days),
    updateId: 1
});
bytes32 digest = vault.hashVoucher(v);
```

Compile with your preferred toolchain (e.g., Foundry `forge build`) pointing at `solidity/`.
