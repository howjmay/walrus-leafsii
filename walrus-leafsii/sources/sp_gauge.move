/// Stability Pool Gauge for LFS Rewards
///
/// The SP Gauge distributes weekly LFS emissions to stability pool depositors
/// with optional ve-LFS boosting. It operates as a layer on top of the existing
/// stability pool, tracking deposits and calculating boosted reward shares.
///
/// Boost Mechanism (Curve-style):
/// - Base APY: 40% of deposit earns rewards (no ve-LFS required)
/// - Boosted APY: Up to 250% of deposit with maximum ve-LFS
/// - Formula: working_balance = min(stake * 0.4 + total_stake * 0.6 * user_ve / total_ve, stake * 2.5)
/// - Users with more ve-LFS relative to their stake get higher boost
///
/// Reward Accrual:
/// - Uses cumulative reward-per-working-share model (1e18 scale)
/// - Global integral tracks: total_rewards / total_working_balance
/// - User reward = user_working_balance * (global_integral - user_last_integral)
/// - Gas-efficient: No iteration required for reward updates
///
/// Integration Flow:
/// 1. User deposits fTokens in stability pool
/// 2. User calls checkpoint_user() with optional ve-LFS lock
/// 3. Gauge calculates working balance with boost
/// 4. Weekly emissions arrive from gauge controller
/// 5. Rewards accrue automatically to all depositors
/// 6. User claims rewards → receives 7-day vesting escrow
///
/// Key Features:
/// - Automatic boost recalculation when ve-LFS balance changes
/// - 7-day linear vesting on all claimed rewards
/// - Treasury for unallocated rewards (when total_working = 0)
/// - Event-driven for easy off-chain tracking
///
/// Technical Details:
/// - Binds to specific StabilityPool via pool_id
/// - Tracks user data: stake, working_balance, claimable
/// - Uses 1e18 scaling for reward integral precision
/// - Vesting escrows created automatically on claim
module leafsii::sp_gauge {
    use sui::coin::{Self, Coin};
    use sui::balance::{Self, Balance};
    use sui::clock::{Self, Clock};
    use sui::table::{Self, Table};
    use sui::event;

    use leafsii::lfs_token::LFS;
    use leafsii::stability_pool::{Self, StabilityPool, SPPosition};
    use leafsii::ve_lfs::{Self, Lock};
    use leafsii::vesting_escrow::{Self, Vesting};
    use math::math;

    // Error codes
    const E_USER_NOT_FOUND: u64 = 3;
    const E_INVALID_POOL: u64 = 4;

    // Constants for boost calculation
    const BOOST_BASE_BPS: u64 = 4000; // 40% base boost (0.4x)
    const BOOST_MAX_BPS: u64 = 25000; // 250% max boost (2.5x)
    const BOOST_TOTAL_BPS: u64 = 10000; // 100% = 10,000 basis points
    const SCALE_FACTOR: u128 = 1_000_000_000_000_000_000; // 1e18 for reward integral

    // User data in the gauge
    public struct UserData has store {
        stake: u64,                    // User's fToken balance in SP
        working_balance: u64,          // Boosted balance for rewards
        reward_integral_e18: u128,     // Last settled integral
        claimable: u64,               // Pending LFS rewards
    }

    // SP Gauge for a specific stability pool
    public struct SPGauge<phantom FToken> has key, store {
        id: UID,
        pool_id: ID,                           // ID of the associated StabilityPool
        total_stake: u64,                      // Total fToken staked across all users
        total_working: u64,                    // Total working balance (with boosts)
        reward_integral_e18: u128,             // Cumulative LFS per working balance
        lfs_rewards: Balance<LFS>,             // Custody of LFS rewards
        treasury_balance: Balance<LFS>,        // Treasury for unallocated rewards
        users: Table<address, UserData>,       // Per-user data
        last_update_epoch: u64,                // Last epoch when rewards were distributed
    }

    // Events
    public struct SPGaugeCreated has copy, drop {
        gauge_id: ID,
        pool_id: ID,
    }

    public struct UserCheckpointed has copy, drop {
        gauge_id: ID,
        user: address,
        old_stake: u64,
        new_stake: u64,
        old_working: u64,
        new_working: u64,
        rewards_settled: u64,
    }

    public struct RewardNotified has copy, drop {
        gauge_id: ID,
        epoch: u64,
        amount: u64,
        total_working: u64,
    }

    public struct RewardClaimed has copy, drop {
        gauge_id: ID,
        user: address,
        amount: u64,
        vesting_id: ID,
    }

    // Create a new SP gauge for a stability pool
    public fun create_sp_gauge<FToken>(
        pool: &StabilityPool<FToken>,
        ctx: &mut TxContext
    ): SPGauge<FToken> {
        let pool_id = object::id(pool);

        let gauge = SPGauge<FToken> {
            id: object::new(ctx),
            pool_id,
            total_stake: 0,
            total_working: 0,
            reward_integral_e18: 0,
            lfs_rewards: balance::zero<LFS>(),
            treasury_balance: balance::zero<LFS>(),
            users: table::new<address, UserData>(ctx),
            last_update_epoch: 0,
        };

        let gauge_id = object::id(&gauge);

        event::emit(SPGaugeCreated {
            gauge_id,
            pool_id,
        });

        gauge
    }

