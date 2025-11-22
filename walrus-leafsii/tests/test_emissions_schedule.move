#[test_only]
module leafsii::test_emissions_schedule {
    use sui::clock;
    use sui::test_scenario::{Self as ts};
    use sui::coin::{Self};
    use std::debug;

    use leafsii::lfs_token::{Self};
    use leafsii::emissions::{Self};

    const ALICE: address = @0xA11CE;

    // Test epoch calculation
    #[test]
    fun test_epoch_calculation() {
        let mut scenario = ts::begin(ALICE);
        let ctx = ts::ctx(&mut scenario);

        let mut clock = clock::create_for_testing(ctx);

        // Test current epoch calculation
        let epoch_start = emissions::epoch_start_time();
        let week_duration = emissions::epoch_duration();

        // Set time to epoch start
        clock::set_for_testing(&mut clock, epoch_start);
        let epoch_0 = emissions::current_epoch(&clock);
        assert!(epoch_0 == 0, 0);

        // Set time to 1 week later
        clock::set_for_testing(&mut clock, epoch_start + week_duration);
        let epoch_1 = emissions::current_epoch(&clock);
        assert!(epoch_1 == 1, 1);

        // Set time to 52 weeks later (1 year)
        clock::set_for_testing(&mut clock, epoch_start + (52 * week_duration));
        let epoch_52 = emissions::current_epoch(&clock);
        assert!(epoch_52 == 52, 2);

        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test emission amounts with decay
    #[test]
    fun test_emission_amounts() {
        // Test initial emission (year 0, epoch 1)
        let epoch_1_emission = emissions::emission_for_epoch(1);
        let expected_initial = emissions::initial_weekly_emission();
        assert!(epoch_1_emission == expected_initial, 0);

        // Test epoch 0 has no emission
        let epoch_0_emission = emissions::emission_for_epoch(0);
        assert!(epoch_0_emission == 0, 1);

        // Test decay after 1 year (epoch 53)
        let epoch_53_emission = emissions::emission_for_epoch(53);
        let expected_year_1 = expected_initial * 9 / 10; // 10% decay
        assert!(epoch_53_emission == expected_year_1, 2);

        // Test decay after 2 years (epoch 105)
        let epoch_105_emission = emissions::emission_for_epoch(105);
        let expected_year_2 = expected_year_1 * 9 / 10; // Another 10% decay
        assert!(epoch_105_emission == expected_year_2, 3);

        debug::print(&expected_initial);
        debug::print(&epoch_53_emission);
        debug::print(&epoch_105_emission);
    }

    // Test year index calculation
    #[test]
    fun test_year_index() {
        // Epoch 0 should be year 0
        let year_0_a = emissions::year_index_for_epoch(0);
        assert!(year_0_a == 0, 0);

        // Epochs 1-52 should be year 0
        let year_0_b = emissions::year_index_for_epoch(1);
        assert!(year_0_b == 0, 1);

        let year_0_c = emissions::year_index_for_epoch(52);
        assert!(year_0_c == 0, 2);

        // Epoch 53 should be year 1
        let year_1 = emissions::year_index_for_epoch(53);
        assert!(year_1 == 1, 3);

        // Epoch 105 should be year 2
        let year_2 = emissions::year_index_for_epoch(105);
        assert!(year_2 == 2, 4);
    }

    // Test emission state and minting
    #[test]
    fun test_emission_minting() {
        let mut scenario = ts::begin(ALICE);

        // Initialize components
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));
        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        emissions::init_for_testing(ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ALICE);
        let mut emissions_state = ts::take_shared<emissions::EmissionsState>(&scenario);

        // Set time to epoch 1
        let epoch_start = emissions::epoch_start_time();
        let week_duration = emissions::epoch_duration();
        clock::set_for_testing(&mut clock, epoch_start + week_duration);

        // Check initial state
        let (last_minted, total_emitted) = emissions::get_emissions_info(&emissions_state);
        assert!(last_minted == 0, 0);
        assert!(total_emitted == 0, 1);

        // Mint epoch 1 emission
        let emission_coin = emissions::mint_epoch_emission(
            &mut emissions_state,
            &mut emissions_cap,
            &mut treasury_cap,
            1,
            &clock,
            ts::ctx(&mut scenario)
        );

        let expected_amount = emissions::emission_for_epoch(1);
        assert!(coin::value(&emission_coin) == expected_amount, 2);

        // Check updated state
        let (last_minted_2, total_emitted_2) = emissions::get_emissions_info(&emissions_state);
        assert!(last_minted_2 == 1, 3);
        assert!(total_emitted_2 == expected_amount, 4);

        // Try to mint the same epoch again (should fail)
        // This would cause an abort, so we test it separately

        transfer::public_transfer(emission_coin, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        ts::return_shared(emissions_state);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test emission bounds over time
    #[test]
    fun test_emission_bounds() {
        let initial_emission = emissions::initial_weekly_emission();

        // Check that emissions decrease over time
        let year_0_emission = emissions::emission_for_epoch(1);
        let year_1_emission = emissions::emission_for_epoch(53);
        let year_2_emission = emissions::emission_for_epoch(105);

        assert!(year_0_emission == initial_emission, 0);
        assert!(year_1_emission < year_0_emission, 1);
        assert!(year_2_emission < year_1_emission, 2);

        // Check that decay is approximately 10%
        let decay_ratio = (year_1_emission * 10) / year_0_emission;
        assert!(decay_ratio == 9, 3); // Should be exactly 9/10

        debug::print(&year_0_emission);
        debug::print(&year_1_emission);
        debug::print(&year_2_emission);
    }
}