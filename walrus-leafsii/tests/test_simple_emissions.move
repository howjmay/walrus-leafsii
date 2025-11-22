#[test_only]
module leafsii::test_simple_emissions {
    use sui::clock::{Self};
    use sui::test_scenario::{Self as ts};

    use leafsii::emissions::{Self};

    const ALICE: address = @0xA11CE;

    // Simple test for basic emission calculation
    #[test]
    fun test_basic_emission_calculation() {
        let mut scenario = ts::begin(ALICE);
        let ctx = ts::ctx(&mut scenario);

        let clock = clock::create_for_testing(ctx);

        // Test emission amounts
        let epoch_0_emission = emissions::emission_for_epoch(0);
        assert!(epoch_0_emission == 0, 0);

        let epoch_1_emission = emissions::emission_for_epoch(1);
        let expected_initial = emissions::initial_weekly_emission();
        assert!(epoch_1_emission == expected_initial, 1);

        // Test year index calculation
        let year_0 = emissions::year_index_for_epoch(1);
        assert!(year_0 == 0, 2);

        let year_1 = emissions::year_index_for_epoch(53);
        assert!(year_1 == 1, 3);

        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }
}