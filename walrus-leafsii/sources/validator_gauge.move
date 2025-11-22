/// Validator Gauge for ve-LFS Based Validator Selection
///
/// The Validator Gauge enables decentralized, community-driven selection of Sui validators
/// for protocol staking operations. Unlike other gauges that distribute LFS rewards,
/// the validator gauge directs where the protocol stakes its SUI reserves.
///
/// Core Purpose:
/// - Determine which Sui validators receive protocol stake delegations
/// - Enable ve-LFS holders to influence validator selection
/// - Optimize staking yield and validator diversity through community governance
///
/// How It Works:
/// 1. Admin maintains a whitelist of eligible validators (address + pool_id)
/// 2. ve-LFS holders vote on validator weights each epoch
/// 3. Gauge controller sends aggregated vote weights to this gauge
/// 4. Protocol queries gauge for staking allocation plan
/// 5. Protocol distributes new stakes proportionally to validator weights
///
/// Staking Plan Generation:
/// ```
/// plan = plan_stake_splits(gauge, total_amount, current_epoch)
/// → Returns: [(validator_1, amount_1), (validator_2, amount_2), ...]
/// where amount_i = total_amount * weight_i / total_weight
/// ```
///
/// Key Differences from Other Gauges:
/// - Does NOT distribute LFS rewards (routes to treasury instead)
/// - Affects protocol operations (staking) rather than user rewards
/// - Requires admin management of validator whitelist
/// - Vote weights determine capital allocation, not token emissions
///
/// Admin Functions:
/// - add_validator: Add eligible validator to whitelist
/// - remove_validator: Remove validator from consideration
/// - set_validators: Batch update entire validator set
///
/// Integration:
/// - Receives epoch weights from gauge_controller
/// - Provides staking plans to main protocol contract
/// - LFS emissions (if any) route to treasury for future use
///
/// Example Use Case:
/// If Protocol has 1M SUI to stake and gauge votes are:
/// - Validator A: 60% votes → receives 600K SUI
/// - Validator B: 40% votes → receives 400K SUI
module leafsii::validator_gauge {
    use sui::coin::{Self, Coin};
    use sui::balance::{Self, Balance};
    use sui::table::{Self, Table};
    use sui::event;

    use leafsii::lfs_token::LFS;

    // Error codes
    const E_INVALID_VALIDATOR_INDEX: u64 = 1;
    const E_INVALID_WEIGHTS: u64 = 3;
    const E_EMPTY_VALIDATORS: u64 = 4;

    // Validator information
    public struct Validator has store, copy, drop {
        addr: address,          // Validator address
        pool_id: ID,           // Validator's staking pool ID
    }

    // Validator gauge for delegation decisions
    public struct ValidatorGauge has key, store {
        id: UID,
        validators: vector<Validator>,                    // List of eligible validators
        epoch_weights: Table<u64, vector<u64>>,          // epoch -> weights per validator
        treasury_balance: Balance<LFS>,                  // Treasury for LFS emissions
        total_epochs: u64,                               // Number of epochs tracked
        last_weight_update: u64,                         // Last epoch when weights were updated
    }

    // Admin capability for validator management
    public struct ValidatorAdminCap has key, store {
        id: UID,
        gauge_id: ID,
    }

    // Staking plan result
    public struct StakingPlan has copy, drop {
        validator_allocations: vector<ValidatorAllocation>,
        total_amount: u64,
    }

    public struct ValidatorAllocation has copy, drop {
        validator_addr: address,
        pool_id: ID,
        amount: u64,
    }

    // Events
    public struct ValidatorGaugeCreated has copy, drop {
        gauge_id: ID,
        admin_cap_id: ID,
    }

    public struct ValidatorsUpdated has copy, drop {
        gauge_id: ID,
        validator_count: u64,
    }

    public struct EpochWeightsNotified has copy, drop {
        gauge_id: ID,
        epoch: u64,
        weights: vector<u64>,
        total_weight: u64,
    }

    public struct StakingPlanGenerated has copy, drop {
        gauge_id: ID,
        epoch: u64,
        total_amount: u64,
        allocation_count: u64,
    }

    // Create a new validator gauge
    public fun create_validator_gauge(ctx: &mut TxContext): (ValidatorGauge, ValidatorAdminCap) {
        let gauge = ValidatorGauge {
            id: object::new(ctx),
            validators: vector::empty<Validator>(),
            epoch_weights: table::new<u64, vector<u64>>(ctx),
            treasury_balance: balance::zero<LFS>(),
            total_epochs: 0,
            last_weight_update: 0,
        };

        let gauge_id = object::id(&gauge);

        let admin_cap = ValidatorAdminCap {
            id: object::new(ctx),
            gauge_id,
        };

        let admin_cap_id = object::id(&admin_cap);

        event::emit(ValidatorGaugeCreated {
            gauge_id,
            admin_cap_id,
        });

        (gauge, admin_cap)
    }

