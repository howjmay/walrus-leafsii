#[test_only]
module leafsii::test_gauge_controller {
    use sui::clock::{Self, Clock};
    use sui::test_scenario::{Self as ts, Scenario};
    use sui::coin::{Self};

    use leafsii::lfs_token::{Self};
    use leafsii::ve_lfs::{Self, Lock};
    use leafsii::gauge_controller::{Self, Controller, VoteChoice};
    use leafsii::emissions::{Self, EmissionsState};

    const ADMIN: address = @0xAD;
    const ALICE: address = @0xA11CE;
    const BOB: address = @0xB0B;
    const CHARLIE: address = @0xCCC;

    const GAUGE_ADDR_SP: address = @0x100;
    const GAUGE_ADDR_LP: address = @0x200;
    const GAUGE_ADDR_VALIDATOR: address = @0x300;

    const LOCK_AMOUNT: u64 = 1000_000_000; // 1000 LFS
    const FOUR_YEARS_MS: u64 = 4 * 365 * 24 * 3600 * 1000;
    const ONE_WEEK_MS: u64 = 7 * 24 * 3600 * 1000;

    // Test gauge registration for all types
    #[test]
    fun test_gauge_registration() {
        let mut scenario = ts::begin(ADMIN);

        gauge_controller::init_for_testing(ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ADMIN);
        let mut controller = ts::take_shared<Controller>(&scenario);

        // Register SP gauge (type 0)
        let sp_id = gauge_controller::register_gauge(
            &mut controller,
            0, // SP gauge
            GAUGE_ADDR_SP,
            ts::ctx(&mut scenario)
        );

        // Register LP gauge (type 1)
        let lp_id = gauge_controller::register_gauge(
            &mut controller,
            1, // LP gauge
            GAUGE_ADDR_LP,
            ts::ctx(&mut scenario)
        );

        // Register Validator gauge (type 2)
        let validator_id = gauge_controller::register_gauge(
            &mut controller,
            2, // Validator gauge
            GAUGE_ADDR_VALIDATOR,
            ts::ctx(&mut scenario)
        );

        // Verify total gauges
        assert!(gauge_controller::total_gauges(&controller) == 3, 0);

        // Verify gauge info
        let (sp_kind, sp_addr) = gauge_controller::get_gauge_info(&controller, 1);
        assert!(sp_kind == 0, 1);
        assert!(sp_addr == GAUGE_ADDR_SP, 2);

        let (lp_kind, lp_addr) = gauge_controller::get_gauge_info(&controller, 2);
        assert!(lp_kind == 1, 3);
        assert!(lp_addr == GAUGE_ADDR_LP, 4);

        let (val_kind, val_addr) = gauge_controller::get_gauge_info(&controller, 3);
        assert!(val_kind == 2, 5);
        assert!(val_addr == GAUGE_ADDR_VALIDATOR, 6);

        ts::return_shared(controller);
        ts::end(scenario);
    }

    // Test invalid gauge type registration
    #[test]
    #[expected_failure(abort_code = 1)] // E_INVALID_GAUGE_KIND
    fun test_invalid_gauge_type() {
        let mut scenario = ts::begin(ADMIN);

        gauge_controller::init_for_testing(ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ADMIN);
        let mut controller = ts::take_shared<Controller>(&scenario);

        // Try to register invalid gauge type (type 99)
        gauge_controller::register_gauge(
            &mut controller,
            99,
            GAUGE_ADDR_SP,
            ts::ctx(&mut scenario)
        );

        ts::return_shared(controller);
        ts::end(scenario);
    }

