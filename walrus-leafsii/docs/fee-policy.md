# Leafsii Protocol — Fees & Bonuses by CR Mode

## Overview

This document describes the fee structure and stability bonus mechanisms in the Leafsii protocol. The fee policy dynamically adjusts based on the protocol's collateralization ratio (CR) to incentivize behaviors that maintain system health and stability.

---

## Quick Glossary

* **fToken** – Stable token with fixed $1.00 target value, designed for low volatility
* **xToken** – Leveraged token that captures amplified exposure to reserve price movements
* **CR (Collateralization Ratio)** – System health metric; CR thresholds trigger different operational modes
* **Stability Bonus** – Economic incentive paid to users performing stabilizing actions during risk modes
* **Operational Mode** – Protocol behavior determined by current CR level

---

## CR Thresholds & Operational Modes

The protocol operates in four distinct modes based on its collateralization ratio:

| Mode                             | Trigger (CR)                    | Protocol Behavior                                                                                                                                    |
| -------------------------------- | ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Normal**                       | CR ≥ 130.6%                     | Standard operations with default fees on all actions. No stability incentives.                                                                       |
| **L1 – Stability Mode**          | 120.6% ≤ CR < 130.6%            | **fToken minting disabled**. Reduced fToken redeem fees. Increased xToken redeem fees. **xToken minters earn stability bonuses**.                   |
| **L2 – User Rebalance Mode**     | 114.4% ≤ CR < 120.6%            | Enhanced stability bonuses. **fToken redeemers earn bonuses** (receive > NAV). **xToken minters continue earning bonuses**.                         |
| **L3 – Protocol Rebalance Mode** | CR < 114.4%                     | Critical undercollateralization. User bonuses continue as in L2. **Protocol can execute automatic rebalancing** via stability pool mechanism.       |

> **Note:** Modes stack cumulatively. If CR drops into L2, all L1 restrictions and incentives remain active until CR recovers above 130.6%.

---

## Complete Fee Policy Matrix

Fee rates shown are default operational parameters. Actual rates are configurable by protocol governance.

| Mode                        | fToken **Mint**         | fToken **Redeem**                            | xToken **Mint**                         | xToken **Redeem**        | **Stability Bonus Recipients**                                              |
| --------------------------- | ----------------------- | -------------------------------------------- | --------------------------------------- | ------------------------ | --------------------------------------------------------------------------- |
| **Normal**                  | 0.5% fee                | 0.5% fee                                     | 0.5% fee                                | 0.5% fee                 | None (no bonuses in Normal mode)                                            |
| **L1 – Stability**          | **DISABLED**            | **0% fee**                                   | 0.5% fee + **0.1% bonus**               | **1.0% fee** (increased) | **xToken minters** receive bonuses to incentivize collateral increase       |
| **L2 – User Rebalance**     | **DISABLED**            | **0% fee + 0.1% bonus** (receive > NAV)     | 0.5% fee + **0.1% bonus**               | 0.5% fee                 | **fToken redeemers** and **xToken minters** both earn bonuses               |
| **L3 – Protocol Rebalance** | **DISABLED**            | **0% fee + 0.1% bonus**                      | 0.5% fee + **0.1% bonus**               | 0.5% fee                 | Same as L2 for users; **Protocol** earns fees from automated rebalancing    |

### Fee Mechanics Explained

**Normal Mode:**
- Standard fee model applies uniformly to all operations
- Fees accrue to protocol treasury for operational expenses and governance

**L1 Stability Mode:**
- **fToken minting blocked** to prevent further leverage during undercollateralization
- **xToken minting incentivized** via bonuses to bring additional collateral into the system
- **fToken redemption encouraged** via 0% fees to reduce protocol liabilities
- **xToken redemption discouraged** via higher fees to prevent collateral drain

**L2 User Rebalance Mode:**
- **fToken redeemers earn bonuses** making redemption economically attractive (> NAV value)
- Creates natural arbitrage opportunity that helps restore healthy CR levels
- xToken minting bonuses continue to encourage collateral additions

**L3 Protocol Rebalance Mode:**
- All user incentives from L2 remain active
- Protocol gains authority to execute automatic rebalancing through stability pool
- Rebalancing burns fTokens from stability pool depositors, reducing total liabilities
- Burned fTokens are compensated with equivalent reserve value to depositors

