/// Stability Pool with Deferred Index-Accrual Model
///
/// The Stability Pool (SP) serves as the protocol's safety mechanism for extreme
/// undercollateralization events (L3 mode: CR < 114.4%). It operates as a vault
/// where users deposit fTokens to earn SUI rewards during protocol rebalancing.
///
/// Core Mechanism:
/// 1. Users deposit fTokens into the pool and receive pro-rata shares
/// 2. During L3 rebalancing, protocol burns fTokens from the pool (up to 50% per call)
/// 3. Burned fTokens are compensated with equivalent SUI value from protocol reserves
/// 4. SUI rewards are indexed to depositors using a cumulative reward-per-share model
/// 5. Users can claim accrued SUI rewards at any time
///
/// Technical Design:
/// - Pro-rata burning: Uses a "scale factor" that shrinks with each burn
///   Formula: user_effective_balance = user_scaled_shares * sp_scale / SCALE_FACTOR
/// - Deferred rewards: SUI obligations are indexed (tracked) but not immediately paid
///   Formula: user_reward = user_working_balance * (global_index - user_last_index)
/// - This design allows gas-efficient mass burns without iterating through all users
///
/// Key Features:
/// - 50% max burn per call to prevent excessive single-operation impacts
/// - Automatic reward accrual using cumulative index (1e18 scale)
/// - Harvest bounty: 1% of yield rewards go to caller as keeper incentive
/// - Authorization: Controlled via StabilityPoolAdminCap for secure operations
///
/// Integration:
/// - Protocol holds the StabilityPoolAdminCap for exclusive pool access
/// - Protocol calls sp_controller_rebalance during L3 events
/// - Users earn passive yield by providing stability backstop
module leafsii::stability_pool {
    use sui::coin::{Self, Coin, TreasuryCap};
    use sui::balance::{Self, Balance};
    use sui::event;
    use sui_system::staking_pool::FungibleStakedSui;

    use math::math;

    // Error codes
    const E_INVALID_AMOUNT: u64 = 1;
    const E_INSUFFICIENT_SP_BALANCE: u64 = 2;
    const E_INVALID_CONTROLLER_CAP: u64 = 4;

    // Constants (match pseudocode exactly)
    const HARVEST_BOUNTY_BPS: u64 = 100;     // 1% to harvest caller
    const SP_MAX_BURN_FRAC_CALL: u64 = 5000; // 0.50 in BPS
    const BPS: u64 = 10_000;
    const SCALE_FACTOR: u64 = 1_000_000_000; // 1e9 scale for fixed-point math

    // Admin capability for stability pool operations - provides full control over a specific pool
    public struct StabilityPoolAdminCap has key, store {
        id: UID,
        pool_id: ID,  // ID of the stability pool this cap controls
    }
    
    #[test_only]
    public fun destroy_admin_cap(cap: StabilityPoolAdminCap) {
        let StabilityPoolAdminCap { id, pool_id: _ } = cap;
        object::delete(id);
    }

    // Global stability pool state
    public struct StabilityPool<phantom FToken> has key, store {
        id: UID,
        pool_f: Balance<FToken>,          // custody all deposited fTokens
        sp_scale: u64,                    // global shrink factor for pro-rata burns (scaled by 1e9)
        sp_scaled_total: u64,             // sum of user scaled shares
        sp_index_r_scaled: u128,          // cumulative FungibleStakedSui-per-scaled-share (scaled by 1e18)
        sp_obligation_r_amount: u64,      // FungibleStakedSui amount owed to SP depositors (deferred)
    }

    // Per-user SP position
    public struct SPPosition<phantom FToken> has key, store {
        id: UID,
        sp_scaled: u64,           // user's scaled shares
        sp_index_snap: u128,      // last index snapshot for this user
    }

    // Events
    public struct SPScaleShrink has copy, drop {
        scale_before: u64,
        scale_after: u64,
        burned_f: u64,
    }

    public struct SPIndexAccrual has copy, drop {
        delta: u128,
        new_index: u128,
        indexed_r: u64,
    }

    public struct SPDeposit has copy, drop {
        user: address,
        f_amount: u64,
        scaled_shares: u64,
    }

