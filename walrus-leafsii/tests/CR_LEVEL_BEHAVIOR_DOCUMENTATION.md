# CR Level Transition Behavior Documentation

## Overview

The Leafsii protocol implements a 5-level Collateral Ratio (CR) system to manage risk and protocol health. This document describes the observed behavior during CR level transitions based on comprehensive testing.

## CR Level System

### Thresholds
- **L1**: CR ≥ 1.306 (130.6%) - High Health
- **L2**: 1.206 ≤ CR < 1.306 (120.6% - 130.6%) - Good Health  
- **L3**: 1.144 ≤ CR < 1.206 (114.4% - 120.6%) - Warning Level
- **L4**: 1.050 ≤ CR < 1.144 (105.0% - 114.4%) - Critical Level
- **L5**: CR < 1.050 (< 105.0%) - Emergency Level

### Action Restrictions
- **L1-L2**: All user operations allowed (mint F, mint X, redeem F, redeem X)
- **L3-L5**: All user operations blocked (E_ACTION_BLOCKED_BY_CR error)

## Observed Behavior

### 1. Normal Operations at L1-L2

**Test**: `test_mint_f_small_amount_at_l2`
- **Setup**: Protocol at L2 level (CR ≈ 1.25)
- **Action**: Mint small amount of F tokens (50,000 units)
- **Result**: ✅ Operation succeeds
- **Final State**: Remains at L1 or L2

**Test**: `test_mint_x_l2_to_l3_transition`  
- **Setup**: Protocol at L2 level
- **Action**: Mint X tokens with large reserve amount (800,000 units)
- **Result**: ✅ Operation succeeds but system maintains L1-L2 level
- **Implication**: System likely implements internal checks to prevent CR dropping below L2

### 2. Operations Blocked at L3+

**Test**: `test_operations_blocked_at_l3`
- **Setup**: Force protocol to L3 via SP obligations
- **Action**: Attempt to mint F tokens (100,000 units)
- **Result**: ❌ Fails with `E_ACTION_BLOCKED_BY_CR`
- **Behavior**: No partial minting - operation rejected entirely

### 3. CR Calculation During Mint Operations

**Test**: `test_cr_calculation_during_mint`
- **Formula**: `CR = (Reserve - SP_Obligations) * Reserve_Price / (NF * PF + NX * PX)`
- **Minting F**: Increases both Reserve and NF proportionally
- **Minting X**: Increases Reserve and NX, with X having higher leverage impact
- **Observable**: CR changes occur with each mint operation

### 4. Level Transition Mechanics

**Test**: `test_multiple_level_transitions`
- **L1 → L2**: Achieved via SP controller rebalancing (adding obligations)
- **L2 → L3**: Further SP obligations or large mint operations
- **Reversibility**: L3 → L2 possible via liquidation settlements

### 5. Protocol Recovery Mechanisms

**Test**: `test_cr_recovery_l3_to_l2`
- **Mechanism**: SP liquidation settlements can improve CR
- **Process**: `stability_pool::settle_user()` reduces obligations
- **Result**: CR improvement allows return to operational levels

## Key Findings

### 1. ⚠️ CRITICAL DISCOVERY: No Pre-emptive Level Checking
**The protocol does NOT check the post-operation CR level before executing operations.**

- ✅ **Operations succeed** if the current CR level allows them (L1-L2)
- ⚠️ **Operations can push the system to L3+** even when starting from L2
- 🚨 **This can result in the system becoming non-operational after a large mint**

**Test Evidence**: `test_large_mint_f_behavior_at_l2_boundary`
- Started at L2 
- Minted 5M reserve tokens (huge amount)
- ✅ Operation succeeded
- System likely ended at L3+ level, making future operations impossible

### 2. Current Level Checking Only
The `require_level_for_user_actions()` function is called at the **beginning** of each operation, using the **current** CR level only. There is no pre-flight check of what the CR would be after the operation.

### 3. Operational State Lock-out Risk
This behavior means users can accidentally lock the protocol into a non-operational state by performing large operations at marginal L2 levels.

### 4. Recovery Mechanisms Work
The protocol can recover from L3+ levels back to operational levels (L1-L2) through:
- SP liquidation settlements
- Natural market movements improving the underlying collateral ratio

## Implementation Implications

### For Users
1. **Monitor CR levels** before initiating large operations
2. **Small operations** are safer at marginal L2 levels  
3. **No operations possible** once system reaches L3+
4. **Wait for recovery** mechanisms to restore system to L1-L2

### For Protocol
1. **Clear binary operation logic** - no complex partial execution
2. **Robust level checking** prevents accidental CR violations
3. **Recovery mechanisms** ensure system can return to operational state
4. **Predictable behavior** for integration and user experience

## Test Coverage

The test suite covers:
- ✅ Small operations at boundary levels
- ✅ Large operations that would violate thresholds  
- ✅ Operations blocked at non-operational levels
- ✅ Multi-level transitions (L1→L2→L3)
- ✅ CR calculation verification during operations
- ✅ Recovery mechanisms from non-operational states

## Recommendations

### Critical Protocol Improvements Needed
1. **🔴 HIGH PRIORITY**: Implement pre-flight CR checking in mint/redeem functions
   - Calculate post-operation CR before executing
   - Reject operations that would push system below L2
   - Or implement partial execution up to L2 boundary

2. **Protocol-Level Safeguards**: Add maximum operation size limits at L2
   - Prevent accidentally locking system into non-operational state
   - Allow small operations to continue safely

### Front-end Integration
3. **Real-time CR Monitoring**: Display current CR level and post-operation estimates
4. **User Warnings**: Alert users when operations might push system to L3+
5. **Operation Sizing**: Provide safe operation size recommendations at L2
6. **Recovery Status**: Track liquidation events that might restore operational levels

### Risk Management
7. **Emergency Procedures**: Document recovery procedures for L3+ lockouts
8. **User Education**: Clearly communicate the lock-out risk at L2 levels

---

*This documentation is based on empirical testing of the Leafsii protocol v1 implementation.*