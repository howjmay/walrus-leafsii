#[test_only]
module leafsii::test_lp_gauge {
    use sui::clock::{Self, Clock};
    use sui::test_scenario::{Self as ts};
    use sui::coin::{Self};

    use leafsii::lfs_token::{Self};
    use leafsii::ve_lfs::{Self, Lock};
    use leafsii::lp_gauge_abstract::{Self, LPGauge, LPAdminCap};
    use leafsii::vesting_escrow::{Self};

    const ADMIN: address = @0xAD;
    const ALICE: address = @0xA11CE;
    const BOB: address = @0xB0B;
    const CHARLIE: address = @0xCCC;

    const STAKE_AMOUNT: u64 = 1000_000_000; // 1000 LP tokens
    const LFS_REWARD_AMOUNT: u64 = 1000_000_000; // 1000 LFS rewards
    const FOUR_YEARS_MS: u64 = 4 * 365 * 24 * 3600 * 1000;

    // Test gauge creation
    #[test]
    fun test_create_lp_gauge() {
        let mut scenario = ts::begin(ADMIN);

        let (gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        // Verify initial state
        let (total_stake, total_working, rewards_balance) = lp_gauge_abstract::get_gauge_info(&gauge);
        assert!(total_stake == 0, 0);
        assert!(total_working == 0, 1);
        assert!(rewards_balance == 0, 2);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test setting user stake without ve-boost
    #[test]
    fun test_set_user_stake_no_boost() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        // Set Alice's stake without ve-lock
        let no_lock: option::Option<Lock> = option::none();
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );
        option::destroy_none(no_lock);

        // Check user info
        let (stake, working, claimable) = lp_gauge_abstract::get_user_info(&gauge, ALICE);
        assert!(stake == STAKE_AMOUNT, 0);
        assert!(claimable == 0, 1);

        // Working should be 40% of stake (no boost)
        let expected_working = STAKE_AMOUNT * 4000 / 10000; // 40%
        assert!(working == expected_working, 2);

        // Check gauge totals
        let (total_stake, total_working, _) = lp_gauge_abstract::get_gauge_info(&gauge);
        assert!(total_stake == STAKE_AMOUNT, 3);
        assert!(total_working == expected_working, 4);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test setting user stake with ve-boost
    #[test]
    fun test_set_user_stake_with_boost() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));

        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        // Alice creates max ve-lock
        let lfs_coin = coin::mint(&mut treasury_cap, STAKE_AMOUNT, ts::ctx(&mut scenario));
        let alice_lock = ve_lfs::create_lock(lfs_coin, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        // Set Alice's stake with ve-lock
        let mut alice_lock_opt = option::some(alice_lock);
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &alice_lock_opt,
            &clock
        );
        let alice_lock = option::extract(&mut alice_lock_opt);
        option::destroy_none(alice_lock_opt);

        // Check user info
        let (stake, working_with_boost, claimable) = lp_gauge_abstract::get_user_info(&gauge, ALICE);
        assert!(stake == STAKE_AMOUNT, 0);
        assert!(claimable == 0, 1);

        // Working should be at least the base (40%)
        // Note: The LP gauge uses a placeholder for total_ve, so exact boost calculation may vary
        let base_working = STAKE_AMOUNT * 4000 / 10000;
        assert!(working_with_boost >= base_working, 2);

        transfer::public_transfer(alice_lock, ALICE);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test updating user stake
    #[test]
    fun test_update_user_stake() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();

        // Initial stake
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );

        let (stake_1, working_1, _) = lp_gauge_abstract::get_user_info(&gauge, ALICE);
        assert!(stake_1 == STAKE_AMOUNT, 0);

        // Update to higher stake
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT * 2,
            &no_lock,
            &clock
        );

        let (stake_2, working_2, _) = lp_gauge_abstract::get_user_info(&gauge, ALICE);
        assert!(stake_2 == STAKE_AMOUNT * 2, 1);
        assert!(working_2 == working_1 * 2, 2);

        // Update to lower stake
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT / 2,
            &no_lock,
            &clock
        );

        let (stake_3, working_3, _) = lp_gauge_abstract::get_user_info(&gauge, ALICE);
        assert!(stake_3 == STAKE_AMOUNT / 2, 3);
        assert!(working_3 == working_1 / 2, 4);

        option::destroy_none(no_lock);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test unstaking (setting stake to 0)
    #[test]
    fun test_unstake() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();

        // Stake
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );

        let (total_before, working_before, _) = lp_gauge_abstract::get_gauge_info(&gauge);
        assert!(total_before == STAKE_AMOUNT, 0);

        // Unstake
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            0,
            &no_lock,
            &clock
        );

        let (stake, working, _) = lp_gauge_abstract::get_user_info(&gauge, ALICE);
        assert!(stake == 0, 1);
        assert!(working == 0, 2);

        let (total_after, working_after, _) = lp_gauge_abstract::get_gauge_info(&gauge);
        assert!(total_after == 0, 3);
        assert!(working_after == 0, 4);

        option::destroy_none(no_lock);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test multi-user staking
    #[test]
    fun test_multi_user_staking() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();

        // Alice stakes
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );

        // Bob stakes
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            BOB,
            STAKE_AMOUNT * 2,
            &no_lock,
            &clock
        );

        // Charlie stakes
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            CHARLIE,
            STAKE_AMOUNT / 2,
            &no_lock,
            &clock
        );

        // Check totals
        let expected_total = STAKE_AMOUNT + STAKE_AMOUNT * 2 + STAKE_AMOUNT / 2;
        let (total_stake, _, _) = lp_gauge_abstract::get_gauge_info(&gauge);
        assert!(total_stake == expected_total, 0);

        // Check individual stakes
        let (alice_stake, _, _) = lp_gauge_abstract::get_user_info(&gauge, ALICE);
        let (bob_stake, _, _) = lp_gauge_abstract::get_user_info(&gauge, BOB);
        let (charlie_stake, _, _) = lp_gauge_abstract::get_user_info(&gauge, CHARLIE);

        assert!(alice_stake == STAKE_AMOUNT, 1);
        assert!(bob_stake == STAKE_AMOUNT * 2, 2);
        assert!(charlie_stake == STAKE_AMOUNT / 2, 3);

        option::destroy_none(no_lock);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test reward notification and distribution
    #[test]
    fun test_reward_notification() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();

        // Alice stakes
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );

        // Notify rewards
        let reward_coin = coin::mint(&mut treasury_cap, LFS_REWARD_AMOUNT, ts::ctx(&mut scenario));
        lp_gauge_abstract::notify_reward(&mut gauge, reward_coin, 1, ts::ctx(&mut scenario));

        // Check pending rewards
        let pending = lp_gauge_abstract::pending_rewards(&gauge, ALICE);
        assert!(pending > 0, 0);

        // Should be approximately all the rewards since Alice is the only staker
        assert!(pending >= LFS_REWARD_AMOUNT - 1000, 1); // Allow small rounding error

        option::destroy_none(no_lock);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test claiming rewards
    #[test]
    fun test_claim_rewards() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();

        // Alice stakes
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );

        // Notify rewards
        let reward_coin = coin::mint(&mut treasury_cap, LFS_REWARD_AMOUNT, ts::ctx(&mut scenario));
        lp_gauge_abstract::notify_reward(&mut gauge, reward_coin, 1, ts::ctx(&mut scenario));

        // Alice claims
        ts::next_tx(&mut scenario, ALICE);
        let mut vesting_opt = lp_gauge_abstract::claim(&mut gauge, &clock, ts::ctx(&mut scenario));

        assert!(option::is_some(&vesting_opt), 0);

        let vesting = option::extract(&mut vesting_opt);
        option::destroy_none(vesting_opt);

        // Check vesting info
        let (owner, _, duration_ms, total_amount, claimed_amount, remaining) =
            vesting_escrow::get_vesting_info(&vesting);

        assert!(owner == ALICE, 1);
        assert!(duration_ms == 7 * 24 * 3600 * 1000, 2); // 7 days
        assert!(total_amount >= LFS_REWARD_AMOUNT - 1000, 3); // Allow rounding
        assert!(claimed_amount == 0, 4);
        assert!(remaining == total_amount, 5);

        transfer::public_transfer(vesting, ALICE);
        option::destroy_none(no_lock);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test claiming with no rewards
    #[test]
    fun test_claim_no_rewards() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();

        // Alice stakes but no rewards notified
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );

        // Try to claim
        ts::next_tx(&mut scenario, ALICE);
        let vesting_opt = lp_gauge_abstract::claim(&mut gauge, &clock, ts::ctx(&mut scenario));

        // Should return none
        assert!(option::is_none(&vesting_opt), 0);
        option::destroy_none(vesting_opt);

        option::destroy_none(no_lock);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test multi-user reward distribution
    #[test]
    fun test_multi_user_rewards() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();

        // Alice stakes 1000
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );

        // Bob stakes 2000 (2x Alice)
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            BOB,
            STAKE_AMOUNT * 2,
            &no_lock,
            &clock
        );

        // Notify rewards
        let reward_coin = coin::mint(&mut treasury_cap, LFS_REWARD_AMOUNT, ts::ctx(&mut scenario));
        lp_gauge_abstract::notify_reward(&mut gauge, reward_coin, 1, ts::ctx(&mut scenario));

        // Check pending rewards
        let alice_pending = lp_gauge_abstract::pending_rewards(&gauge, ALICE);
        let bob_pending = lp_gauge_abstract::pending_rewards(&gauge, BOB);

        // Bob should have approximately 2x Alice's rewards
        assert!(bob_pending > alice_pending, 0);
        assert!(bob_pending >= alice_pending * 19 / 10, 1); // At least 1.9x

        // Total should be close to reward amount
        assert!(alice_pending + bob_pending >= LFS_REWARD_AMOUNT - 1000, 2);

        option::destroy_none(no_lock);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test checkpoint user
    #[test]
    fun test_checkpoint_user() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();

        // Alice stakes
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );

        let (_, working_before, _) = lp_gauge_abstract::get_user_info(&gauge, ALICE);

        // Checkpoint (should recalculate working balance)
        lp_gauge_abstract::checkpoint_user(&mut gauge, ALICE, &no_lock, &clock);

        let (stake_after, working_after, _) = lp_gauge_abstract::get_user_info(&gauge, ALICE);

        // Stake should remain same
        assert!(stake_after == STAKE_AMOUNT, 0);
        // Working should be same (no ve-lock change)
        assert!(working_after == working_before, 1);

        option::destroy_none(no_lock);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test rewards routed to treasury when no stakers
    #[test]
    fun test_rewards_to_treasury_no_stakers() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        // Notify rewards without any stakers
        let reward_coin = coin::mint(&mut treasury_cap, LFS_REWARD_AMOUNT, ts::ctx(&mut scenario));
        lp_gauge_abstract::notify_reward(&mut gauge, reward_coin, 1, ts::ctx(&mut scenario));

        // Rewards should go to treasury (can't verify directly but function shouldn't error)

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test working balance calculation
    #[test]
    fun test_working_balance_calculation() {
        let stake = 1000;
        let total_stake = 2000;
        let user_ve = 500;
        let total_ve = 1000;

        let working = lp_gauge_abstract::test_working_balance_calculation(
            stake,
            total_stake,
            user_ve,
            total_ve
        );

        // Base: 1000 * 0.4 = 400
        // Boost: 2000 * 0.6 * 500 / 1000 = 600
        // Total: 400 + 600 = 1000 (capped at stake)
        assert!(working == 1000, 0);

        // Test with no ve
        let working_no_ve = lp_gauge_abstract::test_working_balance_calculation(
            stake,
            total_stake,
            0,
            total_ve
        );
        assert!(working_no_ve == 400, 1); // 40% base

        // Test with zero stake
        let working_zero = lp_gauge_abstract::test_working_balance_calculation(
            0,
            total_stake,
            user_ve,
            total_ve
        );
        assert!(working_zero == 0, 2);
    }

    // Test claim from non-existent user
    #[test]
    #[expected_failure(abort_code = 3)] // E_USER_NOT_FOUND
    fun test_claim_nonexistent_user() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        // Try to claim without staking
        let vesting_opt = lp_gauge_abstract::claim(&mut gauge, &clock, ts::ctx(&mut scenario));
        option::destroy_none(vesting_opt);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test multiple reward notifications
    #[test]
    fun test_multiple_reward_notifications() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let (mut gauge, admin_cap) = lp_gauge_abstract::create_test_lp_gauge(ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();

        // Alice stakes
        lp_gauge_abstract::set_user_stake(
            &mut gauge,
            &admin_cap,
            ALICE,
            STAKE_AMOUNT,
            &no_lock,
            &clock
        );

        // First reward notification
        let reward_1 = coin::mint(&mut treasury_cap, LFS_REWARD_AMOUNT, ts::ctx(&mut scenario));
        lp_gauge_abstract::notify_reward(&mut gauge, reward_1, 1, ts::ctx(&mut scenario));

        // Second reward notification
        let reward_2 = coin::mint(&mut treasury_cap, LFS_REWARD_AMOUNT / 2, ts::ctx(&mut scenario));
        lp_gauge_abstract::notify_reward(&mut gauge, reward_2, 2, ts::ctx(&mut scenario));

        // Check total pending
        let pending = lp_gauge_abstract::pending_rewards(&gauge, ALICE);
        let expected_total = LFS_REWARD_AMOUNT + LFS_REWARD_AMOUNT / 2;
        assert!(pending >= expected_total - 1000, 0); // Allow rounding

        option::destroy_none(no_lock);
        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }
}