    // Set the list of eligible validators (admin only)
    public fun set_validators(
        gauge: &mut ValidatorGauge,
        _admin_cap: &ValidatorAdminCap,
        validators: vector<Validator>
    ) {
        assert!(!vector::is_empty(&validators), E_EMPTY_VALIDATORS);

        gauge.validators = validators;

        event::emit(ValidatorsUpdated {
            gauge_id: object::id(gauge),
            validator_count: vector::length(&validators),
        });
    }

    // Add a single validator to the list
    public fun add_validator(
        gauge: &mut ValidatorGauge,
        _admin_cap: &ValidatorAdminCap,
        validator: Validator
    ) {
        vector::push_back(&mut gauge.validators, validator);

        event::emit(ValidatorsUpdated {
            gauge_id: object::id(gauge),
            validator_count: vector::length(&gauge.validators),
        });
    }

    // Remove a validator by index
    public fun remove_validator(
        gauge: &mut ValidatorGauge,
        _admin_cap: &ValidatorAdminCap,
        index: u64
    ) {
        assert!(index < vector::length(&gauge.validators), E_INVALID_VALIDATOR_INDEX);

        vector::remove(&mut gauge.validators, index);

        event::emit(ValidatorsUpdated {
            gauge_id: object::id(gauge),
            validator_count: vector::length(&gauge.validators),
        });
    }

    // Receive epoch weights from gauge controller
    // This is called by the gauge controller with the vote results
    public fun notify_epoch_weights(
        gauge: &mut ValidatorGauge,
        epoch: u64,
        weights: vector<u64>
    ) {
        let validator_count = vector::length(&gauge.validators);

        // Validate that weights match validator count
        assert!(vector::length(&weights) == validator_count, E_INVALID_WEIGHTS);

        // Store the weights for this epoch
        if (table::contains(&gauge.epoch_weights, epoch)) {
            // Replace existing weights
            let _ = table::remove(&mut gauge.epoch_weights, epoch);
        };

        table::add(&mut gauge.epoch_weights, epoch, weights);

        if (epoch > gauge.last_weight_update) {
            gauge.last_weight_update = epoch;
        };

        // Calculate total weight for event
        let mut total_weight = 0u64;
        let mut i = 0;
        while (i < vector::length(&weights)) {
            total_weight = total_weight + *vector::borrow(&weights, i);
            i = i + 1;
        };

        event::emit(EpochWeightsNotified {
            gauge_id: object::id(gauge),
            epoch,
            weights,
            total_weight,
        });
    }

    // Generate staking allocation plan based on latest epoch weights
    public fun plan_stake_splits(
        gauge: &ValidatorGauge,
        amount: u64,
        epoch: u64
    ): StakingPlan {
        assert!(amount > 0, E_INVALID_WEIGHTS);
        assert!(!vector::is_empty(&gauge.validators), E_EMPTY_VALIDATORS);

        // Get weights for the epoch (fallback to latest if not found)
        let weights = if (table::contains(&gauge.epoch_weights, epoch)) {
            table::borrow(&gauge.epoch_weights, epoch)
        } else if (gauge.last_weight_update > 0 && table::contains(&gauge.epoch_weights, gauge.last_weight_update)) {
            table::borrow(&gauge.epoch_weights, gauge.last_weight_update)
        } else {
            // No weights available - equal distribution
            let equal_weight = amount / vector::length(&gauge.validators);
            let mut equal_weights = vector::empty<u64>();
            let mut i = 0;
            while (i < vector::length(&gauge.validators)) {
                vector::push_back(&mut equal_weights, equal_weight);
                i = i + 1;
            };
            &equal_weights
        };

        // Calculate total weight
        let mut total_weight = 0u128;
        let mut i = 0;
        while (i < vector::length(weights)) {
            total_weight = total_weight + (*vector::borrow(weights, i) as u128);
            i = i + 1;
        };

        // Generate allocations
        let mut allocations = vector::empty<ValidatorAllocation>();
        let mut allocated_total = 0u64;

        let mut j = 0;
        while (j < vector::length(&gauge.validators)) {
            let validator = vector::borrow(&gauge.validators, j);
            let weight = *vector::borrow(weights, j);

            let allocation = if (j == vector::length(&gauge.validators) - 1) {
                // Last validator gets remaining amount to handle rounding
                amount - allocated_total
            } else if (total_weight > 0) {
                (((weight as u128) * (amount as u128)) / total_weight) as u64
            } else {
                0
            };

            if (allocation > 0) {
                vector::push_back(&mut allocations, ValidatorAllocation {
                    validator_addr: validator.addr,
                    pool_id: validator.pool_id,
                    amount: allocation,
                });

                allocated_total = allocated_total + allocation;
            };

            j = j + 1;
        };

        let plan = StakingPlan {
            validator_allocations: allocations,
            total_amount: allocated_total,
        };

        event::emit(StakingPlanGenerated {
            gauge_id: object::id(gauge),
            epoch,
            total_amount: allocated_total,
            allocation_count: vector::length(&allocations),
        });

        plan
    }