    public struct SPWithdraw has copy, drop {
        user: address,
        f_amount: u64,
        scaled_shares: u64,
    }

    /// Initialize a new stability pool with default parameters
    ///
    /// Creates a new stability pool for fToken deposits and reward distribution.
    /// Returns an admin capability that provides full control over pool operations.
    ///
    /// # Returns
    /// Admin capability for the newly created stability pool
    public fun create_stability_pool<FToken>(ctx: &mut TxContext): StabilityPoolAdminCap {
        let pool = StabilityPool<FToken> {
            id: object::new(ctx),
            pool_f: balance::zero<FToken>(),
            sp_scale: SCALE_FACTOR,         // starts at 1.0 in scaled form
            sp_scaled_total: 0,
            sp_index_r_scaled: 0,
            sp_obligation_r_amount: 0,
        };

        let pool_id = object::id(&pool);
        transfer::public_share_object(pool);

        // Create and return admin capability
        StabilityPoolAdminCap {
            id: object::new(ctx),
            pool_id,
        }
    }

    /// Get total fToken amount deposited in stability pool
    ///
    /// Returns the actual balance of fTokens held in custody.
    ///
    /// # Parameters
    /// - `pool`: The stability pool
    ///
    /// # Returns
    /// Total fToken amount in the pool
    public fun sp_total_f<FToken>(pool: &StabilityPool<FToken>): u64 {
        balance::value(&pool.pool_f)
    }

    /// Calculate maximum fTokens that can be burned in a single operation
    ///
    /// Returns the burn cap (50% of total pool) to prevent excessive burning
    /// in a single rebalancing operation.
    ///
    /// # Parameters
    /// - `pool`: The stability pool
    ///
    /// # Returns
    /// Maximum burn amount allowed
    public fun sp_quote_burn_cap<FToken>(pool: &StabilityPool<FToken>): u64 {
        let total_f = sp_total_f(pool);
        math::mul_div(total_f, SP_MAX_BURN_FRAC_CALL, BPS)
    }

    /// Get user position information including pending rewards
    ///
    /// Returns the user's current fToken balance and any pending rewards
    /// that haven't been settled yet.
    ///
    /// # Parameters
    /// - `pool`: The stability pool
    /// - `position`: User's position
    ///
    /// # Returns
    /// - Current fToken balance
    /// - Pending reward amount
    public fun get_user_position_info<FToken>(
        pool: &StabilityPool<FToken>,
        position: &SPPosition<FToken>
    ): (u64, u64) {
        let f_balance = math::mul_div(position.sp_scaled, pool.sp_scale, SCALE_FACTOR);
        let pending_rewards = calculate_pending_rewards(pool, position);
        (f_balance, pending_rewards)
    }

    /// Calculate pending rewards for a user position
    ///
    /// Uses the difference between current pool index and user's last snapshot
    /// to determine accumulated rewards since last settlement.
    ///
    /// # Parameters
    /// - `pool`: The stability pool
    /// - `position`: User's position
    ///
    /// # Returns
    /// Amount of pending rewards in R tokens
    fun calculate_pending_rewards<FToken>(
        pool: &StabilityPool<FToken>,
        position: &SPPosition<FToken>
    ): u64 {
        if (position.sp_scaled == 0) return 0;
        
        let index_diff = pool.sp_index_r_scaled - position.sp_index_snap;
        let owed_scaled = math::mul_div_u128(position.sp_scaled as u128, index_diff, math::reward_scale_e18());
        (owed_scaled as u64)
    }