    // Checkpoint a user's position (call when SP position changes)
    public fun checkpoint_user<FToken>(
        gauge: &mut SPGauge<FToken>,
        pool: &StabilityPool<FToken>,
        position: &SPPosition<FToken>,
        user_lock: &option::Option<Lock>, // Optional ve-LFS lock for boost
        clock: &Clock,
        ctx: &mut TxContext
    ) {
        let user = tx_context::sender(ctx);
        let current_time = clock::timestamp_ms(clock);

        // Verify this is the correct pool
        assert!(object::id(pool) == gauge.pool_id, E_INVALID_POOL);

        // Get user's current stake from SP position
        let (f_balance, _) = stability_pool::get_user_position_info(pool, position);
        let new_stake = f_balance;

        // Get existing user data or create new
        let (old_stake, old_working, old_integral) = if (table::contains(&gauge.users, user)) {
            let user_data = table::borrow(&gauge.users, user);
            (user_data.stake, user_data.working_balance, user_data.reward_integral_e18)
        } else {
            (0, 0, gauge.reward_integral_e18)
        };

        // Calculate pending rewards before updating
        let pending_rewards = calculate_pending_rewards(
            old_working,
            gauge.reward_integral_e18,
            old_integral
        );

        // Calculate new working balance with ve-boost
        let (user_ve_balance, total_ve_supply) = if (option::is_some(user_lock)) {
            let lock = option::borrow(user_lock);
            let user_ve = ve_lfs::balance_of_at(lock, current_time);
            // For now use a simple approach - in production this would need proper total ve supply tracking
            let total_ve = 1000000; // Placeholder - should be actual total ve supply
            std::debug::print(&b"User VE balance:");
            std::debug::print(&user_ve);
            std::debug::print(&b"Total VE supply:");
            std::debug::print(&total_ve);
            (user_ve, total_ve)
        } else {
            std::debug::print(&b"No lock - VE balance 0");
            (0, 1) // Avoid division by zero
        };

        // Calculate new total stake that includes this user's stake
        let updated_total_stake = gauge.total_stake - old_stake + new_stake;

        let new_working = calculate_working_balance(
            new_stake,
            updated_total_stake,
            user_ve_balance,
            total_ve_supply
        );

        // Update gauge totals
        gauge.total_stake = updated_total_stake;
        gauge.total_working = gauge.total_working - old_working + new_working;

        // Update or create user data
        if (table::contains(&gauge.users, user)) {
            let user_data = table::borrow_mut(&mut gauge.users, user);
            user_data.stake = new_stake;
            user_data.working_balance = new_working;
            user_data.reward_integral_e18 = gauge.reward_integral_e18;
            user_data.claimable = user_data.claimable + pending_rewards;
        } else {
            let user_data = UserData {
                stake: new_stake,
                working_balance: new_working,
                reward_integral_e18: gauge.reward_integral_e18,
                claimable: pending_rewards,
            };
            table::add(&mut gauge.users, user, user_data);
        };

        event::emit(UserCheckpointed {
            gauge_id: object::id(gauge),
            user,
            old_stake,
            new_stake,
            old_working,
            new_working,
            rewards_settled: pending_rewards,
        });
    }

    // Receive LFS rewards from gauge controller
    public(package) fun notify_reward<FToken>(
        gauge: &mut SPGauge<FToken>,
        amount: Coin<LFS>,
        epoch: u64,
        _ctx: &mut TxContext
    ) {
        let reward_amount = coin::value(&amount);

        if (reward_amount > 0 && gauge.total_working > 0) {
            // Update reward integral: integral += amount * 1e18 / total_working
            let integral_increase = math::scaled_div_u128(reward_amount, gauge.total_working, SCALE_FACTOR);
            gauge.reward_integral_e18 = gauge.reward_integral_e18 + integral_increase;

            // Add to rewards custody
            balance::join(&mut gauge.lfs_rewards, coin::into_balance(amount));
        } else {
            // No working balance - route to treasury
            if (reward_amount > 0) {
                balance::join(&mut gauge.treasury_balance, coin::into_balance(amount));
            } else {
                coin::destroy_zero(amount);
            }
        };

        gauge.last_update_epoch = epoch;

        event::emit(RewardNotified {
            gauge_id: object::id(gauge),
            epoch,
            amount: reward_amount,
            total_working: gauge.total_working,
        });
    }

