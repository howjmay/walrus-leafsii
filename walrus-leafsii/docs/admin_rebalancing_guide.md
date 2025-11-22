# Admin Protocol Rebalancing Guide

## Overview

This guide explains how protocol administrators can rebalance the Leafsii protocol using the stability pool mechanism. The rebalancing process is critical for maintaining protocol solvency during periods of undercollateralization.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Understanding the Rebalancing Mechanism](#understanding-the-rebalancing-mechanism)
3. [Step-by-Step Rebalancing Process](#step-by-step-rebalancing-process)
4. [Safety Mechanisms](#safety-mechanisms)
5. [Error Handling](#error-handling)
6. [Code Examples](#code-examples)
7. [Monitoring and Best Practices](#monitoring-and-best-practices)

## Prerequisites

### Required Capabilities

To perform protocol rebalancing, you must possess:

1. **StabilityPoolAdminCap**: Controls stability pool operations
   - Location: `sources/stability_pool.move:26`
   - Provides: Full control over a specific stability pool
   - Validation: Must match the target pool's ID

2. **AdminCap**: Controls protocol-level operations
   - Location: `sources/leafsii.move:53`
   - Provides: Protocol administration rights
   - Validation: Must match the protocol instance ID

### System State Requirements

Before initiating rebalancing:

1. **Collateral Ratio Check**: Protocol must be in Level 3 (L3) state
   - Current CR < `CR_T_L3` (1.144)
   - Calculate via: `reserve_net_usd() / (Nf * Pf)`
   - Where `reserve_net_usd = (reserve_sui - sp_obligation_sui) * p_sui`

2. **Stability Pool Deposits**: Must have sufficient fToken deposits
   - Check via: `sp_total_f()` function
   - Minimum viable amount needed for meaningful rebalancing

3. **Oracle Data**: Recent price data must be available
   - Price staleness < `MAX_STALENESS_SEC` (3600 seconds)
   - Price step < `MAX_REL_STEP` (20%)

## Understanding the Rebalancing Mechanism

### Deferred Payment Model

The Leafsii protocol implements a **deferred payment system** for stability pool rebalancing:

1. **Immediate Actions**:
   - Burns fTokens pro-rata from all depositors
   - Reduces fToken supply (`Nf`)
   - Increases collateral ratio

2. **Deferred Actions**:
   - Records SUI obligations to depositors
   - No immediate SUI transfer from reserve
   - Users claim SUI later via separate transactions

### Key Components

#### Scale Factor (`sp_scale`)
- **Purpose**: Implements pro-rata burning across all depositors
- **Initial Value**: 1.0 (scaled to `SCALE_FACTOR` = 1e9)
- **Burn Effect**: Scale factor decreases proportionally to burn amount
- **User Impact**: `user_actual_f = user.sp_scaled * sp_scale`

#### Index System (`sp_index_r_scaled`)
- **Purpose**: Tracks cumulative rewards per scaled share
- **Unit**: Scaled by 1e18 for precision
- **Updates**: Increases when SUI obligations are indexed
- **User Tracking**: Each user has `sp_index_snap` for reward calculation

#### Obligation Tracking (`sp_obligation_r_amount`)
- **Purpose**: Records total SUI owed to depositors
- **Increases**: During rebalancing and yield harvesting
- **Decreases**: When users claim rewards
- **Constraint**: Must not exceed actual reserve

## Step-by-Step Rebalancing Process

### Step 1: Assess Protocol State

```move
// Check current collateral ratio and level
let cr = collateral_ratio(protocol);
let level = current_level(protocol);

// Ensure we're in L3 territory
assert!(level >= 3, "Protocol not in rebalancing range");
```

### Step 2: Calculate Required Burn Amount

The system automatically calculates the fToken burn needed:

```move
// Internal calculation in protocol_rebalance_l3_to_target
let target_cr = CR_T_L1; // 1.306 - stability mode target
let reserve_net = reserve_net_sui() * p_sui;
let nf_target = reserve_net / (target_cr * Pf);
let f_burn_needed = max(0, Nf - nf_target);
```

### Step 3: Apply Burn Caps

Multiple caps protect against excessive burning:

```move
// 1. SP-side cap (50% of pool per call)
let sp_cap = sp_quote_burn_cap(pool); // 50% of total pool

// 2. Protocol-side cap
let protocol_cap = Nf * MAX_F_BURN_FRACTION_PER_CALL; // 50% of supply

// 3. Actual cap applied
let f_burn = min(f_burn_needed, sp_cap, protocol_cap);
```

### Step 4: Execute Rebalancing Transaction

```move
public fun protocol_rebalance_l3_to_target<CoinTypeF, CoinTypeX, CoinTypeR>(
    protocol: &mut Protocol<CoinTypeF, CoinTypeX, CoinTypeR>,
    pool: &mut StabilityPool<CoinTypeR, CoinTypeF>,
    pool_admin_cap: &StabilityPoolAdminCap,
    target_cr: u64,
    admin: &AdminCap,
    ctx: &mut TxContext
): (u64, u64)
```

**Parameters**:
- `protocol`: Mutable reference to protocol instance
- `pool`: Mutable reference to stability pool
- `pool_admin_cap`: Admin capability for pool operations
- `target_cr`: Target collateral ratio (typically `CR_T_L1`)
- `admin`: Protocol admin capability
- `ctx`: Transaction context

### Step 5: Process Results

The function returns `(actual_burn, actual_payout)`:

```move
let (burned_f, indexed_sui) = protocol_rebalance_l3_to_target(
    protocol,
    pool,
    pool_admin_cap,
    target_cr,
    admin,
    ctx
);

// Verify results
assert!(burned_f > 0, "No tokens were burned");
assert!(indexed_sui > 0, "No obligations were indexed");
```

## Safety Mechanisms

### Authorization Checks

1. **Pool Admin Capability Validation**:
   ```move
   assert!(cap.pool_id == object::id(pool), E_INVALID_CONTROLLER_CAP);
   ```

2. **Protocol Admin Capability Validation**:
   ```move
   assert!(admin.protocol_id == object::id(protocol), E_INVALID_ADMIN);
   ```

### Burn Limitations

1. **Per-Call Limits**: Maximum 50% of pool/supply per transaction
2. **Zero-Check Protection**: Prevents division by zero and invalid operations
3. **Balance Validation**: Ensures sufficient pool balance before burning

### Solvency Constraints

1. **Reserve Coverage**: `reserve_sui >= sp_obligation_sui`
2. **Net Reserve Calculation**: Accounts for pending obligations
3. **Invariant Preservation**: Maintains protocol accounting invariants

## Error Handling

### Common Error Codes

- `E_INVALID_CONTROLLER_CAP` (3): Admin cap doesn't match pool
- `E_INVALID_ADMIN`: Protocol admin cap mismatch
- `E_INSUFFICIENT_SP_BALANCE` (2): Insufficient stability pool balance
- `E_INVALID_AMOUNT` (1): Invalid burn amount specified

### Troubleshooting Guide

1. **"No tokens burned" (burned_f = 0)**:
   - Check if pool has sufficient deposits
   - Verify collateral ratio is below L3 threshold
   - Ensure oracle data is current

2. **"Capability mismatch" errors**:
   - Verify admin capabilities match target objects
   - Check object IDs are correct

3. **"Insufficient balance" errors**:
   - Check actual pool balance vs requested burn
   - Consider burn caps may be limiting operation

## Code Examples

### Complete Rebalancing Transaction

```move
use leafsii::stability_pool;
use leafsii::leafsii;

public entry fun admin_rebalance_protocol<F, X, R>(
    protocol: &mut leafsii::Protocol<F, X, R>,
    pool: &mut stability_pool::StabilityPool<R, F>,
    pool_admin_cap: &stability_pool::StabilityPoolAdminCap,
    protocol_admin_cap: &leafsii::AdminCap,
    ctx: &mut TxContext
) {
    // Execute rebalancing to target stability mode
    let (burned, indexed) = leafsii::protocol_rebalance_l3_to_target(
        protocol,
        pool,
        pool_admin_cap,
        1306, // CR_T_L1 in basis points (1.306)
        protocol_admin_cap,
        ctx
    );

    // Log results (in production, emit events)
    debug::print(&b"Burned fTokens: ");
    debug::print(&burned);
    debug::print(&b"Indexed SUI obligations: ");
    debug::print(&indexed);
}
```

### Monitoring Protocol State

```move
public fun check_rebalancing_eligibility<F, X, R>(
    protocol: &leafsii::Protocol<F, X, R>,
    pool: &stability_pool::StabilityPool<R, F>
): (bool, u64, u64, u64) {
    let cr_bps = leafsii::collateral_ratio_bps(protocol);
    let level = leafsii::current_level(protocol);
    let pool_balance = stability_pool::sp_total_f(pool);
    let burn_cap = stability_pool::sp_quote_burn_cap(pool);

    let eligible = level >= 3 && pool_balance > 0;

    (eligible, cr_bps, pool_balance, burn_cap)
}
```

## Monitoring and Best Practices

### Pre-Rebalancing Checklist

1. **State Verification**:
   - [ ] Collateral ratio below L3 threshold (1.144)
   - [ ] Stability pool has sufficient deposits
   - [ ] Oracle data is recent and valid
   - [ ] Admin capabilities are properly configured

2. **Impact Assessment**:
   - [ ] Calculate expected burn amount
   - [ ] Estimate impact on depositors
   - [ ] Verify post-rebalancing CR will reach target
   - [ ] Check reserve solvency post-operation

3. **Authorization**:
   - [ ] Confirm admin access to both protocol and pool
   - [ ] Verify transaction will be executed by authorized account
   - [ ] Double-check capability object IDs match targets

### Post-Rebalancing Monitoring

1. **State Changes**:
   - Monitor new collateral ratio
   - Verify fToken supply reduction
   - Check stability pool obligation increases
   - Confirm reserve solvency maintained

2. **User Impact**:
   - Track depositor balance changes (pro-rata reduction)
   - Monitor pending reward accumulation
   - Observe claim activity and timing

3. **System Health**:
   - Verify protocol invariants maintained
   - Check oracle price impact
   - Monitor for any edge case behaviors

### Emergency Procedures

If rebalancing fails or produces unexpected results:

1. **Immediate Actions**:
   - Stop further rebalancing attempts
   - Assess system state and user impact
   - Contact technical team for investigation

2. **Recovery Options**:
   - Emergency recapitalization (L4 mechanism)
   - Governance intervention
   - User communication and support

3. **Investigation Steps**:
   - Review transaction logs and events
   - Analyze state changes and calculations
   - Identify root cause and prevention measures

## Technical Reference

### Key Constants

```move
// Collateral ratio thresholds (in ratio form)
CR_T_L1 = 1.306   // Stability mode
CR_T_L2 = 1.206   // User rebalance mode
CR_T_L3 = 1.144   // Protocol rebalance
CR_T_L4 = 1.050   // Emergency recap

// Burn limitations
SP_MAX_BURN_FRAC_CALL = 5000  // 50% in basis points
MAX_F_BURN_FRACTION_PER_CALL = 0.50

// Scaling factors
SCALE_FACTOR = 1_000_000_000   // 1e9 for sp_scale
BPS = 10_000                   // Basis points denominator
```

### Function References

- `protocol_rebalance_l3_to_target()`: `sources/leafsii.move:973`
- `sp_controller_rebalance()`: `sources/stability_pool.move:220`
- `burn_from_pool()`: `sources/stability_pool.move:416`
- `sp_total_f()`: `sources/stability_pool.move:141`
- `sp_quote_burn_cap()`: `sources/stability_pool.move:155`
- `collateral_ratio()`: Calculated in protocol logic
- `current_level()`: Determined by CR thresholds

This guide provides the comprehensive framework for safe and effective protocol rebalancing using the stability pool mechanism.