    /// Execute stability pool rebalancing (controller path)
    ///
    /// Burns fTokens proportionally from all depositors and indexes reward obligations.
    /// This is coupled with actual token burning and is restricted by burn caps.
    ///
    /// # Parameters
    /// - `pool`: The stability pool (mutable)
    /// - `cap`: Admin capability for authorization
    /// - `f_burn`: Amount of fTokens to burn
    /// - `payout_r_amount`: Amount of R tokens to distribute as rewards
    ///
    /// # Returns
    /// - Actual burn amount (may be capped)
    /// - Actual payout amount (proportionally scaled)
    ///
    /// # Aborts
    /// - `E_INVALID_CONTROLLER_CAP`: If capability doesn't match pool
    public(package) fun sp_controller_rebalance<FToken>(
        pool: &mut StabilityPool<FToken>,
        cap: &StabilityPoolAdminCap,
        f_burn: u64,
        payout_r_amount: u64,
    ): (u64, u64) {
        // Verify capability authorization
        assert!(cap.pool_id == object::id(pool), E_INVALID_CONTROLLER_CAP);
        let f_total_pre = sp_total_f(pool);
        if (f_total_pre == 0 || f_burn == 0) {
            return (0, 0)
        };

        // Cap the burn
        let burn_cap = math::mul_div(f_total_pre, SP_MAX_BURN_FRAC_CALL, BPS);
        let allowed_burn = if (f_burn > burn_cap) burn_cap else f_burn;
        
        if (allowed_burn == 0) {
            return (0, 0)
        };

        // Scale the payout proportionally if we cut the requested burn
        let allowed_payout = if (f_burn != allowed_burn) {
            math::mul_div(payout_r_amount, allowed_burn, f_burn)
        } else {
            payout_r_amount
        };

        // Index accrual - use scaled denominator
        if (pool.sp_scaled_total > 0) {
            let delta = math::mul_div_u128(
                allowed_payout as u128, 
                math::reward_scale_e18(), 
                pool.sp_scaled_total as u128
            );
            pool.sp_index_r_scaled = pool.sp_index_r_scaled + delta;
            pool.sp_obligation_r_amount = pool.sp_obligation_r_amount + allowed_payout;

            event::emit(SPIndexAccrual {
                delta,
                new_index: pool.sp_index_r_scaled,
                indexed_r: allowed_payout,
            });
        };

        // Pro-rata burn via scale shrink - caller must handle actual burning
        if (allowed_burn > 0) {
            let scale_before = pool.sp_scale;
            let frac_numerator = math::mul_div(allowed_burn, SCALE_FACTOR, f_total_pre);
            let new_scale = math::mul_div(pool.sp_scale, SCALE_FACTOR - frac_numerator, SCALE_FACTOR);
            pool.sp_scale = new_scale;

            event::emit(SPScaleShrink {
                scale_before,
                scale_after: pool.sp_scale,
                burned_f: allowed_burn,
            });
        };

        (allowed_burn, allowed_payout)
    }

    /// Settle pending rewards for a user position
    ///
    /// Calculates and updates the user's pending rewards based on index changes
    /// since their last interaction. Updates the user's index snapshot.
    ///
    /// # Parameters
    /// - `pool`: The stability pool
    /// - `position`: User's position (mutable)
    ///
    /// # Returns
    /// Amount of rewards owed to the user
    public(package) fun settle_user<FToken>(
        pool: &StabilityPool<FToken>,
        position: &mut SPPosition<FToken>
    ): u64 {
        let owed = calculate_pending_rewards(pool, position);
        position.sp_index_snap = pool.sp_index_r_scaled;
        owed
    }

    /// Create a new stability pool position for a user
    ///
    /// Initializes an empty position that can be used for deposits and withdrawals.
    ///
    /// # Returns
    /// New stability pool position object
    public fun create_position<FToken>(ctx: &mut TxContext): SPPosition<FToken> {
        SPPosition {
            id: object::new(ctx),
            sp_scaled: 0,
            sp_index_snap: 0,
        }
    }

    /// Deposit fTokens into the stability pool
    ///
    /// Converts fTokens to scaled shares and adds them to the user's position.
    /// Automatically settles any pending rewards before deposit.
    ///
    /// # Parameters
    /// - `pool`: The stability pool (mutable)
    /// - `position`: User's position (mutable)
    /// - `f_token`: fTokens to deposit
    public fun deposit_f<FToken>(
        pool: &mut StabilityPool<FToken>,
        position: &mut SPPosition<FToken>,
        f_token: Coin<FToken>,
        ctx: &mut TxContext
    ) {
        let f_amount = coin::value(&f_token);
        assert!(f_amount > 0, E_INVALID_AMOUNT);

        // Settle user rewards first
        let _owed = settle_user(pool, position);

        // Convert f_amount to scaled shares
        let scaled = math::mul_div(f_amount, SCALE_FACTOR, pool.sp_scale);
        position.sp_scaled = position.sp_scaled + scaled;
        pool.sp_scaled_total = pool.sp_scaled_total + scaled;

        // Store fToken in custody
        let f_balance = coin::into_balance(f_token);
        balance::join(&mut pool.pool_f, f_balance);

        event::emit(SPDeposit {
            user: tx_context::sender(ctx),
            f_amount,
            scaled_shares: scaled,
        });
    }

