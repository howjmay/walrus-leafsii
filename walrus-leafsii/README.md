# Leafsii Protocol - Sui/Move Implementation

This repository implements the Leafsii Protocol in Sui/Move with **β = 0** (fixed Pf = $1.00) and a **deferred stability pool** model, faithfully following the provided pseudocode specifications.

## Overview

The Leafsii Protocol is a synthetic asset system that creates two tokens:
- **ftoken**: Low-volatility liability token (fixed at $1.00 when β=0)  
- **xtoken**: High-volatility equity token (tracked but not minted in this β=0 implementation)

The protocol uses a **deferred stability pool** model where L3 rebalances burn ftoken from the pool but defer SUI payments via indexing, only paying out when users explicitly claim rewards.

## File Structure

```
sources/
├── leafsii.move          # Core protocol implementation
├── collateral_registry.move # Cross-chain collateral registry + Walrus proof anchors
├── crosschain_vault.move # FXN cross-chain pair implementation with Walrus vouchers
├── stability_pool.move  # Stability pool with scaled-shares + index accrual  
├── oracle.move         # Price oracle interface and mock implementation
└── math.move           # Fixed-point math utilities

tests/
└── test_leafsii_core.move # Comprehensive unit tests
```

## Pseudocode to Move Mapping

### Core Constants (pseudocode_stable.py → leafsii.move)

| Pseudocode | Move Implementation | Value |
|------------|-------------------|--------|
| `CR_T_L1 = 1.306` | `CR_T_L1: u64 = 1306000` | 1.306 in milli-units |
| `CR_T_L2 = 1.206` | `CR_T_L2: u64 = 1206000` | 1.206 in milli-units |
| `CR_T_L3 = 1.144` | `CR_T_L3: u64 = 1144000` | 1.144 in milli-units |
| `CR_T_L4 = 1.050` | `CR_T_L4: u64 = 1050000` | 1.050 in milli-units |
| `BETA_F = 0.0` | `BETA_F: u64 = 0` | β_f = 0 (locked) |
| `PF_FIXED = 1.0` | `PF_FIXED: u64 = 1_000_000` | $1.00 in micro-USD |
| `MAX_STALENESS_SEC = 3600` | `MAX_STALENESS_MS: u64 = 3600000` | 1 hour in milliseconds |
| `MAX_REL_STEP = 0.20` | `MAX_REL_STEP: u64 = 200000` | 20% in basis points |

### Global State Mapping

| Pseudocode | Move Implementation | Type |
|------------|-------------------|------|
| `Nf` | `protocol.nf` | `u64` |
| `Nx` | `protocol.nx` | `u64` |
| `Pf` | `protocol.pf` | `u64` (micro-USD) |
| `Px` | `protocol.px` | `u64` (micro-USD) |
| `p_sui` | `protocol.p_r` | `u64` (micro-USD) |
| `reserve_sui` | `protocol.reserve_r` | `Balance<R>` |
| `treasury_sui` | `protocol.treasury_r` | `Balance<R>` |
| `last_oracle_ts` | `protocol.last_oracle_ts` | `u64` |

### Key Functions

#### Oracle Update (`update_from_oracle`)

**Pseudocode:**
```python
def update_from_oracle(p_new: float, now_ts: int):
    # Staleness and step checks
    # Pf = PF_FIXED (β_f = 0)
    # Update Px from invariant
```

**Move:**
```move
public fun update_from_oracle<R>(
    protocol: &mut Protocol<R>,
    oracle: &MockOracle<R>, 
    stability_pool: &StabilityPool<R, FToken<R>>,
    clock: &Clock,
    _admin: &AdminCap
)
```

#### Mint fToken (`mint_ftoken` → `mint_f`)

**Pseudocode:**
```python
def mint_ftoken(deposit_sui: float) -> float:
    reserve_sui += deposit_sui
    issued_f = deposit_sui * p_sui / Pf
    Nf += issued_f
    return issued_f
```

**Move:**
```move
public fun mint_f<R>(
    protocol: &mut Protocol<R>,
    stability_pool: &StabilityPool<R, FToken<R>>,
    reserve_in: Coin<R>,
    ctx: &mut TxContext
)
```

#### Redeem fToken (`redeem_ftoken` → `redeem_f`)

**Pseudocode:**
```python
def redeem_ftoken(burn_f: float) -> float:
    sui_out = burn_f * Pf / p_sui
    if sui_out > reserve_net_sui(): raise Exception("insufficient reserve")
    reserve_sui -= sui_out
    Nf -= burn_f
    return sui_out
```

**Move:**
```move
public fun redeem_f<R>(
    protocol: &mut Protocol<R>,
    stability_pool: &StabilityPool<R, FToken<R>>,
    f_in: Coin<FToken<R>>,
    ctx: &mut TxContext
)
```

