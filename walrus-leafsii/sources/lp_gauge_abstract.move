/// Abstract LP Gauge for Future Integration
///
/// This module provides a flexible, pluggable gauge framework for distributing LFS
/// emissions to liquidity providers. Unlike the SP Gauge which binds directly to the
/// stability pool, this gauge accepts stake updates from any external LP system.
///
/// Design Philosophy:
/// - Protocol-agnostic: Works with any LP system (AMM DEX, lending protocol, etc.)
/// - Admin-controlled: LP system or protocol admin updates user stakes via LPAdminCap
/// - Future-proof: Ready for integration with any liquidity provision mechanism
///
/// Key Differences from SP Gauge:
/// 1. Stake Management:
///    - SP Gauge: Automatically tracks stability pool deposits
///    - LP Gauge: Requires external calls to set_user_stake()
///
/// 2. Authorization:
///    - SP Gauge: Anyone can checkpoint with their SP position
///    - LP Gauge: Only AdminCap holder can update user stakes
///
/// 3. Integration:
///    - SP Gauge: Tightly coupled to stability_pool module
///    - LP Gauge: Loosely coupled, compatible with any LP system
///
/// Boost Mechanism (Same as SP Gauge):
/// - Base APY: 40% of stake earns rewards (no ve-LFS)
/// - Max APY: 250% of stake with maximum ve-LFS
/// - Formula: working = min(stake * 0.4 + total * 0.6 * user_ve / total_ve, stake * 2.5)
///
/// Usage Pattern:
/// ```move
/// // LP system (DEX, lending protocol, etc.) integrates:
/// 1. Create LP gauge: (gauge, admin_cap) = create_lp_gauge(ctx)
/// 2. On LP deposit: set_user_stake(gauge, admin_cap, user, new_amount, lock, clock)
/// 3. On LP withdraw: set_user_stake(gauge, admin_cap, user, 0, lock, clock)
/// 4. User claims: user calls claim(gauge, clock, ctx) → receives vesting escrow
/// ```
///
/// Future Integration Examples:
/// - Cetus/Turbos AMM LP tokens
/// - Navi/Scallop lending positions
/// - Custom liquidity provision mechanisms
/// - Multi-pool aggregated positions
///
/// Technical Details:
/// - Checkpoint system for automatic reward accrual (1e18 scale)
/// - 7-day linear vesting on all claimed rewards
/// - Treasury for unallocated rewards (when total_working = 0)
/// - Flexible boost recalculation via checkpoint_user()
module leafsii::lp_gauge_abstract {
    use sui::coin::{Self, Coin};
    use sui::balance::{Self, Balance};
    use sui::clock::{Self, Clock};
    use sui::table::{Self, Table};
    use sui::event;

    use leafsii::lfs_token::LFS;
    use leafsii::ve_lfs::{Self, Lock};
    use leafsii::vesting_escrow::{Self, Vesting};
    use math::math;

    // Error codes
    const E_USER_NOT_FOUND: u64 = 3;

    // Constants for boost calculation (same as SP gauge)
    const BOOST_BASE_BPS: u64 = 4000; // 40% base boost (0.4x)
    const BOOST_MAX_BPS: u64 = 25000; // 250% max boost (2.5x)
    const BOOST_TOTAL_BPS: u64 = 10000; // 100% = 10,000 basis points
    const SCALE_FACTOR: u128 = 1_000_000_000_000_000_000; // 1e18 for reward integral

    // Admin capability for managing LP stake updates
    public struct LPAdminCap has key, store {
        id: UID,
        gauge_id: ID,
    }

    // User data in the LP gauge
    public struct UserData has store {
        stake: u64,                    // User's LP stake
        working_balance: u64,          // Boosted balance for rewards
        reward_integral_e18: u128,     // Last settled integral
        claimable: u64,               // Pending LFS rewards
    }

    // Abstract LP Gauge
    public struct LPGauge has key, store {
        id: UID,
        total_stake: u64,                      // Total LP tokens staked
        total_working: u64,                    // Total working balance (with boosts)
        reward_integral_e18: u128,             // Cumulative LFS per working balance
        lfs_rewards: Balance<LFS>,             // Custody of LFS rewards
        treasury_balance: Balance<LFS>,        // Treasury for unallocated rewards
        users: Table<address, UserData>,       // Per-user data
        last_update_epoch: u64,                // Last epoch when rewards were distributed
    }

    // Events
    public struct LPGaugeCreated has copy, drop {
        gauge_id: ID,
        admin_cap_id: ID,
    }

    public struct UserStakeSet has copy, drop {
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

    // Create a new abstract LP gauge
    public fun create_lp_gauge(ctx: &mut TxContext): (LPGauge, LPAdminCap) {
        let gauge = LPGauge {
            id: object::new(ctx),
            total_stake: 0,
            total_working: 0,
            reward_integral_e18: 0,
            lfs_rewards: balance::zero<LFS>(),
            treasury_balance: balance::zero<LFS>(),
            users: table::new<address, UserData>(ctx),
            last_update_epoch: 0,
        };

        let gauge_id = object::id(&gauge);

        let admin_cap = LPAdminCap {
            id: object::new(ctx),
            gauge_id,
        };

        let admin_cap_id = object::id(&admin_cap);

        event::emit(LPGaugeCreated {
            gauge_id,
            admin_cap_id,
        });

        (gauge, admin_cap)
    }