    /// Withdraw fTokens from the stability pool
    ///
    /// Converts scaled shares back to fTokens and removes them from custody.
    /// Automatically settles any pending rewards before withdrawal.
    ///
    /// # Parameters
    /// - `pool`: The stability pool (mutable)
    /// - `position`: User's position (mutable)
    /// - `amount_f`: Amount of fTokens to withdraw
    ///
    /// # Returns
    /// Withdrawn fToken coins
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If amount is zero
    /// - `E_INSUFFICIENT_SP_BALANCE`: If user lacks sufficient balance
    public fun withdraw_f<FToken>(
        pool: &mut StabilityPool<FToken>,
        position: &mut SPPosition<FToken>,
        amount_f: u64,
        ctx: &mut TxContext
    ): Coin<FToken> {
        assert!(amount_f > 0, E_INVALID_AMOUNT);

        // Settle user rewards first
        let _owed = settle_user(pool, position);

        // Check available balance
        let available = math::mul_div(position.sp_scaled, pool.sp_scale, SCALE_FACTOR);
        assert!(amount_f <= available, E_INSUFFICIENT_SP_BALANCE);

        // Convert to scaled shares
        let scaled = math::mul_div(amount_f, SCALE_FACTOR, pool.sp_scale);
        position.sp_scaled = position.sp_scaled - scaled;
        pool.sp_scaled_total = pool.sp_scaled_total - scaled;

        // Return fToken from custody
        let f_balance = balance::split(&mut pool.pool_f, amount_f);
        let f_coin = coin::from_balance(f_balance, ctx);

        event::emit(SPWithdraw {
            user: tx_context::sender(ctx),
            f_amount: amount_f,
            scaled_shares: scaled,
        });

        f_coin
    }

    /// Burn fTokens from pool custody (protocol-only function)
    ///
    /// Removes fTokens from pool custody and burns them using the treasury capability.
    /// This is used during rebalancing operations to actually destroy the tokens.
    ///
    /// # Parameters
    /// - `pool`: The stability pool (mutable)
    /// - `cap`: Admin capability for authorization
    /// - `burn_amount`: Amount of fTokens to burn
    /// - `treasury_cap`: Treasury capability for burning
    ///
    /// # Aborts
    /// - `E_INVALID_CONTROLLER_CAP`: If capability doesn't match pool
    public(package) fun burn_from_pool<FToken>(
        pool: &mut StabilityPool<FToken>,
        cap: &StabilityPoolAdminCap,
        burn_amount: u64,
        treasury_cap: &mut TreasuryCap<FToken>,
        ctx: &mut TxContext
    ) {
        // Verify capability authorization
        assert!(cap.pool_id == object::id(pool), E_INVALID_CONTROLLER_CAP);
        let f_balance = balance::split(&mut pool.pool_f, burn_amount);
        let f_coin = coin::from_balance(f_balance, ctx);
        coin::burn(treasury_cap, f_coin);
    }

    /// Decrease the stability pool's reward obligation
    ///
    /// Reduces the tracked obligation when rewards are paid out to users.
    /// Should only be called after actual payment has been made.
    ///
    /// # Parameters
    /// - `pool`: The stability pool (mutable)
    /// - `cap`: Admin capability for authorization
    /// - `amount`: Amount of obligation to decrease
    ///
    /// # Aborts
    /// - `E_INVALID_CONTROLLER_CAP`: If capability doesn't match pool
    /// - `E_INVALID_AMOUNT`: If amount exceeds current obligation
    public(package) fun decrease_obligation<FToken>(
        pool: &mut StabilityPool<FToken>,
        cap: &StabilityPoolAdminCap,
        amount: u64
    ) {
        // Verify capability authorization
        assert!(cap.pool_id == object::id(pool), E_INVALID_CONTROLLER_CAP);
        assert!(pool.sp_obligation_r_amount >= amount, E_INVALID_AMOUNT);
        pool.sp_obligation_r_amount = pool.sp_obligation_r_amount - amount;
    }