#### L3 Protocol Rebalance

**Pseudocode:**
```python
def protocol_rebalance_L3_to_target(target_cr: float = CR_T_L1) -> tuple[float, float]:
    need = _compute_f_burn_needed_for_target(target_cr)
    cap_sp = sp_quote_burn_cap()
    f_burn = min(need, cap_sp, Nf)
    payout_sui = f_burn * Pf / p_sui
    burned, indexed_sui = sp_controller_rebalance(f_burn, payout_sui)
    Nf -= burned  # Do NOT touch reserve_sui (deferred)
    return (burned, indexed_sui)
```

**Move:**
```move
public fun protocol_rebalance_l3_to_target<R>(
    protocol: &mut Protocol<R>,
    stability_pool: &mut StabilityPool<R, FToken<R>>,
    target_cr: u64,
    _admin: &AdminCap,
    ctx: &mut TxContext
)
```

### Stability Pool Implementation (rebalance_pool.py → stability_pool.move)

#### State Mapping

| Pseudocode | Move Implementation | Type |
|------------|-------------------|------|
| `sp_scale` | `pool.sp_scale` | `u64` (scaled by 1e9) |
| `sp_scaled_total` | `pool.sp_scaled_total` | `u64` |
| `sp_index_sui_scaled` | `pool.sp_index_r_scaled` | `u128` (scaled by 1e18) |
| `sp_obligation_sui` | `pool.sp_obligation_r` | `Balance<R>` |

#### Controller Rebalance (`sp_controller_rebalance`)

**Pseudocode:**
```python
def sp_controller_rebalance(f_burn: float, payout_sui: float) -> tuple[float, float]:
    f_total_pre = sp_total_f()
    allowed_burn = min(f_burn, SP_MAX_BURN_FRAC_CALL * f_total_pre)
    # Index accrual: delta = allowed_payout / sp_scaled_total
    sp_index_sui_scaled += delta
    sp_obligation_sui += allowed_payout
    # Pro-rata burn: sp_scale *= (1 - frac)
    frac = allowed_burn / f_total_pre
    sp_scale *= (1.0 - frac)
    return (allowed_burn, allowed_payout)
```

**Move:**
```move
public fun sp_controller_rebalance<R, FToken>(
    pool: &mut StabilityPool<R, FToken>,
    f_burn: u64,
    payout_r: Balance<R>
): (u64, Balance<R>)
```

#### User Flows

**Note:** `SPUser` in the pseudocode represents a stability pool participant. In the Move implementation, this is represented by a `Position` object that users create to track their stability pool deposits and rewards.

**Deposit (`sp_deposit` → `deposit_f`):**
```python
def sp_deposit(user: SPUser, f_amount: float):
    newly = _settle_user(user)
    scaled = f_amount / sp_scale
    user.sp_scaled += scaled
    sp_scaled_total += scaled
```

**Withdraw (`sp_withdraw` → `withdraw_f`):**
```python
def sp_withdraw(user: SPUser, f_amount: float):
    newly = _settle_user(user)
    scaled = f_amount / sp_scale
    user.sp_scaled -= scaled
    sp_scaled_total -= scaled
```

**Claim (`sp_claim` → `claim_rewards`):**
```python
def sp_claim(user: SPUser, core_pay_sui_cb):
    owed = _settle_user(user)
    core_pay_sui_cb(owed)  # Reduces reserve_sui
    sp_obligation_sui -= owed
```

## Key Differences from Pseudocode

### 1. Fixed-Point Arithmetic
- **Pseudocode:** Uses floats
- **Move:** Uses fixed-point integers with specific scales:
  - Prices: 1e6 (micro-USD)
  - Collateral ratios: 1e6 (milli-units)
  - SP scale: 1e9
  - SP index: 1e18 (for precision)

### 2. Sui-Specific Patterns
- **Entry functions:** Transfer coins to sender instead of returning
- **Object model:** Protocol state and user positions are objects
- **Balance vs Coin:** Internal accounting uses `Balance<T>`, external transfers use `Coin<T>`
- **Friend functions:** SP can call core functions via `public(package)` visibility

### 3. Safety Features
- **AdminCap:** Required for governance functions like oracle updates and L3 rebalancing
- **Staleness checks:** Oracle updates enforce maximum age and step size
- **Solvency guards:** Reserve must always cover SP obligations
- **Burn caps:** Both core and SP enforce 50% max burn per call

### 4. Events
Rich event emission for all major operations:
- `MintF`, `RedeemF`, `EnterRisk`, `UserRebalanceL3`
- `SPScaleShrink`, `SPIndexAccrual`, `ClaimRewards`
- `OracleUpdate`, `EmergencyRecap`

