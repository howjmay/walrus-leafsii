#[test_only]
module leafsii::test_vesting_escrow {
    use sui::clock::{Self, Clock};
    use sui::test_scenario::{Self as ts};
    use sui::coin::{Self};

    use leafsii::lfs_token::{Self};
    use leafsii::vesting_escrow::{Self};

    const ALICE: address = @0xA11CE;
    const BOB: address = @0xB0B;

    const VESTING_AMOUNT: u64 = 1000_000_000; // 1000 LFS
    const SEVEN_DAYS_MS: u64 = 7 * 24 * 3600 * 1000;
    const ONE_DAY_MS: u64 = 24 * 3600 * 1000;
    const THREE_DAYS_MS: u64 = 3 * 24 * 3600 * 1000;

    // Test creating standard vesting
    #[test]
    fun test_create_standard_vesting() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let vesting = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_coin,
            &clock,
            ts::ctx(&mut scenario)
        );

        // Check vesting info
        let (owner, start_ms, duration_ms, total_amount, claimed_amount, remaining) =
            vesting_escrow::get_vesting_info(&vesting);

        assert!(owner == ALICE, 0);
        assert!(start_ms == 0, 1); // Clock starts at 0
        assert!(duration_ms == SEVEN_DAYS_MS, 2);
        assert!(total_amount == VESTING_AMOUNT, 3);
        assert!(claimed_amount == 0, 4);
        assert!(remaining == VESTING_AMOUNT, 5);

        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test custom vesting duration
    #[test]
    fun test_create_custom_vesting() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let custom_duration = 30 * 24 * 3600 * 1000; // 30 days
        let vesting = vesting_escrow::create_vesting(
            ALICE,
            lfs_coin,
            0,
            custom_duration,
            ts::ctx(&mut scenario)
        );

        let (_, _, duration_ms, _, _, _) = vesting_escrow::get_vesting_info(&vesting);
        assert!(duration_ms == custom_duration, 0);

        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test claimable amount calculation over time
    #[test]
    fun test_claimable_over_time() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let vesting = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_coin,
            &clock,
            ts::ctx(&mut scenario)
        );

        // Day 0: Nothing claimable yet
        let claimable_0 = vesting_escrow::claimable(&vesting, &clock);
        assert!(claimable_0 == 0, 0);

        // Day 1: ~14.29% should be claimable (1/7)
        let claimable_1 = vesting_escrow::claimable_at(&vesting, ONE_DAY_MS);
        let expected_day_1 = VESTING_AMOUNT / 7;
        assert!(claimable_1 >= expected_day_1 - 1000, 1); // Allow small rounding
        assert!(claimable_1 <= expected_day_1 + 1000, 2);

        // Day 3: ~42.86% should be claimable (3/7)
        let claimable_3 = vesting_escrow::claimable_at(&vesting, THREE_DAYS_MS);
        let expected_day_3 = VESTING_AMOUNT * 3 / 7;
        assert!(claimable_3 >= expected_day_3 - 1000, 3);
        assert!(claimable_3 <= expected_day_3 + 1000, 4);

        // Day 7: 100% should be claimable
        let claimable_7 = vesting_escrow::claimable_at(&vesting, SEVEN_DAYS_MS);
        assert!(claimable_7 == VESTING_AMOUNT, 5);

        // After day 7: Still 100% claimable
        let claimable_8 = vesting_escrow::claimable_at(&vesting, SEVEN_DAYS_MS + ONE_DAY_MS);
        assert!(claimable_8 == VESTING_AMOUNT, 6);

        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test claiming tokens at different times
    #[test]
    fun test_claim_progressive() {
        let mut scenario = ts::begin(ALICE);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let mut vesting = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_coin,
            &clock,
            ts::ctx(&mut scenario)
        );

        // Advance to day 3
        clock::set_for_testing(&mut clock, THREE_DAYS_MS);

        // Claim at day 3
        let claimed_3 = vesting_escrow::claim(&mut vesting, &clock, ts::ctx(&mut scenario));
        let claimed_3_value = coin::value(&claimed_3);

        // Should be approximately 3/7 of total
        let expected = VESTING_AMOUNT * 3 / 7;
        assert!(claimed_3_value >= expected - 1000, 0);
        assert!(claimed_3_value <= expected + 1000, 1);

        // Check updated state
        let (_, _, _, _, claimed_amount, remaining) = vesting_escrow::get_vesting_info(&vesting);
        assert!(claimed_amount == claimed_3_value, 2);
        assert!(remaining == VESTING_AMOUNT - claimed_3_value, 3);

        // Advance to day 7
        clock::set_for_testing(&mut clock, SEVEN_DAYS_MS);

        // Claim remaining
        let claimed_7 = vesting_escrow::claim(&mut vesting, &clock, ts::ctx(&mut scenario));
        let claimed_7_value = coin::value(&claimed_7);

        // Should be remaining amount
        assert!(claimed_3_value + claimed_7_value == VESTING_AMOUNT, 4);

        let (_, _, _, _, final_claimed, final_remaining) = vesting_escrow::get_vesting_info(&vesting);
        assert!(final_claimed == VESTING_AMOUNT, 5);
        assert!(final_remaining == 0, 6);

        transfer::public_transfer(claimed_3, ALICE);
        transfer::public_transfer(claimed_7, ALICE);
        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test claiming nothing at start
    #[test]
    #[expected_failure(abort_code = 3)] // E_NOTHING_TO_CLAIM
    fun test_claim_at_start() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let mut vesting = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_coin,
            &clock,
            ts::ctx(&mut scenario)
        );

        // Try to claim immediately - should fail
        let claimed = vesting_escrow::claim(&mut vesting, &clock, ts::ctx(&mut scenario));

        transfer::public_transfer(claimed, ALICE);
        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test unauthorized claim
    #[test]
    #[expected_failure(abort_code = 4)] // E_UNAUTHORIZED
    fun test_unauthorized_claim() {
        let mut scenario = ts::begin(ALICE);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let mut vesting = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_coin,
            &clock,
            ts::ctx(&mut scenario)
        );

        clock::set_for_testing(&mut clock, THREE_DAYS_MS);

        // Bob tries to claim Alice's vesting - should fail
        ts::next_tx(&mut scenario, BOB);
        let claimed = vesting_escrow::claim(&mut vesting, &clock, ts::ctx(&mut scenario));

        transfer::public_transfer(claimed, BOB);
        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test adding to existing vesting
    #[test]
    fun test_add_vesting() {
        let mut scenario = ts::begin(ALICE);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let mut vesting = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_coin,
            &clock,
            ts::ctx(&mut scenario)
        );

        // Advance time to day 3
        clock::set_for_testing(&mut clock, THREE_DAYS_MS);

        // Add more vesting
        let additional_lfs = coin::mint(&mut treasury_cap, VESTING_AMOUNT / 2, ts::ctx(&mut scenario));
        vesting_escrow::add_vesting(&mut vesting, additional_lfs, &clock);

        // Check updated info
        let (_, start_ms, duration_ms, total_amount, _, remaining) =
            vesting_escrow::get_vesting_info(&vesting);

        // Start time should be reset to current time
        assert!(start_ms == THREE_DAYS_MS, 0);
        // Duration should reset to 7 days
        assert!(duration_ms == SEVEN_DAYS_MS, 1);
        // Total should be original + additional
        assert!(total_amount == VESTING_AMOUNT + VESTING_AMOUNT / 2, 2);
        assert!(remaining == VESTING_AMOUNT + VESTING_AMOUNT / 2, 3);

        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test is_fully_vested check
    #[test]
    fun test_is_fully_vested() {
        let mut scenario = ts::begin(ALICE);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let vesting = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_coin,
            &clock,
            ts::ctx(&mut scenario)
        );

        // At start: not fully vested
        assert!(!vesting_escrow::is_fully_vested(&vesting, &clock), 0);

        // At day 3: still not fully vested
        clock::set_for_testing(&mut clock, THREE_DAYS_MS);
        assert!(!vesting_escrow::is_fully_vested(&vesting, &clock), 1);

        // At day 7: fully vested
        clock::set_for_testing(&mut clock, SEVEN_DAYS_MS);
        assert!(vesting_escrow::is_fully_vested(&vesting, &clock), 2);

        // After day 7: still fully vested
        clock::set_for_testing(&mut clock, SEVEN_DAYS_MS + ONE_DAY_MS);
        assert!(vesting_escrow::is_fully_vested(&vesting, &clock), 3);

        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test multiple concurrent vestings for same user
    #[test]
    fun test_multiple_vestings() {
        let mut scenario = ts::begin(ALICE);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Create first vesting
        let lfs_1 = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));
        let mut vesting_1 = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_1,
            &clock,
            ts::ctx(&mut scenario)
        );

        // Advance time
        clock::set_for_testing(&mut clock, THREE_DAYS_MS);

        // Create second vesting (starts at day 3)
        let lfs_2 = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));
        let mut vesting_2 = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_2,
            &clock,
            ts::ctx(&mut scenario)
        );

        // At day 7: vesting_1 is fully vested, vesting_2 is ~57% vested
        clock::set_for_testing(&mut clock, SEVEN_DAYS_MS);

        let claimable_1 = vesting_escrow::claimable(&vesting_1, &clock);
        assert!(claimable_1 == VESTING_AMOUNT, 0);

        let claimable_2 = vesting_escrow::claimable(&vesting_2, &clock);
        // vesting_2 started at day 3, so at day 7 it's 4 days into 7-day vesting
        let expected_2 = VESTING_AMOUNT * 4 / 7;
        assert!(claimable_2 >= expected_2 - 1000, 1);
        assert!(claimable_2 <= expected_2 + 1000, 2);

        // Claim from both
        let claimed_1 = vesting_escrow::claim(&mut vesting_1, &clock, ts::ctx(&mut scenario));
        let claimed_2 = vesting_escrow::claim(&mut vesting_2, &clock, ts::ctx(&mut scenario));

        assert!(coin::value(&claimed_1) == VESTING_AMOUNT, 3);

        transfer::public_transfer(claimed_1, ALICE);
        transfer::public_transfer(claimed_2, ALICE);
        transfer::public_transfer(vesting_1, ALICE);
        transfer::public_transfer(vesting_2, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test claim all at once after full vesting
    #[test]
    fun test_claim_all_after_vesting() {
        let mut scenario = ts::begin(ALICE);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let mut vesting = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_coin,
            &clock,
            ts::ctx(&mut scenario)
        );

        // Advance to after vesting period
        clock::set_for_testing(&mut clock, SEVEN_DAYS_MS + ONE_DAY_MS);

        // Claim everything
        let claimed = vesting_escrow::claim(&mut vesting, &clock, ts::ctx(&mut scenario));

        assert!(coin::value(&claimed) == VESTING_AMOUNT, 0);

        let (_, _, _, _, claimed_amount, remaining) = vesting_escrow::get_vesting_info(&vesting);
        assert!(claimed_amount == VESTING_AMOUNT, 1);
        assert!(remaining == 0, 2);

        transfer::public_transfer(claimed, ALICE);
        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test vesting duration constant
    #[test]
    fun test_vesting_duration_constant() {
        let duration = vesting_escrow::vesting_duration();
        assert!(duration == SEVEN_DAYS_MS, 0);
    }

    // Test claiming twice in same day
    #[test]
    #[expected_failure(abort_code = 3)] // E_NOTHING_TO_CLAIM
    fun test_double_claim_same_time() {
        let mut scenario = ts::begin(ALICE);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let lfs_coin = coin::mint(&mut treasury_cap, VESTING_AMOUNT, ts::ctx(&mut scenario));

        let mut vesting = vesting_escrow::create_standard_vesting(
            ALICE,
            lfs_coin,
            &clock,
            ts::ctx(&mut scenario)
        );

        clock::set_for_testing(&mut clock, THREE_DAYS_MS);

        // First claim
        let claimed_1 = vesting_escrow::claim(&mut vesting, &clock, ts::ctx(&mut scenario));

        // Second claim at same time - should fail
        let claimed_2 = vesting_escrow::claim(&mut vesting, &clock, ts::ctx(&mut scenario));

        transfer::public_transfer(claimed_1, ALICE);
        transfer::public_transfer(claimed_2, ALICE);
        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }
}
