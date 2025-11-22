#[test_only]
module leafsii::test_ve_locking {
    use sui::clock;
    use sui::test_scenario::{Self as ts};
    use sui::coin::{Self};
    use std::debug;

    use leafsii::lfs_token::{Self};
    use leafsii::ve_lfs::{Self};

    const ALICE: address = @0xA11CE;

    const LOCK_AMOUNT: u64 = 1000_000_000; // 1000 LFS
    const ONE_YEAR_MS: u64 = 365 * 24 * 3600 * 1000;
    const TWO_YEARS_MS: u64 = 2 * 365 * 24 * 3600 * 1000;
    const FOUR_YEARS_MS: u64 = 4 * 365 * 24 * 3600 * 1000;

    // Test lock creation
    #[test]
    fun test_create_lock() {
        let mut scenario = ts::begin(ALICE);
        let ctx = ts::ctx(&mut scenario);

        let clock = clock::create_for_testing(ctx);
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ctx);
        ve_lfs::init_for_testing(ctx);

        // Mint LFS for testing
        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ctx);

        // Create a 1-year lock
        let lock = ve_lfs::create_lock(
            lfs_coin,
            ONE_YEAR_MS,
            &clock,
            ctx
        );

        // Check lock info
        let (owner, amount, start_ms, end_ms) = ve_lfs::get_lock_info(&lock);
        assert!(owner == ALICE, 0);
        assert!(amount == LOCK_AMOUNT, 1);
        assert!(start_ms == 0, 2); // Clock starts at 0
        assert!(end_ms == ONE_YEAR_MS, 3);

        // Check initial ve balance (should be close to locked amount for max duration)
        let ve_balance = ve_lfs::balance_of_at(&lock, 0);
        let max_lock = ve_lfs::max_lock_duration();
        let expected_ve = (LOCK_AMOUNT as u128) * (ONE_YEAR_MS as u128) / (max_lock as u128);
        assert!(ve_balance == (expected_ve as u64), 4);

        transfer::public_transfer(lock, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test ve balance calculation over time
    #[test]
    fun test_ve_balance_decay() {
        let mut scenario = ts::begin(ALICE);
        let ctx = ts::ctx(&mut scenario);

        let clock = clock::create_for_testing(ctx);
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ctx);
        ve_lfs::init_for_testing(ctx);

        // Create a 4-year lock (maximum duration)
        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ctx);
        let lock = ve_lfs::create_lock(
            lfs_coin,
            FOUR_YEARS_MS,
            &clock,
            ctx
        );

        // At start: ve balance should equal locked amount (max boost)
        let ve_at_start = ve_lfs::balance_of_at(&lock, 0);
        assert!(ve_at_start == LOCK_AMOUNT, 0);

        // At 2 years: ve balance should be 50% of locked amount
        let ve_at_half = ve_lfs::balance_of_at(&lock, TWO_YEARS_MS);
        assert!(ve_at_half == LOCK_AMOUNT / 2, 1);

        // At end: ve balance should be 0
        let ve_at_end = ve_lfs::balance_of_at(&lock, FOUR_YEARS_MS);
        assert!(ve_at_end == 0, 2);

        // After end: ve balance should still be 0
        let ve_after_end = ve_lfs::balance_of_at(&lock, FOUR_YEARS_MS + 1000);
        assert!(ve_after_end == 0, 3);

        debug::print(&ve_at_start);
        debug::print(&ve_at_half);
        debug::print(&ve_at_end);

        transfer::public_transfer(lock, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test lock extension
    #[test]
    fun test_extend_lock() {
        let mut scenario = ts::begin(ALICE);
        let ctx = ts::ctx(&mut scenario);

        let clock = clock::create_for_testing(ctx);
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ctx);
        ve_lfs::init_for_testing(ctx);

        // Create a 1-year lock
        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ctx);
        let mut lock = ve_lfs::create_lock(
            lfs_coin,
            ONE_YEAR_MS,
            &clock,
            ctx
        );

        let initial_ve = ve_lfs::balance_of_at(&lock, 0);

        // Extend to 2 years
        ve_lfs::extend_lock(&mut lock, TWO_YEARS_MS, &clock);

        // Check updated lock info
        let (_, _, _, new_end_ms) = ve_lfs::get_lock_info(&lock);
        assert!(new_end_ms == TWO_YEARS_MS, 0);

        // Ve balance should now be higher due to longer duration
        let extended_ve = ve_lfs::balance_of_at(&lock, 0);
        assert!(extended_ve > initial_ve, 1);
        assert!(extended_ve == LOCK_AMOUNT / 2, 2); // 2 years out of 4 max

        debug::print(&initial_ve);
        debug::print(&extended_ve);

        transfer::public_transfer(lock, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test increasing lock amount
    #[test]
    fun test_increase_amount() {
        let mut scenario = ts::begin(ALICE);
        let ctx = ts::ctx(&mut scenario);

        let clock = clock::create_for_testing(ctx);
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ctx);
        ve_lfs::init_for_testing(ctx);

        // Create initial lock
        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ctx);
        let mut lock = ve_lfs::create_lock(
            lfs_coin,
            TWO_YEARS_MS,
            &clock,
            ctx
        );

        let initial_ve = ve_lfs::balance_of_at(&lock, 0);

        // Add more LFS to the lock
        let additional_lfs = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ctx);
        ve_lfs::increase_amount(&mut lock, additional_lfs, &clock, ctx);

        // Check updated lock amount
        let (_, new_amount, _, _) = ve_lfs::get_lock_info(&lock);
        assert!(new_amount == LOCK_AMOUNT * 2, 0);

        // Ve balance should double
        let increased_ve = ve_lfs::balance_of_at(&lock, 0);
        assert!(increased_ve == initial_ve * 2, 1);

        debug::print(&initial_ve);
        debug::print(&increased_ve);

        transfer::public_transfer(lock, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test withdrawal after expiry
    #[test]
    fun test_withdraw_after_expiry() {
        let mut scenario = ts::begin(ALICE);
        let ctx = ts::ctx(&mut scenario);

        let mut clock = clock::create_for_testing(ctx);
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ctx);
        ve_lfs::init_for_testing(ctx);

        // Create a 1-year lock
        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ctx);
        let lock = ve_lfs::create_lock(
            lfs_coin,
            ONE_YEAR_MS,
            &clock,
            ctx
        );

        // Try to withdraw before expiry (should fail - tested separately)

        // Advance time past expiry
        clock::set_for_testing(&mut clock, ONE_YEAR_MS + 1000);

        // Withdraw should succeed
        let withdrawn_lfs = ve_lfs::withdraw(lock, &clock, ctx);
        assert!(coin::value(&withdrawn_lfs) == LOCK_AMOUNT, 0);

        transfer::public_transfer(withdrawn_lfs, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test multiple locks with different durations
    #[test]
    fun test_multiple_locks() {
        let mut scenario = ts::begin(ALICE);
        let ctx = ts::ctx(&mut scenario);

        let clock = clock::create_for_testing(ctx);
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ctx);
        ve_lfs::init_for_testing(ctx);

        // Create locks with different durations
        let lfs_1 = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ctx);
        let lock_1_year = ve_lfs::create_lock(lfs_1, ONE_YEAR_MS, &clock, ctx);

        let lfs_2 = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ctx);
        let lock_2_year = ve_lfs::create_lock(lfs_2, TWO_YEARS_MS, &clock, ctx);

        let lfs_4 = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ctx);
        let lock_4_year = ve_lfs::create_lock(lfs_4, FOUR_YEARS_MS, &clock, ctx);

        // Check ve balances
        let ve_1_year = ve_lfs::balance_of_at(&lock_1_year, 0);
        let ve_2_year = ve_lfs::balance_of_at(&lock_2_year, 0);
        let ve_4_year = ve_lfs::balance_of_at(&lock_4_year, 0);

        // Longer locks should have higher ve balances
        assert!(ve_4_year > ve_2_year, 0);
        assert!(ve_2_year > ve_1_year, 1);
        assert!(ve_4_year == LOCK_AMOUNT, 2); // 4 years = max duration

        debug::print(&ve_1_year);
        debug::print(&ve_2_year);
        debug::print(&ve_4_year);

        transfer::public_transfer(lock_1_year, ALICE);
        transfer::public_transfer(lock_2_year, ALICE);
        transfer::public_transfer(lock_4_year, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }
}