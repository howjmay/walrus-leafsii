#[test_only]
module leafsii::test_validator_gauge {
    use sui::test_scenario::{Self as ts};
    use sui::coin::{Self};

    use leafsii::lfs_token::{Self, LFS};
    use leafsii::validator_gauge::{Self, ValidatorGauge, ValidatorAdminCap, Validator, StakingPlan};

    const ADMIN: address = @0xAD;
    const VALIDATOR_1: address = @0x101;
    const VALIDATOR_2: address = @0x202;
    const VALIDATOR_3: address = @0x303;

    const STAKE_AMOUNT: u64 = 1000_000_000; // 1000 SUI

    // Test gauge creation
    #[test]
    fun test_create_gauge() {
        let mut scenario = ts::begin(ADMIN);

        let (gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        // Verify initial state
        assert!(validator_gauge::get_validator_count(&gauge) == 0, 0);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test adding validators
    #[test]
    fun test_add_validators() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        // Add validator 1
        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        let validator_1 = validator_gauge::create_validator(VALIDATOR_1, pool_id_1);
        validator_gauge::add_validator(&mut gauge, &admin_cap, validator_1);

        assert!(validator_gauge::get_validator_count(&gauge) == 1, 0);

        // Add validator 2
        let pool_id_2 = object::id_from_address(VALIDATOR_2);
        let validator_2 = validator_gauge::create_validator(VALIDATOR_2, pool_id_2);
        validator_gauge::add_validator(&mut gauge, &admin_cap, validator_2);

        assert!(validator_gauge::get_validator_count(&gauge) == 2, 1);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test setting validators in batch
    #[test]
    fun test_set_validators() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        let mut validators = vector::empty<Validator>();

        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_1, pool_id_1));