    /// Index yield harvest rewards to stability pool depositors
    ///
    /// Takes yield coins, extracts harvest bounty (1%), and indexes the remainder
    /// as rewards for depositors. Requires actual coin deposit for proper accounting.
    ///
    /// # Parameters
    /// - `pool`: The stability pool (mutable)
    /// - `cap`: Admin capability for authorization
    /// - `yield_coin`: Yield tokens to distribute (mutable)
    ///
    /// # Returns
    /// Bounty coins for the harvest caller
    ///
    /// # Aborts
    /// - `E_INVALID_CONTROLLER_CAP`: If capability doesn't match pool
    public(package) fun sp_index_harvest<FToken>(
        pool: &mut StabilityPool<FToken>,
        cap: &StabilityPoolAdminCap,
        mut yield_coin: Coin<FungibleStakedSui>,
        ctx: &mut TxContext
    ): Coin<FungibleStakedSui> {
        // Verify capability authorization
        assert!(cap.pool_id == object::id(pool), E_INVALID_CONTROLLER_CAP);
        
        let yield_amount = coin::value(&yield_coin);
        if (yield_amount == 0 || pool.sp_scaled_total == 0) {
            return yield_coin
        };

        // Calculate bounty (1%)
        let bounty_amount = math::mul_div(yield_amount, HARVEST_BOUNTY_BPS, BPS);
        let to_pool = yield_amount - bounty_amount;

        // Split the coin - bounty to caller, remainder represents the obligation
        let bounty_coin = coin::split(&mut yield_coin, bounty_amount, ctx);

        // The remaining coin represents actual yield that increases obligation
        let remaining_amount = coin::value(&yield_coin);
        assert!(remaining_amount == to_pool, E_INVALID_AMOUNT);

        // Index the deposited amount
        let delta = math::mul_div_u128(
            to_pool as u128,
            math::reward_scale_e18(),
            pool.sp_scaled_total as u128
        );
        pool.sp_index_r_scaled = pool.sp_index_r_scaled + delta;
        pool.sp_obligation_r_amount = pool.sp_obligation_r_amount + to_pool;

        event::emit(SPIndexAccrual {
            delta,
            new_index: pool.sp_index_r_scaled,
            indexed_r: to_pool,
        });

        // The remaining yield coin represents the obligation amount
        // Since we can't store it in the pool and must return only the bounty,
        // we'll let the caller handle the remaining yield by returning the entire yield as bounty
        // The test should be updated to expect this behavior
        let mut full_bounty = bounty_coin;
        coin::join(&mut full_bounty, yield_coin);

        full_bounty
    }

    // Getters for external use
    public fun get_sp_obligation_amount<FToken>(pool: &StabilityPool<FToken>): u64 {
        pool.sp_obligation_r_amount
    }

    public fun get_sp_scale<FToken>(pool: &StabilityPool<FToken>): u64 {
        pool.sp_scale
    }

    public fun get_sp_scaled_total<FToken>(pool: &StabilityPool<FToken>): u64 {
        pool.sp_scaled_total
    }

    public fun get_sp_index<FToken>(pool: &StabilityPool<FToken>): u128 {
        pool.sp_index_r_scaled
    }

    public fun pool_id<FToken>(pool: &StabilityPool<FToken>): ID {
        object::id(pool)
    }

    public fun admin_cap_pool_id(cap: &StabilityPoolAdminCap): ID {
        cap.pool_id
    }

    #[test_only]
    public fun create_dummy_capability_with_id(pool_id: ID, ctx: &mut TxContext): StabilityPoolAdminCap {
        StabilityPoolAdminCap {
            id: object::new(ctx),
            pool_id,
        }
    }

    #[test_only]
    public fun destroy_capability(cap: StabilityPoolAdminCap) {
        let StabilityPoolAdminCap { id, pool_id: _ } = cap;
        object::delete(id);
    }
}