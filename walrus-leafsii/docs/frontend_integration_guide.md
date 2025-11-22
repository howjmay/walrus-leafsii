# Leafsii Protocol - Frontend Integration Guide

This comprehensive guide details all necessary functions for frontend integration with the Leafsii protocol, including detailed function descriptions, user operation flows, and implementation examples.

## Table of Contents
- [Core Protocol Functions](#core-protocol-functions)
- [Stability Pool Functions](#stability-pool-functions)
- [User Operation Flows](#user-operation-flows)
- [Protocol Modes and Restrictions](#protocol-modes-and-restrictions)
- [Implementation Examples](#implementation-examples)
- [Error Handling](#error-handling)
- [State Management](#state-management)

## Core Protocol Functions

### Minting Operations

#### `mint_f` - Mint Stable Tokens (fTokens)
```move
public fun mint_f<CoinTypeF, CoinTypeX, CoinTypeR>(
    protocol: &mut Protocol<CoinTypeF, CoinTypeX, CoinTypeR>,
    pool: &StabilityPool<CoinTypeR, CoinTypeF>,
    reserve_in: Coin<CoinTypeR>,
    ctx: &mut TxContext
): Coin<CoinTypeF>
```

**Description:** Mints stable tokens (fTokens) with fixed $1.00 value in exchange for reserve tokens. Only available in Normal mode (CR ≥ 130.6%).

**Parameters:**
- `protocol`: Mutable reference to the protocol instance
- `pool`: Reference to the authorized stability pool for CR calculations
- `reserve_in`: Reserve tokens to deposit (e.g., SUI coins)
- `ctx`: Transaction context

**Returns:** Newly minted fToken coins

**Fee Structure:**
- Normal mode: 0.5% fee on deposit amount
- L1+ modes: Blocked (throws E_ACTION_BLOCKED_BY_CR)

**Example Usage:**
```typescript
// Frontend flow:
1. Check current_level() == 0 (Normal mode)
2. Get current reserve price from protocol state
3. Calculate expected fTokens: (reserve_amount * price) / $1.00
4. Account for 0.5% fee
5. Execute mint_f transaction
```

#### `mint_x` - Mint Leverage Tokens (xTokens)
```move
public fun mint_x<CoinTypeF, CoinTypeX, CoinTypeR>(
    protocol: &mut Protocol<CoinTypeF, CoinTypeX, CoinTypeR>,
    pool: &StabilityPool<CoinTypeR, CoinTypeF>,
    reserve_in: Coin<CoinTypeR>,
    ctx: &mut TxContext
): Coin<CoinTypeX>
```

**Description:** Mints leverage tokens (xTokens) that capture upside/downside of reserve price movements. Available in all protocol modes with stability bonuses in risk modes.

**Parameters:**
- `protocol`: Mutable reference to the protocol instance
- `pool`: Reference to the authorized stability pool
- `reserve_in`: Reserve tokens to deposit
- `ctx`: Transaction context

**Returns:** Newly minted xToken coins

**Fee Structure:**
- All modes: 0.5% fee on deposit amount
- L1+ modes: Additional 0.1% stability bonus (paid from fee treasury)

**Dynamic Pricing:** xToken price (px) is calculated from protocol invariant:
```
px = (Total Reserve USD - fToken USD) / xToken Supply
```

### Redemption Operations

#### `redeem_f` - Redeem Stable Tokens
```move
public fun redeem_f<CoinTypeF, CoinTypeX, CoinTypeR>(
    protocol: &mut Protocol<CoinTypeF, CoinTypeX, CoinTypeR>,
    pool: &StabilityPool<CoinTypeR, CoinTypeF>,
    f_in: Coin<CoinTypeF>,
    ctx: &mut TxContext
): Coin<CoinTypeR>
```

**Description:** Burns fTokens and returns equivalent reserve value. Provides stability bonuses in L2+ modes to incentivize fToken burning when protocol is undercollateralized.

**Fee Structure:**
- Normal mode: 0.5% fee
- L1 mode: 0% fee (no bonus)
- L2+ modes: 0% fee + 0.1% stability bonus

#### `redeem_x` - Redeem Leverage Tokens
```move
public fun redeem_x<CoinTypeF, CoinTypeX, CoinTypeR>(
    protocol: &mut Protocol<CoinTypeF, CoinTypeX, CoinTypeR>,
    pool: &StabilityPool<CoinTypeR, CoinTypeF>,
    x_in: Coin<CoinTypeX>,
    ctx: &mut TxContext
): Coin<CoinTypeR>
```

**Description:** Burns xTokens and returns current market value in reserves. Fees vary by operational level to manage protocol risk.

**Fee Structure:**
- Normal/L2/L3 modes: 0.5% fee
- L1 mode: 1.0% fee (higher to discourage xToken redemption)

### State Query Functions

#### `get_user_position_info` - Enhanced Position Information
```move
public fun get_user_position_info<R, FToken>(
    pool: &StabilityPool<R, FToken>,
    position: &SPPosition<R, FToken>
): (u64, u64)
```

**Description:** Returns comprehensive information about a user's stability pool position, including current fToken balance and accumulated rewards ready for claiming.

**Parameters:**
- `pool`: Reference to the stability pool
- `position`: Reference to the user's position object

**Returns:**
- `u64`: Current fToken balance in the stability pool (after any burns from rebalancing)
- `u64`: Pending reward amount in reserve tokens that can be claimed

**Key Features:**
- Accounts for pro-rata burns from protocol rebalancing
- Calculates rewards based on index differential since last settlement
- Real-time calculation without state changes

**Frontend Integration:**
```typescript
// Example implementation
interface UserPosition {
  fTokenBalance: number;      // Current SP balance (may decrease from burns)
  pendingRewards: number;     // Claimable rewards
  lastUpdateTime: number;     // For UI freshness
}

async function getUserPositionInfo(poolId: string, positionId: string): Promise<UserPosition> {
  const [fBalance, pendingRewards] = await contract.get_user_position_info(poolId, positionId);
  return {
    fTokenBalance: fBalance / 1e6,  // Convert from micro units
    pendingRewards: pendingRewards / 1e6,
    lastUpdateTime: Date.now()
  };
}
```

#### `collateral_ratio` - Protocol Health Metric
```move
public fun collateral_ratio<CoinTypeF, CoinTypeX, CoinTypeR>(
    protocol: &Protocol<CoinTypeF, CoinTypeX, CoinTypeR>,
    pool: &StabilityPool<CoinTypeR, FToken>
): u64
```

**Description:** Calculates the current collateral ratio of the protocol, which determines operational mode and available user actions.

**Formula:** `CR = (Net Reserve USD Value) / (Total fToken USD Value)`

**Returns:** Collateral ratio scaled by 1e6 (e.g., 1.5 = 1,500,000)

#### `current_level` - Operational Mode
```move
public fun current_level<CoinTypeF, CoinTypeX, CoinTypeR>(
    protocol: &Protocol<CoinTypeF, CoinTypeX, CoinTypeR>,
    pool: &StabilityPool<CoinTypeR, FToken>
): u8
```

**Description:** Determines the current operational level based on collateral ratio thresholds.

**Returns:**
- `0`: Normal mode (CR ≥ 130.6%) - Full functionality
- `1`: L1 Stability mode (120.6% ≤ CR < 130.6%) - fToken minting blocked
- `2`: L2 User Rebalance mode (114.4% ≤ CR < 120.6%) - Stability bonuses active
- `3`: L3 Protocol Rebalance mode (CR < 114.4%) - Automatic rebalancing active

## Stability Pool Functions

### Position Management

#### `create_position` - Initialize User Position
```move
public fun create_position<R, FToken>(ctx: &mut TxContext): SPPosition<R, FToken>
```

**Description:** Creates a new stability pool position for a user. This is a one-time setup required before any stability pool operations.

**Returns:** New stability pool position object that the user owns

**Important:** Each user needs exactly one position object per stability pool.

#### `deposit_f` - Deposit to Stability Pool
```move
public fun deposit_f<R, FToken>(
    pool: &mut StabilityPool<R, FToken>,
    position: &mut SPPosition<R, FToken>,
    f_token: Coin<FToken>,
    ctx: &mut TxContext
)
```

**Description:** Deposits fTokens into the stability pool to earn rewards from protocol rebalancing. Automatically settles any pending rewards before deposit.

**Risk/Reward Profile:**
- **Rewards:** Share in SUI payments from protocol rebalancing operations
- **Risk:** Pro-rata burn of deposits during L3 rebalancing (fTokens may decrease)
- **Benefit:** Helps stabilize the protocol while earning yield

#### `withdraw_f` - Withdraw from Stability Pool
```move
public fun withdraw_f<R, FToken>(
    pool: &mut StabilityPool<R, FToken>,
    position: &mut SPPosition<R, FToken>,
    amount_f: u64,
    ctx: &mut TxContext
): Coin<FToken>
```

**Description:** Withdraws fTokens from the stability pool. Available balance may be less than originally deposited due to protocol rebalancing burns.

### Reward Operations

#### `claim_sp_rewards` - Claim Stability Pool Rewards
```move
public fun claim_sp_rewards<CoinTypeF, CoinTypeX, CoinTypeR>(
    protocol: &mut Protocol<CoinTypeF, CoinTypeX, CoinTypeR>,
    pool: &mut StabilityPool<CoinTypeR, CoinTypeF>,
    pool_admin_cap: &stability_pool::StabilityPoolAdminCap,
    position: &mut SPPosition<CoinTypeR, CoinTypeF>,
    ctx: &mut TxContext
): Coin<CoinTypeR>
```

**Description:** Claims accumulated rewards from stability pool participation. Rewards come from protocol rebalancing operations where fTokens are burned and equivalent reserve value is distributed to depositors.

**Note:** This function requires protocol admin capabilities in the current implementation. Frontend may need to use a different claiming mechanism or the protocol may provide a user-callable wrapper.

## User Operation Flows

### Complete User Journey

#### 1. Initial Setup
```typescript
// One-time setup per user
const position = await stability_pool.create_position();
// Store position ID for future operations
```

#### 2. Normal Trading Flow
```typescript
// Check protocol mode
const level = await protocol.current_level();
const cr = await protocol.collateral_ratio();

// Mint fTokens (only in Normal mode)
if (level === 0) {
  const fTokens = await protocol.mint_f(reserveTokens);
}

// Mint xTokens (always available)
const xTokens = await protocol.mint_x(reserveTokens);

// Redeem operations (always available, fees vary)
const reserves1 = await protocol.redeem_f(fTokens);
const reserves2 = await protocol.redeem_x(xTokens);
```

#### 3. Stability Pool Participation
```typescript
// Deposit fTokens to earn rewards
await stability_pool.deposit_f(pool, position, fTokens);

// Monitor position
const [balance, pendingRewards] = await stability_pool.get_user_position_info(pool, position);

// Withdraw when needed
const withdrawnFTokens = await stability_pool.withdraw_f(pool, position, amount);

// Claim rewards
const rewards = await protocol.claim_sp_rewards(protocol, pool, poolAdminCap, position);
```

## Protocol Modes and Restrictions

### Mode-Based Feature Matrix

| Feature | Normal (0) | L1 (1) | L2 (2) | L3 (3) |
|---------|------------|--------|--------|--------|
| **Mint fTokens** | ✅ 0.5% fee | ❌ Blocked | ❌ Blocked | ❌ Blocked |
| **Mint xTokens** | ✅ 0.5% fee | ✅ 0.5% fee + 0.1% bonus | ✅ 0.5% fee + 0.1% bonus | ✅ 0.5% fee + 0.1% bonus |
| **Redeem fTokens** | ✅ 0.5% fee | ✅ 0% fee | ✅ 0% fee + 0.1% bonus | ✅ 0% fee + 0.1% bonus |
| **Redeem xTokens** | ✅ 0.5% fee | ✅ 1.0% fee | ✅ 0.5% fee | ✅ 0.5% fee |
| **SP Operations** | ✅ Full | ✅ Full | ✅ Full | ✅ Full + Auto Rebalance |

### Dynamic Incentive System

The protocol uses dynamic fees and bonuses to encourage stabilizing behaviors:

**Risk Modes (L1+):**
- Block fToken minting to prevent further leverage
- Provide xToken minting bonuses to increase collateral
- Provide fToken redemption bonuses to reduce liability
- Increase xToken redemption fees in L1 to discourage collateral reduction

## Implementation Examples

### React/TypeScript Integration

```typescript
interface ProtocolState {
  level: number;
  collateralRatio: number;
  fTokenPrice: number;
  xTokenPrice: number;
  reservePrice: number;
}

interface UserBalances {
  reserveTokens: number;
  fTokens: number;
  xTokens: number;
  spBalance: number;
  pendingRewards: number;
}

class LeafsiiProtocol {
  async getProtocolState(): Promise<ProtocolState> {
    const [nf, nx, pf, px, pr, reserve, fees, active] = await this.contract.get_protocol_state();
    const level = await this.contract.current_level();
    const cr = await this.contract.collateral_ratio();

    return {
      level,
      collateralRatio: cr / 1e6,
      fTokenPrice: pf / 1e6,
      xTokenPrice: px / 1e6,
      reservePrice: pr / 1e6
    };
  }

  async getUserBalances(userAddress: string, positionId: string): Promise<UserBalances> {
    // Get wallet balances
    const reserveTokens = await this.getTokenBalance(userAddress, 'RESERVE');
    const fTokens = await this.getTokenBalance(userAddress, 'FTOKEN');
    const xTokens = await this.getTokenBalance(userAddress, 'XTOKEN');

    // Get stability pool position
    const [spBalance, pendingRewards] = await this.contract.get_user_position_info(positionId);

    return {
      reserveTokens: reserveTokens / 1e6,
      fTokens: fTokens / 1e6,
      xTokens: xTokens / 1e6,
      spBalance: spBalance / 1e6,
      pendingRewards: pendingRewards / 1e6
    };
  }

  async mintFTokens(amount: number): Promise<TransactionResult> {
    const state = await this.getProtocolState();
    if (state.level !== 0) {
      throw new Error('fToken minting only available in Normal mode');
    }

    const fee = amount * 0.005; // 0.5% fee
    const expectedFTokens = (amount - fee) * state.reservePrice; // $1 per fToken

    return await this.contract.mint_f(amount * 1e6);
  }

  async mintXTokens(amount: number): Promise<TransactionResult> {
    const state = await this.getProtocolState();
    const fee = amount * 0.005; // 0.5% fee
    const bonus = state.level >= 1 ? amount * 0.001 : 0; // 0.1% bonus in risk modes
    const netAmount = amount - fee + bonus;
    const expectedXTokens = (netAmount * state.reservePrice) / state.xTokenPrice;

    return await this.contract.mint_x(amount * 1e6);
  }
}
```

### Vue.js Composition API Example

```typescript
import { ref, computed, onMounted } from 'vue';

export function useLeafsiiProtocol() {
  const protocolState = ref<ProtocolState | null>(null);
  const userBalances = ref<UserBalances | null>(null);
  const loading = ref(false);
  const error = ref<string | null>(null);

  const protocolMode = computed(() => {
    if (!protocolState.value) return null;
    const modes = ['Normal', 'L1 Stability', 'L2 User Rebalance', 'L3 Protocol Rebalance'];
    return modes[protocolState.value.level];
  });

  const availableActions = computed(() => {
    if (!protocolState.value) return [];
    const level = protocolState.value.level;

    return {
      mintF: level === 0,
      mintX: true,
      redeemF: true,
      redeemX: true,
      stabilityPool: true
    };
  });

  async function refreshData() {
    loading.value = true;
    try {
      protocolState.value = await protocol.getProtocolState();
      userBalances.value = await protocol.getUserBalances(userAddress, positionId);
      error.value = null;
    } catch (e) {
      error.value = e.message;
    } finally {
      loading.value = false;
    }
  }

  onMounted(() => {
    refreshData();
    // Set up polling for real-time updates
    setInterval(refreshData, 10000); // Update every 10 seconds
  });

  return {
    protocolState,
    userBalances,
    protocolMode,
    availableActions,
    loading,
    error,
    refreshData
  };
}
```

## Error Handling

### Common Error Codes

```typescript
enum LeafsiiErrors {
  E_INVALID_AMOUNT = 1,
  E_INSUFFICIENT_RESERVE = 2,
  E_MINT_BLOCKED = 3,
  E_ORACLE_STALE = 4,
  E_ORACLE_STEP_TOO_LARGE = 5,
  E_ACTION_BLOCKED_BY_CR = 6,
  E_INVALID_ADMIN = 7,
  E_UNAUTHORIZED_POOL = 8,
  E_INSUFFICIENT_SP_BALANCE = 2, // From stability pool
  E_UNAUTHORIZED_CONTROLLER = 3,
  E_INVALID_CONTROLLER_CAP = 4
}

function handleLeafsiiError(error: any): string {
  const errorCode = error.code || error.error_code;

  switch (errorCode) {
    case LeafsiiErrors.E_ACTION_BLOCKED_BY_CR:
      return "This action is not available in the current protocol mode. Check collateral ratio.";
    case LeafsiiErrors.E_INSUFFICIENT_RESERVE:
      return "Insufficient protocol reserves to complete this operation.";
    case LeafsiiErrors.E_INSUFFICIENT_SP_BALANCE:
      return "Insufficient stability pool balance for withdrawal.";
    case LeafsiiErrors.E_MINT_BLOCKED:
      return "User actions are currently disabled by protocol admin.";
    case LeafsiiErrors.E_ORACLE_STALE:
      return "Oracle price is stale. Please wait for price update.";
    default:
      return `Protocol error: ${error.message || 'Unknown error'}`;
  }
}
```

### User-Friendly Error Messages

```typescript
const ErrorMessages = {
  FTOKEN_MINT_BLOCKED: "fToken minting is only available when the protocol is in Normal mode (CR ≥ 130.6%). Current mode: {mode}",
  INSUFFICIENT_COLLATERAL: "Protocol collateral ratio is too low for this operation. Current CR: {cr}%",
  SP_BALANCE_TOO_LOW: "You don't have enough fTokens in the stability pool. Available: {available}, Requested: {requested}",
  ORACLE_OUTDATED: "Price data is outdated. The protocol will update prices automatically when new data is available.",
  SLIPPAGE_HIGH: "Price has moved significantly. Expected: {expected}, Actual: {actual}. Please retry.",
  BURN_CAP_EXCEEDED: "Operation exceeds maximum burn limit per transaction. Maximum: {max}, Requested: {requested}"
};
```

## State Management

### Real-time Updates

```typescript
interface ProtocolSubscription {
  onPriceUpdate: (newPrice: number) => void;
  onModeChange: (newLevel: number) => void;
  onRebalance: (burnAmount: number, rewards: number) => void;
  onUserAction: (action: string, amount: number) => void;
}

class ProtocolStateManager {
  private subscriptions: ProtocolSubscription[] = [];
  private eventStream: EventSource;

  constructor(private protocolAddress: string) {
    this.setupEventStream();
  }

  private setupEventStream() {
    // Listen to blockchain events
    this.eventStream = new EventSource(`/api/events/${this.protocolAddress}`);

    this.eventStream.addEventListener('PriceUpdate', (event) => {
      const data = JSON.parse(event.data);
      this.notifySubscribers('onPriceUpdate', data.new_price / 1e6);
    });

    this.eventStream.addEventListener('SPIndexAccrual', (event) => {
      const data = JSON.parse(event.data);
      this.notifySubscribers('onRebalance', data.burned_f, data.indexed_r);
    });
  }

  subscribe(subscription: ProtocolSubscription) {
    this.subscriptions.push(subscription);
  }

  private notifySubscribers(method: keyof ProtocolSubscription, ...args: any[]) {
    this.subscriptions.forEach(sub => {
      const callback = sub[method];
      if (callback) callback(...args);
    });
  }
}
```

### Optimistic Updates

```typescript
class OptimisticStateManager {
  private pendingTransactions = new Map<string, PendingTransaction>();

  async executeMintF(amount: number): Promise<string> {
    const txId = generateTxId();
    const expectedFTokens = this.calculateExpectedMint(amount);

    // Optimistic update
    this.pendingTransactions.set(txId, {
      type: 'mint_f',
      amount,
      expectedResult: expectedFTokens,
      timestamp: Date.now()
    });

    try {
      const result = await this.protocol.mint_f(amount);
      this.pendingTransactions.delete(txId);
      return result;
    } catch (error) {
      this.pendingTransactions.delete(txId);
      throw error;
    }
  }

  getPendingBalance(tokenType: string): number {
    let pending = 0;
    for (const [_, tx] of this.pendingTransactions) {
      if (tx.type.includes(tokenType)) {
        pending += tx.expectedResult;
      }
    }
    return pending;
  }
}
```

This comprehensive guide provides everything needed for frontend integration with the Leafsii protocol, including detailed function descriptions, complete user flows, error handling strategies, and practical implementation examples.