---

## Stability Bonus Funding

**Source of Bonuses:**
- Bonuses are paid from the protocol's **fee treasury**
- Fee treasury accumulates from:
  - Collected fees during Normal mode operations
  - Protocol revenues from various sources
  - Initial treasury allocation

**Economic Impact:**
- Stability bonuses create small temporary NAV adjustments
- During risk modes, bonuses effectively transfer value from fToken holders to rebalancers
- This mechanism naturally incentivizes CR restoration through market forces

---

## Mint/Redeem Impact on Protocol State

**Normal Mode:**
- Mint/redeem operations occur at NAV and don't change token prices
- Operations affect CR through supply changes but not through price changes
- Single-sided operations (e.g., only minting fTokens) shift CR

**Risk Modes (L1+):**
- Stability bonuses can create small NAV effects
- Fee waivers and increased fees guide user behavior toward CR restoration
- Combined with bonus incentives, creates powerful rebalancing forces

---

## Operational Level Determination

The protocol's operational level is determined programmatically at transaction time:

```move
public fun current_level<CoinTypeF, CoinTypeX>(
    protocol: &Protocol<CoinTypeF, CoinTypeX>,
    pool: &StabilityPool<FungibleStakedSui, CoinTypeF>
): u8
```

**Returns:**
- `0`: Normal mode (CR ≥ 130.6%)
- `1`: L1 Stability mode (120.6% ≤ CR < 130.6%)
- `2`: L2 User Rebalance mode (114.4% ≤ CR < 120.6%)
- `3`: L3 Protocol Rebalance mode (CR < 114.4%)

---

## Configuration Parameters

All fee rates are expressed in **basis points** (bps) where 10,000 bps = 100%.

**Default Fee Rates:**
- Normal mode mint/redeem: 50 bps (0.5%)
- L1 xToken redeem (increased): 100 bps (1.0%)
- Stability bonus rate: 10 bps (0.1%)

**Admin Configurable:**
```move
public fun set_fee_config<CoinTypeF, CoinTypeX>(
    protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
    normal_mint_f_fee_bps: u64,
    normal_mint_x_fee_bps: u64,
    normal_redeem_f_fee_bps: u64,
    normal_redeem_x_fee_bps: u64,
    l1_redeem_x_fee_bps: u64,
    stability_bonus_rate_bps: u64,
    admin: &AdminCap
)
```

---

## Safety Mechanisms

**Authorization:**
- Only AdminCap holders can modify fee configuration
- Fee recipients must be set before fee withdrawals

**Economic Bounds:**
- Bonuses require sufficient fee treasury balance
- Operations revert if treasury cannot fund bonuses
- Fee rates bounded by reasonable operational limits

**Transparency:**
- All fee-related operations emit detailed events
- `FeeCharged` and `BonusPaid` events track all flows
- Mint/Redeem events include fee, bonus, and CR level data

---

## Integration Notes

**For Frontends:**
- Always check `current_level()` before displaying available actions
- Show estimated fees and bonuses before user confirmation
- Display current CR prominently to explain mode-based restrictions

**For Keepers/Bots:**
- Monitor CR levels for arbitrage opportunities in risk modes
- L2 mode creates profitable arbitrage for fToken redemption
- Keeper operations help restore protocol health while earning bonuses

**For Protocol Operators:**
- Monitor fee treasury balance to ensure sufficient bonus funding
- Adjust fee rates via governance if market conditions warrant
- Set up monitoring for CR threshold crossings

---

## Related Documentation

- **Implementation Details:** See `fee-implementation-notes.md` for technical implementation
- **Admin Operations:** See `admin_rebalancing_guide.md` for L3 rebalancing procedures
- **Frontend Integration:** See `frontend_integration_guide.md` for complete API reference

---

## Summary

The Leafsii protocol uses a sophisticated, CR-based fee policy that:
- Protects protocol solvency through operational restrictions in risk modes
- Incentivizes stabilizing behaviors via dynamic bonuses
- Creates natural market forces that restore healthy collateralization
- Maintains user freedom while guiding economically rational actions

This design ensures the protocol can self-stabilize through market mechanisms before requiring administrative intervention, creating a resilient and sustainable stablecoin system.