    // Handle LFS rewards - for validator gauge, route to treasury
    // since validators are chosen by ve-votes not rewarded with LFS directly
    public(package) fun notify_reward(
        gauge: &mut ValidatorGauge,
        amount: Coin<LFS>,
        _epoch: u64,
        _ctx: &mut TxContext
    ) {
        // Route LFS emissions to treasury since validators are selected
        // by ve-votes and don't receive direct LFS rewards
        if (coin::value(&amount) > 0) {
            balance::join(&mut gauge.treasury_balance, coin::into_balance(amount));
        } else {
            coin::destroy_zero(amount);
        }

        // Future: Could implement a reward distribution to validator stakers
        // but for now we route to treasury as specified
    }

    // View functions
    public fun get_validators(gauge: &ValidatorGauge): &vector<Validator> {
        &gauge.validators
    }

    public fun get_validator_count(gauge: &ValidatorGauge): u64 {
        vector::length(&gauge.validators)
    }

    public fun get_epoch_weights(gauge: &ValidatorGauge, epoch: u64): option::Option<vector<u64>> {
        if (table::contains(&gauge.epoch_weights, epoch)) {
            option::some(*table::borrow(&gauge.epoch_weights, epoch))
        } else {
            option::none()
        }
    }

    public fun get_latest_epoch_weights(gauge: &ValidatorGauge): (u64, option::Option<vector<u64>>) {
        if (gauge.last_weight_update > 0 && table::contains(&gauge.epoch_weights, gauge.last_weight_update)) {
            (gauge.last_weight_update, option::some(*table::borrow(&gauge.epoch_weights, gauge.last_weight_update)))
        } else {
            (0, option::none())
        }
    }

    public fun last_weight_update_epoch(gauge: &ValidatorGauge): u64 {
        gauge.last_weight_update
    }

    // Helper to create validator struct
    public fun create_validator(addr: address, pool_id: ID): Validator {
        Validator { addr, pool_id }
    }

    // Extract allocation details from staking plan
    public fun get_staking_plan_details(plan: &StakingPlan): (&vector<ValidatorAllocation>, u64) {
        (&plan.validator_allocations, plan.total_amount)
    }

    public fun get_allocation_details(allocation: &ValidatorAllocation): (address, ID, u64) {
        (allocation.validator_addr, allocation.pool_id, allocation.amount)
    }

    // Test-only functions
    #[test_only]
    public fun create_test_validator_gauge(ctx: &mut TxContext): (ValidatorGauge, ValidatorAdminCap) {
        create_validator_gauge(ctx)
    }

    #[test_only]
    public fun create_test_validator(addr: address): Validator {
        Validator {
            addr,
            pool_id: object::id_from_address(addr), // Simple test mapping
        }
    }

    #[test_only]
    public fun test_plan_calculation(
        validators: vector<Validator>,
        amount: u64,
        weights: vector<u64>,
        ctx: &mut TxContext
    ): StakingPlan {
        // Create a test gauge for calculation
        let mut test_gauge = ValidatorGauge {
            id: object::new(ctx),
            validators,
            epoch_weights: table::new<u64, vector<u64>>(ctx),
            treasury_balance: balance::zero<LFS>(),
            total_epochs: 0,
            last_weight_update: 1,
        };

        table::add(&mut test_gauge.epoch_weights, 1, weights);
        let plan = plan_stake_splits(&test_gauge, amount, 1);

        // Clean up
        let ValidatorGauge { id, validators: _, epoch_weights, treasury_balance, total_epochs: _, last_weight_update: _ } = test_gauge;
        balance::destroy_zero(treasury_balance);
        table::destroy_empty(epoch_weights);
        object::delete(id);

        plan
    }
}