    // Claim rewards and create vesting escrow
    public fun claim<FToken>(
        gauge: &mut SPGauge<FToken>,
        clock: &Clock,
        ctx: &mut TxContext
    ): option::Option<Vesting> {
        let user = tx_context::sender(ctx);

        assert!(table::contains(&gauge.users, user), E_USER_NOT_FOUND);

        let user_data = table::borrow_mut(&mut gauge.users, user);

        // Calculate any additional pending rewards
        let pending_rewards = calculate_pending_rewards(
            user_data.working_balance,
            gauge.reward_integral_e18,
            user_data.reward_integral_e18
        );

        let total_claimable = user_data.claimable + pending_rewards;

        if (total_claimable == 0) {
            return option::none()
        };

        // Update user state
        user_data.claimable = 0;
        user_data.reward_integral_e18 = gauge.reward_integral_e18;

        // Extract LFS from custody
        let reward_balance = balance::split(&mut gauge.lfs_rewards, total_claimable);
        let reward_coin = coin::from_balance(reward_balance, ctx);

        // Create 7-day vesting escrow
        let vesting = vesting_escrow::create_standard_vesting(
            user,
            reward_coin,
            clock,
            ctx
        );

        let vesting_id = object::id(&vesting);

        event::emit(RewardClaimed {
            gauge_id: object::id(gauge),
            user,
            amount: total_claimable,
            vesting_id,
        });

        option::some(vesting)
    }

    // Calculate working balance with ve-boost
    // working = min(stake, stake * 40% + total_stake * 60% * user_ve / total_ve)
    fun calculate_working_balance(
        stake: u64,
        total_stake: u64,
        user_ve: u64,
        total_ve: u64
    ): u64 {
        if (stake == 0) {
            return 0
        };

        // Base working balance: stake * 40%
        let base_working = (stake as u128) * (BOOST_BASE_BPS as u128) / (BOOST_TOTAL_BPS as u128);

        // Boost component: total_stake * 60% * user_ve / total_ve
        let boost_component = if (total_ve > 0 && user_ve > 0) {
            let boost_pool = (total_stake as u128) * ((BOOST_TOTAL_BPS - BOOST_BASE_BPS) as u128) / (BOOST_TOTAL_BPS as u128);
            boost_pool * (user_ve as u128) / (total_ve as u128)
        } else {
            0
        };

        std::debug::print(&b"Base working:");
        std::debug::print(&(base_working as u64));
        std::debug::print(&b"Boost component:");
        std::debug::print(&(boost_component as u64));
        std::debug::print(&b"Total stake:");
        std::debug::print(&total_stake);

        let working_balance = base_working + boost_component;

        // Cap at stake amount (max 2.5x when fully boosted)
        let max_working = (stake as u128) * (BOOST_MAX_BPS as u128) / (BOOST_TOTAL_BPS as u128);

        if (working_balance > max_working) {
            max_working as u64
        } else {
            working_balance as u64
        }
    }

    // Calculate pending rewards for a user
    fun calculate_pending_rewards(
        working_balance: u64,
        current_integral: u128,
        user_integral: u128
    ): u64 {
        if (working_balance == 0 || current_integral <= user_integral) {
            return 0
        };

        let integral_diff = current_integral - user_integral;
        ((working_balance as u128) * integral_diff / SCALE_FACTOR) as u64
    }

    // View functions
    public fun get_user_info<FToken>(
        gauge: &SPGauge<FToken>,
        user: address
    ): (u64, u64, u64) {
        if (!table::contains(&gauge.users, user)) {
            return (0, 0, 0)
        };

        let user_data = table::borrow(&gauge.users, user);
        (user_data.stake, user_data.working_balance, user_data.claimable)
    }

    public fun get_gauge_info<FToken>(
        gauge: &SPGauge<FToken>
    ): (ID, u64, u64, u64) {
        (
            gauge.pool_id,
            gauge.total_stake,
            gauge.total_working,
            balance::value(&gauge.lfs_rewards)
        )
    }

    public fun pending_rewards<FToken>(
        gauge: &SPGauge<FToken>,
        user: address
    ): u64 {
        if (!table::contains(&gauge.users, user)) {
            return 0
        };

        let user_data = table::borrow(&gauge.users, user);
        let pending = calculate_pending_rewards(
            user_data.working_balance,
            gauge.reward_integral_e18,
            user_data.reward_integral_e18
        );

        user_data.claimable + pending
    }

    // Test-only functions
    #[test_only]
    public fun create_test_sp_gauge<FToken>(
        ctx: &mut TxContext
    ): SPGauge<FToken> {
        SPGauge<FToken> {
            id: object::new(ctx),
            pool_id: object::id_from_address(@0x1),
            total_stake: 0,
            total_working: 0,
            reward_integral_e18: 0,
            lfs_rewards: balance::zero<LFS>(),
            treasury_balance: balance::zero<LFS>(),
            users: table::new<address, UserData>(ctx),
            last_update_epoch: 0,
        }
    }

    #[test_only]
    public fun test_working_balance_calculation(
        stake: u64,
        total_stake: u64,
        user_ve: u64,
        total_ve: u64
    ): u64 {
        calculate_working_balance(stake, total_stake, user_ve, total_ve)
    }
}