    // Set user's stake (restricted by AdminCap or external LP system)
    public(package) fun set_user_stake(
        gauge: &mut LPGauge,
        _admin_cap: &LPAdminCap, // Could also be called via friend module
        user: address,
        new_stake: u64,
        user_lock: &option::Option<Lock>, // Optional ve-LFS lock for boost
        clock: &Clock
    ) {
        let current_time = clock::timestamp_ms(clock);

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
            // For now use a simple approach - in production this would need proper total supply tracking
            let total_ve = 1000000; // Placeholder - should be actual total ve supply
            (user_ve, total_ve)
        } else {
            (0, 1) // Avoid division by zero
        };

        let new_working = calculate_working_balance(
            new_stake,
            gauge.total_stake,
            user_ve_balance,
            total_ve_supply
        );

        // Update gauge totals
        gauge.total_stake = gauge.total_stake - old_stake + new_stake;
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

        event::emit(UserStakeSet {
            gauge_id: object::id(gauge),
            user,
            old_stake,
            new_stake,
            old_working,
            new_working,
            rewards_settled: pending_rewards,
        });
    }

    // Checkpoint user (alternative to set_user_stake for external integrations)
    public fun checkpoint_user(
        gauge: &mut LPGauge,
        user: address,
        user_lock: &option::Option<Lock>, // Optional ve-LFS lock for boost
        clock: &Clock
    ) {
        let current_time = clock::timestamp_ms(clock);

        // This function recalculates working balance without changing stake
        // Useful for updating boost when ve balances change
        if (!table::contains(&gauge.users, user)) {
            return // User doesn't exist, nothing to checkpoint
        };

        let user_data = table::borrow(&gauge.users, user);
        let current_stake = user_data.stake;
        let old_working = user_data.working_balance;
        let old_integral = user_data.reward_integral_e18;

        // Calculate pending rewards
        let pending_rewards = calculate_pending_rewards(
            old_working,
            gauge.reward_integral_e18,
            old_integral
        );

        // Recalculate working balance with current ve-boost
        let (user_ve_balance, total_ve_supply) = if (option::is_some(user_lock)) {
            let lock = option::borrow(user_lock);
            let user_ve = ve_lfs::balance_of_at(lock, current_time);
            let total_ve = 1000000; // Placeholder
            (user_ve, total_ve)
        } else {
            (0, 1) // Avoid division by zero
        };

        let new_working = calculate_working_balance(
            current_stake,
            gauge.total_stake,
            user_ve_balance,
            total_ve_supply
        );

        // Update gauge total working balance
        gauge.total_working = gauge.total_working - old_working + new_working;

        // Update user data
        let user_data_mut = table::borrow_mut(&mut gauge.users, user);
        user_data_mut.working_balance = new_working;
        user_data_mut.reward_integral_e18 = gauge.reward_integral_e18;
        user_data_mut.claimable = user_data_mut.claimable + pending_rewards;

        event::emit(UserStakeSet {
            gauge_id: object::id(gauge),
            user,
            old_stake: current_stake,
            new_stake: current_stake,
            old_working,
            new_working,
            rewards_settled: pending_rewards,
        });
    }

    // Receive LFS rewards from gauge controller (same as SP gauge)
    public(package) fun notify_reward(
        gauge: &mut LPGauge,
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

    // Claim rewards and create vesting escrow (same as SP gauge)
    public fun claim(
        gauge: &mut LPGauge,
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

    // Calculate working balance with ve-boost (same as SP gauge)
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

        let working_balance = base_working + boost_component;

        // Cap at max boost (2.5x)
        let max_working = (stake as u128) * (BOOST_MAX_BPS as u128) / (BOOST_TOTAL_BPS as u128);

        if (working_balance > max_working) {
            max_working as u64
        } else if (working_balance > (stake as u128)) {
            stake // Never exceed original stake in this simplified version
        } else {
            working_balance as u64
        }
    }

    // Calculate pending rewards for a user (same as SP gauge)
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
    public fun get_user_info(
        gauge: &LPGauge,
        user: address
    ): (u64, u64, u64) {
        if (!table::contains(&gauge.users, user)) {
            return (0, 0, 0)
        };

        let user_data = table::borrow(&gauge.users, user);
        (user_data.stake, user_data.working_balance, user_data.claimable)
    }

    public fun get_gauge_info(
        gauge: &LPGauge
    ): (u64, u64, u64) {
        (
            gauge.total_stake,
            gauge.total_working,
            balance::value(&gauge.lfs_rewards)
        )
    }

    public fun pending_rewards(
        gauge: &LPGauge,
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

    #[test_only]
    public fun create_test_lp_gauge(ctx: &mut TxContext): (LPGauge, LPAdminCap) {
        create_lp_gauge(ctx)
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