        let pool_id_2 = object::id_from_address(VALIDATOR_2);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_2, pool_id_2));

        let pool_id_3 = object::id_from_address(VALIDATOR_3);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_3, pool_id_3));

        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);

        assert!(validator_gauge::get_validator_count(&gauge) == 3, 0);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test removing validators
    #[test]
    fun test_remove_validators() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        // Set up validators
        let mut validators = vector::empty<Validator>();
        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_1, pool_id_1));
        let pool_id_2 = object::id_from_address(VALIDATOR_2);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_2, pool_id_2));

        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);
        assert!(validator_gauge::get_validator_count(&gauge) == 2, 0);

        // Remove validator at index 0
        validator_gauge::remove_validator(&mut gauge, &admin_cap, 0);
        assert!(validator_gauge::get_validator_count(&gauge) == 1, 1);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test epoch weights notification
    #[test]
    fun test_notify_epoch_weights() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        // Set up validators
        let mut validators = vector::empty<Validator>();
        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_1, pool_id_1));
        let pool_id_2 = object::id_from_address(VALIDATOR_2);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_2, pool_id_2));

        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);

        // Notify weights for epoch 1
        let mut weights = vector::empty<u64>();
        vector::push_back(&mut weights, 6000); // 60% to validator 1
        vector::push_back(&mut weights, 4000); // 40% to validator 2

        validator_gauge::notify_epoch_weights(&mut gauge, 1, weights);

        // Verify weights were stored
        let (last_epoch, latest_weights_opt) = validator_gauge::get_latest_epoch_weights(&gauge);
        assert!(last_epoch == 1, 0);
        assert!(option::is_some(&latest_weights_opt), 1);

        let latest_weights = option::destroy_some(latest_weights_opt);
        assert!(vector::length(&latest_weights) == 2, 2);
        assert!(*vector::borrow(&latest_weights, 0) == 6000, 3);
        assert!(*vector::borrow(&latest_weights, 1) == 4000, 4);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test staking plan generation with equal weights
    #[test]
    fun test_staking_plan_equal_weights() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        // Set up 2 validators
        let mut validators = vector::empty<Validator>();
        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_1, pool_id_1));
        let pool_id_2 = object::id_from_address(VALIDATOR_2);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_2, pool_id_2));

        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);

        // Equal weights
        let mut weights = vector::empty<u64>();
        vector::push_back(&mut weights, 5000);
        vector::push_back(&mut weights, 5000);

        validator_gauge::notify_epoch_weights(&mut gauge, 1, weights);

        // Generate staking plan
        let plan = validator_gauge::plan_stake_splits(&gauge, STAKE_AMOUNT, 1);
        let (allocations, total) = validator_gauge::get_staking_plan_details(&plan);

        assert!(total == STAKE_AMOUNT, 0);
        assert!(vector::length(allocations) == 2, 1);

        // Each should get approximately 50%
        let alloc_1 = vector::borrow(allocations, 0);
        let (_, _, amount_1) = validator_gauge::get_allocation_details(alloc_1);

        let alloc_2 = vector::borrow(allocations, 1);
        let (_, _, amount_2) = validator_gauge::get_allocation_details(alloc_2);

        assert!(amount_1 + amount_2 == STAKE_AMOUNT, 2);
        // Allow small rounding difference
        assert!(amount_1 >= STAKE_AMOUNT / 2 - 100, 3);
        assert!(amount_1 <= STAKE_AMOUNT / 2 + 100, 4);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test staking plan with weighted distribution
    #[test]
    fun test_staking_plan_weighted() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        // Set up 3 validators
        let mut validators = vector::empty<Validator>();
        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_1, pool_id_1));
        let pool_id_2 = object::id_from_address(VALIDATOR_2);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_2, pool_id_2));
        let pool_id_3 = object::id_from_address(VALIDATOR_3);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_3, pool_id_3));

        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);

        // Weights: 50%, 30%, 20%
        let mut weights = vector::empty<u64>();
        vector::push_back(&mut weights, 5000);
        vector::push_back(&mut weights, 3000);
        vector::push_back(&mut weights, 2000);

        validator_gauge::notify_epoch_weights(&mut gauge, 1, weights);

        // Generate staking plan for 1000 units
        let plan = validator_gauge::plan_stake_splits(&gauge, STAKE_AMOUNT, 1);
        let (allocations, total) = validator_gauge::get_staking_plan_details(&plan);

        assert!(total == STAKE_AMOUNT, 0);
        assert!(vector::length(allocations) == 3, 1);

        let alloc_1 = vector::borrow(allocations, 0);
        let (_, _, amount_1) = validator_gauge::get_allocation_details(alloc_1);

        let alloc_2 = vector::borrow(allocations, 1);
        let (_, _, amount_2) = validator_gauge::get_allocation_details(alloc_2);

        let alloc_3 = vector::borrow(allocations, 2);
        let (_, _, amount_3) = validator_gauge::get_allocation_details(alloc_3);

        // Verify total
        assert!(amount_1 + amount_2 + amount_3 == STAKE_AMOUNT, 2);

        // Verify proportions (with tolerance for rounding)
        let expected_1 = STAKE_AMOUNT / 2; // 50%
        let expected_2 = STAKE_AMOUNT * 3 / 10; // 30%
        let expected_3 = STAKE_AMOUNT / 5; // 20%

        assert!(amount_1 >= expected_1 - 1000, 3);
        assert!(amount_1 <= expected_1 + 1000, 4);
        assert!(amount_2 >= expected_2 - 1000, 5);
        assert!(amount_2 <= expected_2 + 1000, 6);
        assert!(amount_3 >= expected_3 - 1000, 7);
        assert!(amount_3 <= expected_3 + 1000, 8);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test staking plan with zero weight for one validator
    #[test]
    fun test_staking_plan_zero_weight() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        let mut validators = vector::empty<Validator>();
        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_1, pool_id_1));
        let pool_id_2 = object::id_from_address(VALIDATOR_2);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_2, pool_id_2));

        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);

        // One validator gets all weight, other gets zero
        let mut weights = vector::empty<u64>();
        vector::push_back(&mut weights, 10000);
        vector::push_back(&mut weights, 0);

        validator_gauge::notify_epoch_weights(&mut gauge, 1, weights);

        let plan = validator_gauge::plan_stake_splits(&gauge, STAKE_AMOUNT, 1);
        let (allocations, total) = validator_gauge::get_staking_plan_details(&plan);

        assert!(total == STAKE_AMOUNT, 0);
        // Only validator with weight should get allocation
        assert!(vector::length(allocations) == 1, 1);

        let alloc = vector::borrow(allocations, 0);
        let (addr, _, amount) = validator_gauge::get_allocation_details(alloc);

        assert!(addr == VALIDATOR_1, 2);
        assert!(amount == STAKE_AMOUNT, 3);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test staking plan with no weights (should fallback to equal distribution)
    #[test]
    fun test_staking_plan_no_weights() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        let mut validators = vector::empty<Validator>();
        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_1, pool_id_1));
        let pool_id_2 = object::id_from_address(VALIDATOR_2);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_2, pool_id_2));

        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);

        // Don't set any weights, generate plan anyway
        let plan = validator_gauge::plan_stake_splits(&gauge, STAKE_AMOUNT, 1);
        let (allocations, total) = validator_gauge::get_staking_plan_details(&plan);

        // Should fallback to equal distribution
        assert!(total == STAKE_AMOUNT, 0);
        assert!(vector::length(allocations) == 2, 1);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test notify_reward (should route to treasury)
    #[test]
    fun test_notify_reward() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        // Mint LFS for reward
        let reward_amount = 1000_000_000;
        let reward_coin = coin::mint(&mut treasury_cap, reward_amount, ts::ctx(&mut scenario));

        // Notify reward - should be routed to treasury
        validator_gauge::notify_reward(&mut gauge, reward_coin, 1, ts::ctx(&mut scenario));

        // Treasury should now have balance (we can't directly check, but function shouldn't error)

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test invalid weights length (mismatch with validator count)
    #[test]
    #[expected_failure(abort_code = 3)] // E_INVALID_WEIGHTS
    fun test_invalid_weights_length() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        // Set up 2 validators
        let mut validators = vector::empty<Validator>();
        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_1, pool_id_1));
        let pool_id_2 = object::id_from_address(VALIDATOR_2);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_2, pool_id_2));

        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);

        // Try to notify with 3 weights (should fail)
        let mut weights = vector::empty<u64>();
        vector::push_back(&mut weights, 3000);
        vector::push_back(&mut weights, 3000);
        vector::push_back(&mut weights, 4000);

        validator_gauge::notify_epoch_weights(&mut gauge, 1, weights);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test empty validator set
    #[test]
    #[expected_failure(abort_code = 4)] // E_EMPTY_VALIDATORS
    fun test_empty_validators() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        // Try to set empty validator list
        let validators = vector::empty<Validator>();
        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }

    // Test remove invalid validator index
    #[test]
    #[expected_failure(abort_code = 1)] // E_INVALID_VALIDATOR_INDEX
    fun test_remove_invalid_index() {
        let mut scenario = ts::begin(ADMIN);

        let (mut gauge, admin_cap) = validator_gauge::create_validator_gauge(ts::ctx(&mut scenario));

        let mut validators = vector::empty<Validator>();
        let pool_id_1 = object::id_from_address(VALIDATOR_1);
        vector::push_back(&mut validators, validator_gauge::create_validator(VALIDATOR_1, pool_id_1));

        validator_gauge::set_validators(&mut gauge, &admin_cap, validators);

        // Try to remove index 5 (only index 0 exists)
        validator_gauge::remove_validator(&mut gauge, &admin_cap, 5);

        transfer::public_transfer(gauge, ADMIN);
        transfer::public_transfer(admin_cap, ADMIN);
        ts::end(scenario);
    }
}