    // Test single voter scenario
    #[test]
    fun test_single_voter() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        // Initialize LFS and ve-LFS
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));
        gauge_controller::init_for_testing(ts::ctx(&mut scenario));

        // Create lock for Alice
        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ts::ctx(&mut scenario));
        let alice_lock = ve_lfs::create_lock(lfs_coin, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ALICE);
        let mut controller = ts::take_shared<Controller>(&scenario);

        // Register gauges
        gauge_controller::register_gauge(&mut controller, 0, GAUGE_ADDR_SP, ts::ctx(&mut scenario));
        gauge_controller::register_gauge(&mut controller, 1, GAUGE_ADDR_LP, ts::ctx(&mut scenario));

        // Alice votes: 60% to SP, 40% to LP
        let mut votes = vector::empty<VoteChoice>();
        vector::push_back(&mut votes, gauge_controller::create_vote_choice(1, 6000));
        vector::push_back(&mut votes, gauge_controller::create_vote_choice(2, 4000));

        gauge_controller::vote(&mut controller, &alice_lock, votes, &clock, ts::ctx(&mut scenario));

        transfer::public_transfer(alice_lock, ALICE);
        ts::return_shared(controller);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test multi-voter scenario with different ve-LFS balances
    #[test]
    fun test_multi_voter() {
        let mut scenario = ts::begin(ADMIN);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        // Initialize system
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));
        gauge_controller::init_for_testing(ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ADMIN);
        let mut controller = ts::take_shared<Controller>(&scenario);

        // Register gauges
        gauge_controller::register_gauge(&mut controller, 0, GAUGE_ADDR_SP, ts::ctx(&mut scenario));
        gauge_controller::register_gauge(&mut controller, 1, GAUGE_ADDR_LP, ts::ctx(&mut scenario));

        // Alice creates lock with 1000 LFS
        ts::next_tx(&mut scenario, ALICE);
        let lfs_alice = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ts::ctx(&mut scenario));
        let alice_lock = ve_lfs::create_lock(lfs_alice, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        // Alice votes: 100% to SP
        let mut votes_alice = vector::empty<VoteChoice>();
        vector::push_back(&mut votes_alice, gauge_controller::create_vote_choice(1, 10000));

        gauge_controller::vote(&mut controller, &alice_lock, votes_alice, &clock, ts::ctx(&mut scenario));

        // Bob creates lock with 2000 LFS (2x Alice)
        ts::next_tx(&mut scenario, BOB);
        let lfs_bob = coin::mint(&mut treasury_cap, LOCK_AMOUNT * 2, ts::ctx(&mut scenario));
        let bob_lock = ve_lfs::create_lock(lfs_bob, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        // Bob votes: 100% to LP
        let mut votes_bob = vector::empty<VoteChoice>();
        vector::push_back(&mut votes_bob, gauge_controller::create_vote_choice(2, 10000));

        gauge_controller::vote(&mut controller, &bob_lock, votes_bob, &clock, ts::ctx(&mut scenario));

        // Charlie creates lock with 1000 LFS
        ts::next_tx(&mut scenario, CHARLIE);
        let lfs_charlie = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ts::ctx(&mut scenario));
        let charlie_lock = ve_lfs::create_lock(lfs_charlie, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        // Charlie votes: 50/50
        let mut votes_charlie = vector::empty<VoteChoice>();
        vector::push_back(&mut votes_charlie, gauge_controller::create_vote_choice(1, 5000));
        vector::push_back(&mut votes_charlie, gauge_controller::create_vote_choice(2, 5000));

        gauge_controller::vote(&mut controller, &charlie_lock, votes_charlie, &clock, ts::ctx(&mut scenario));

        // Expected weights: SP = 1000 + 500 = 1500, LP = 2000 + 500 = 2500
        // LP should get ~62.5% of emissions, SP should get ~37.5%

        transfer::public_transfer(alice_lock, ALICE);
        transfer::public_transfer(bob_lock, BOB);
        transfer::public_transfer(charlie_lock, CHARLIE);
        ts::return_shared(controller);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test voting with weights not summing to 10000
    #[test]
    #[expected_failure(abort_code = 5)] // E_WEIGHTS_SUM_INVALID
    fun test_invalid_weight_sum() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));
        gauge_controller::init_for_testing(ts::ctx(&mut scenario));

        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ts::ctx(&mut scenario));
        let alice_lock = ve_lfs::create_lock(lfs_coin, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ALICE);
        let mut controller = ts::take_shared<Controller>(&scenario);

        gauge_controller::register_gauge(&mut controller, 0, GAUGE_ADDR_SP, ts::ctx(&mut scenario));

        // Invalid: weights sum to 5000 instead of 10000
        let mut votes = vector::empty<VoteChoice>();
        vector::push_back(&mut votes, gauge_controller::create_vote_choice(1, 5000));

        gauge_controller::vote(&mut controller, &alice_lock, votes, &clock, ts::ctx(&mut scenario));

        transfer::public_transfer(alice_lock, ALICE);
        ts::return_shared(controller);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test voting for non-existent gauge
    #[test]
    #[expected_failure(abort_code = 2)] // E_GAUGE_NOT_FOUND
    fun test_vote_nonexistent_gauge() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));
        gauge_controller::init_for_testing(ts::ctx(&mut scenario));

        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ts::ctx(&mut scenario));
        let alice_lock = ve_lfs::create_lock(lfs_coin, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ALICE);
        let mut controller = ts::take_shared<Controller>(&scenario);

        // Try to vote for gauge ID 999 which doesn't exist
        let mut votes = vector::empty<VoteChoice>();
        vector::push_back(&mut votes, gauge_controller::create_vote_choice(999, 10000));

        gauge_controller::vote(&mut controller, &alice_lock, votes, &clock, ts::ctx(&mut scenario));

        transfer::public_transfer(alice_lock, ALICE);
        ts::return_shared(controller);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test double voting in same epoch
    #[test]
    #[expected_failure(abort_code = 3)] // E_ALREADY_VOTED_THIS_EPOCH
    fun test_double_voting_same_epoch() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));
        gauge_controller::init_for_testing(ts::ctx(&mut scenario));

        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ts::ctx(&mut scenario));
        let alice_lock = ve_lfs::create_lock(lfs_coin, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ALICE);
        let mut controller = ts::take_shared<Controller>(&scenario);

        gauge_controller::register_gauge(&mut controller, 0, GAUGE_ADDR_SP, ts::ctx(&mut scenario));

        // First vote
        let mut votes1 = vector::empty<VoteChoice>();
        vector::push_back(&mut votes1, gauge_controller::create_vote_choice(1, 10000));
        gauge_controller::vote(&mut controller, &alice_lock, votes1, &clock, ts::ctx(&mut scenario));

        // Second vote in same epoch - should fail
        let mut votes2 = vector::empty<VoteChoice>();
        vector::push_back(&mut votes2, gauge_controller::create_vote_choice(1, 10000));
        gauge_controller::vote(&mut controller, &alice_lock, votes2, &clock, ts::ctx(&mut scenario));

        transfer::public_transfer(alice_lock, ALICE);
        ts::return_shared(controller);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test epoch checkpoint and voting in new epoch
    #[test]
    fun test_epoch_transition() {
        let mut scenario = ts::begin(ALICE);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));
        gauge_controller::init_for_testing(ts::ctx(&mut scenario));
        emissions::init_for_testing(ts::ctx(&mut scenario));

        // Set clock to after epoch start (Jan 1, 2025)
        let epoch_start = 1735689600000; // Jan 1, 2025
        clock::set_for_testing(&mut clock, epoch_start);

        let lfs_coin = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ts::ctx(&mut scenario));
        let alice_lock = ve_lfs::create_lock(lfs_coin, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ALICE);
        let mut controller = ts::take_shared<Controller>(&scenario);

        gauge_controller::register_gauge(&mut controller, 0, GAUGE_ADDR_SP, ts::ctx(&mut scenario));

        // Vote in epoch 0
        let mut votes1 = vector::empty<VoteChoice>();
        vector::push_back(&mut votes1, gauge_controller::create_vote_choice(1, 10000));
        gauge_controller::vote(&mut controller, &alice_lock, votes1, &clock, ts::ctx(&mut scenario));

        // Advance to next epoch (1 week later)
        clock::set_for_testing(&mut clock, epoch_start + ONE_WEEK_MS);

        // Checkpoint to new epoch
        gauge_controller::checkpoint_epoch(&mut controller, &clock);

        // Vote again in new epoch - should succeed
        let mut votes2 = vector::empty<VoteChoice>();
        vector::push_back(&mut votes2, gauge_controller::create_vote_choice(1, 10000));
        gauge_controller::vote(&mut controller, &alice_lock, votes2, &clock, ts::ctx(&mut scenario));

        transfer::public_transfer(alice_lock, ALICE);
        ts::return_shared(controller);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test emissions distribution with votes
    #[test]
    fun test_emissions_distribution() {
        let mut scenario = ts::begin(ADMIN);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));
        gauge_controller::init_for_testing(ts::ctx(&mut scenario));
        emissions::init_for_testing(ts::ctx(&mut scenario));

        // Set clock to epoch 1
        let epoch_start = 1735689600000; // Jan 1, 2025
        let one_week = 604800000; // 1 week in ms
        clock::set_for_testing(&mut clock, epoch_start + one_week);

        ts::next_tx(&mut scenario, ADMIN);
        let mut controller = ts::take_shared<Controller>(&scenario);
        let mut emissions_state = ts::take_shared<EmissionsState>(&scenario);

        // Register gauges
        gauge_controller::register_gauge(&mut controller, 0, GAUGE_ADDR_SP, ts::ctx(&mut scenario));
        gauge_controller::register_gauge(&mut controller, 1, GAUGE_ADDR_LP, ts::ctx(&mut scenario));

        // Alice votes
        ts::next_tx(&mut scenario, ALICE);
        let lfs_alice = coin::mint(&mut treasury_cap, LOCK_AMOUNT, ts::ctx(&mut scenario));
        let alice_lock = ve_lfs::create_lock(lfs_alice, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        let mut votes = vector::empty<VoteChoice>();
        vector::push_back(&mut votes, gauge_controller::create_vote_choice(1, 7000));
        vector::push_back(&mut votes, gauge_controller::create_vote_choice(2, 3000));

        gauge_controller::vote(&mut controller, &alice_lock, votes, &clock, ts::ctx(&mut scenario));

        // Distribute emissions for epoch 1
        gauge_controller::distribute_epoch(
            &mut controller,
            &mut emissions_state,
            &mut emissions_cap,
            &mut treasury_cap,
            1,
            &clock,
            ts::ctx(&mut scenario)
        );

        // Verify distribution happened
        // Gauge addresses should have received LFS tokens
        // This would be verified by checking their balances if we had access

        transfer::public_transfer(alice_lock, ALICE);
        ts::return_shared(controller);
        ts::return_shared(emissions_state);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test emissions routed to treasury when no votes
    #[test]
    fun test_no_votes_treasury() {
        let mut scenario = ts::begin(ADMIN);
        let mut clock = clock::create_for_testing(ts::ctx(&mut scenario));

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        gauge_controller::init_for_testing(ts::ctx(&mut scenario));
        emissions::init_for_testing(ts::ctx(&mut scenario));

        // Set clock to epoch 1
        let epoch_start = 1735689600000; // Jan 1, 2025
        let one_week = 604800000; // 1 week in ms
        clock::set_for_testing(&mut clock, epoch_start + one_week);

        ts::next_tx(&mut scenario, ADMIN);
        let mut controller = ts::take_shared<Controller>(&scenario);
        let mut emissions_state = ts::take_shared<EmissionsState>(&scenario);

        // Register gauge but don't vote
        gauge_controller::register_gauge(&mut controller, 0, GAUGE_ADDR_SP, ts::ctx(&mut scenario));

        let treasury_before = gauge_controller::treasury_balance(&controller);

        // Distribute emissions without any votes
        gauge_controller::distribute_epoch(
            &mut controller,
            &mut emissions_state,
            &mut emissions_cap,
            &mut treasury_cap,
            1,
            &clock,
            ts::ctx(&mut scenario)
        );

        let treasury_after = gauge_controller::treasury_balance(&controller);

        // Treasury should have increased
        assert!(treasury_after > treasury_before, 0);

        ts::return_shared(controller);
        ts::return_shared(emissions_state);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }
}
