# Fee Policy Implementation Notes

## Overview

This document describes the implementation of the CR-based fee policy in the Leafsii protocol, following the specification in `docs/fee-policy.md`.

## CR Level Mapping

The code now uses a 4-level CR system aligned with the documentation:

- **Level 0 (Normal Mode)**: CR ≥ 130.6%
- **Level 1 (L1 - Stability Mode)**: 120.6% ≤ CR < 130.6%  
- **Level 2 (L2 - User Rebalance Mode)**: 114.4% ≤ CR < 120.6%
- **Level 3 (L3 - Protocol Rebalance Mode)**: CR < 114.4%

## Fee Policy Implementation

### Normal Mode (Level 0)
- **fToken mint**: Standard fee (default 0.5%)
- **xToken mint**: Standard fee (default 0.5%)
- **fToken redeem**: Standard fee (default 0.5%)
- **xToken redeem**: Standard fee (default 0.5%)
- **Bonuses**: None

### L1 - Stability Mode (Level 1)
- **fToken mint**: **DISABLED** (will revert with `E_ACTION_BLOCKED_BY_CR`)
- **xToken mint**: Standard fee + **stability bonus** (default 0.1%)
- **fToken redeem**: **0% fee**
- **xToken redeem**: **Increased fee** (default 1.0%)
- **Bonuses**: xToken minters earn stability bonus

### L2 - User Rebalance Mode (Level 2)
- **fToken mint**: **DISABLED** (will revert with `E_ACTION_BLOCKED_BY_CR`)
- **xToken mint**: Standard fee + **stability bonus** (default 0.1%)
- **fToken redeem**: **0% fee** + **stability bonus** (users get >NAV)
- **xToken redeem**: Standard fee
- **Bonuses**: Both fToken redeemers and xToken minters earn bonuses

### L3 - Protocol Rebalance Mode (Level 3)
- **User actions**: Same as L2
- **Protocol actions**: Protocol can perform rebalancing and earn fees
- **Bonuses**: Same as L2 for users, protocol earns rebalancing fees

## Technical Implementation

### Fee Calculation
- All fees are calculated using basis points (10,000 = 100%)
- Fee calculation: `amount * fee_bps / 10000`
- Bonus calculation: `amount * bonus_bps / 10000`

### Fee Storage
- Fees are stored in `protocol.fee_treasury_balance: Balance<CoinTypeR>`
- Configuration is stored in `protocol.fee_config: FeeConfig`
- Admin can withdraw fees and update configuration

### Rounding
- **Fee deduction**: Rounded against user (fees rounded up)
- **Bonus payments**: Rounded in favor of user (bonuses rounded down)
- **Net amounts**: User receives/pays the net amount after fee/bonus calculation

### Event Emission
The implementation emits comprehensive events:
- `MintF`, `MintX`, `RedeemF`, `RedeemX`: Include fee and bonus amounts, CR level
- `FeeCharged`: Detailed fee information
- `BonusPaid`: Detailed bonus information

## Admin Functions

### Fee Configuration
```move
public fun set_fee_config(
    protocol: &mut Protocol<CoinTypeF, CoinTypeX, CoinTypeR>,
    normal_mint_f_fee_bps: u64,
    normal_mint_x_fee_bps: u64,
    normal_redeem_f_fee_bps: u64,
    normal_redeem_x_fee_bps: u64,
    l1_redeem_x_fee_bps: u64,
    stability_bonus_rate_bps: u64,
    _admin: &AdminCap
)
```

### Fee Management
```move
public fun set_fee_recipient(protocol, recipient, admin)
public fun withdraw_fees(protocol, amount, admin, ctx): Coin<CoinTypeR>
```

## Safety Features

### Access Control
- Only `AdminCap` holders can modify fee configuration
- Fee rates are configurable but bounded by reasonable limits in practice

### Economic Safety
- Bonuses are funded from the protocol's fee treasury
- Protocol checks sufficient balance before paying bonuses
- Fee deduction happens before reserve addition to prevent manipulation

### State Consistency
- All operations maintain protocol invariants
- Fee accounting is atomic with the underlying operations
- CR level checks are performed at transaction time

## Testing

Comprehensive test suite covers:
- Normal mode fee collection
- L1 stability mode (fToken mint blocking, increased xToken redeem fees, xToken mint bonuses)
- L2 user rebalance mode (fToken redeem bonuses, continued xToken mint bonuses)
- Fee configuration updates
- Fee calculation accuracy
- Boundary condition testing

## Example Fee Flows

### fToken Mint in Normal Mode
1. User provides 1000 reserve tokens
2. Fee calculated: 1000 * 50 / 10000 = 5 tokens
3. Fee deposited to fee treasury: 5 tokens  
4. Net reserve added: 995 tokens
5. fTokens minted based on net reserve value

### xToken Mint in L1 Mode
1. User provides 1000 reserve tokens
2. Fee calculated: 1000 * 50 / 10000 = 5 tokens
3. Bonus calculated: 1000 * 10 / 10000 = 1 token
4. Fee deposited to treasury: 5 tokens
5. Bonus funded from treasury: 1 token  
6. Net reserve for minting: 1000 - 5 + 1 = 996 tokens
7. xTokens minted based on net value

### fToken Redeem in L2 Mode  
1. User redeems 100 fTokens
2. Base reserve calculated: 100 USD worth
3. Fee: 0% (no fee in L2)
4. Bonus: 100 * 10 / 10000 = 0.1 USD worth
5. User receives base + bonus reserve tokens

## Configuration Recommendations

### Default Fee Rates (in basis points)
- Normal mint/redeem fees: 50 bps (0.5%)
- L1 xToken redeem fee: 100 bps (1.0%) 
- Stability bonus rate: 10 bps (0.1%)

### Operational Parameters
- Fee treasury should be seeded with initial funds to pay early bonuses
- Fee recipient should be set to protocol treasury or governance contract
- Monitor fee treasury balance to ensure sufficient funds for bonuses

## Edge Cases Handled

1. **Insufficient fee treasury**: Bonus payments fail gracefully if treasury insufficient
2. **Zero amounts**: Fee calculations handle zero amounts correctly
3. **CR boundary transitions**: Level checks performed at operation time
4. **Rounding precision**: Consistent rounding prevents dust accumulation

## Future Enhancements

Potential improvements for future versions:
1. **Dynamic fee rates**: Fees that adjust based on utilization or market conditions
2. **Fee sharing**: Revenue sharing with token holders or liquidity providers  
3. **Gas optimization**: Batch fee operations for improved efficiency
4. **Advanced bonuses**: More sophisticated bonus curves based on CR distance from target