## Usage Examples

### Initialize Protocol
```move
let treasury_cap = create_ftoken_treasury_cap_for_testing<SUI>(ctx);
let (protocol, admin_cap) = leafsii::init_protocol(
    treasury_cap, 
    2_000_000, // $2.00 initial price
    &clock, 
    ctx
);
let stability_pool = stability_pool::create_stability_pool<SUI, FToken<SUI>>(ctx);
```

### Mint fTokens
```move
let sui_coin = coin::mint_for_testing<SUI>(1_000_000, ctx); // 1 SUI
leafsii::mint_f(&mut protocol, &stability_pool, sui_coin, ctx);
// User receives fTokens worth $2.00 (assuming $2/SUI price)
```

### Deposit to Stability Pool
```move
let position = stability_pool::create_position<SUI, FToken<SUI>>(ctx);
let f_tokens = /* get f_tokens */;
stability_pool::deposit_f(&mut stability_pool, &mut position, f_tokens, ctx);
```

### Protocol Rebalance (L3)
```move
leafsii::protocol_rebalance_l3_to_target(
    &mut protocol,
    &mut stability_pool,
    1306000, // Target CR = 1.306
    &admin_cap,
    ctx
);
```

### Claim SP Rewards
```move
let rewards_balance = stability_pool::claim_rewards(
    &mut stability_pool, 
    &mut position, 
    ctx
);
// Convert balance to coin and transfer to user
```

## Testing

Run the comprehensive test suite:

```bash
sui move test
```

Tests cover:
- ✅ Oracle updates and mode gates
- ✅ Mint/redeem with proper CR calculation  
- ✅ L3 protocol rebalance with deferred payments
- ✅ SP deposit/withdraw with scaled-shares math
- ✅ SP claim with obligation reduction
- ✅ Burn cap enforcement on both sides
- ✅ Invariant and solvency checks
- ✅ Yield harvest indexing

## Deployment

1. **Set up addresses** in `Move.toml`
2. **Deploy contracts:** `sui client publish --gas-budget 50000000`
3. **Initialize protocol** with proper oracle and initial parameters
4. **Set up governance** with appropriate `AdminCap` holders

## Security Considerations

- **Oracle Safety:** Staleness and step size limits prevent manipulation
- **Burn Caps:** Prevent excessive liquidation in single transaction  
- **Solvency:** Protocol ensures `reserve_r >= sp_obligation_r` at all times
- **Access Control:** Critical functions require `AdminCap`
- **Integer Safety:** All math uses overflow-safe operations from `math.move`

## Audit Checklist ✅

- [x] Constants match pseudocode values exactly
- [x] Both sides enforce burn caps (50% max per call)
- [x] `reserve_net` subtracts `sp_obligation_r` for CR calculation
- [x] L3 path uses burn-and-index (no immediate reserve deduction)
- [x] SP claims reduce both `reserve_r` and `sp_obligation_r`
- [x] Fixed-point math with proper rounding direction
- [x] All state changes emit comprehensive events
- [x] Idiomatic Sui/Move with snake_case naming
- [x] Compact functions with clear invariant documentation

## Cross-Chain Collateral Extension (Walrus-backed)

`collateral_registry.move` and `crosschain_vault.move` extend Leafsii beyond staked SUI by letting FXN pairs mint against off-chain assets attested via Walrus checkpoints.

### Collateral Registry
- `register_crosschain<Marker>()` stores per-asset params (β split, LTV, oracle ids, Walrus anchor, mint/redeem caps)
- Walrus proofs are anchored via `WalrusBlobRef` objects and can be rotated with `update_walrus_anchor`
- Governance can pause a collateral type (`set_collateral_active`) or refresh risk caps with `set_crosschain_risk_config`
- Helper getters expose metadata without leaking struct internals, so downstream modules stay agnostic to storage layout

### Cross-Chain Series Lifecycle
- `init_crosschain_series<StableToken, LeverageToken>()` binds a Walrus registry entry to new f/x treasuries
- `update_checkpoint` ingests Walrus proofs, while `mint_from_attested_shares` verifies user balance proofs, enforces LTV, and mints f/x tokens at oracle prices
- `redeem_f` burns fTokens and emits `WithdrawalVoucher` objects encoding owner, nonce, expiry, and share data for off-chain settlement

### Vouchers and Rate Limits
- Vouchers advance through `mark_voucher_spent` / `mark_voucher_settled`, letting relayers attach external references as on-chain audit trails
- Per-epoch mint/redeem rate accounting ensures flows stay within configured USD caps to throttle bridge risk exposure
