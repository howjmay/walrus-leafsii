/// Gauge Controller for LFS Protocol
///
/// The Gauge Controller is the central coordination point for vote-directed LFS emissions.
/// It registers gauges (reward distribution targets), collects ve-LFS votes, and allocates
/// weekly emissions proportionally to gauge weights.
///
/// System Architecture:
/// 1. Gauge Types:
///    - SP Gauge (Type 0): Stability Pool - rewards fToken depositors
///    - LP Gauge (Type 1): Liquidity Pool - rewards LP token stakers
///    - Validator Gauge (Type 2): Validator selection - directs protocol staking
///
/// 2. Voting Mechanism:
///    - Users with ve-LFS locks vote on gauge weights each epoch
///    - Voting power = current ve-LFS balance at time of vote
///    - Votes are expressed in basis points summing to 10,000 (100%)
///    - One vote per user per epoch, votes don't carry over
///
/// 3. Emissions Distribution:
///    - Weekly LFS emissions are split proportionally to gauge votes
///    - Formula: gauge_allocation = total_emission * gauge_weight / total_weight
///    - Unallocated emissions (no votes) route to treasury
///    - Emissions are sent directly to gauge addresses for distribution
///
/// 4. Dynamic Fields Architecture:
///    Uses dynamic fields to avoid table size limitations:
///    - epoch_weights_{epoch}: Maps gauge_id → vote weight for specific epoch
///    - user_votes_{epoch}_{user}: Stores user's vote record for epoch
///    This enables unlimited epochs and voters without storage constraints
///
/// Workflow:
/// 1. Gauges are registered by admin with (type, address)
/// 2. ve-LFS holders vote on gauge allocations for current epoch
/// 3. Epoch progresses, checkpointing finalizes voting
/// 4. distribute_epoch() mints emissions and sends to gauges by weight
/// 5. Each gauge handles its own reward distribution to participants
///
/// Integration:
/// - Works with emissions module for epoch timing and minting
/// - Validates votes against ve_lfs module for voting power
/// - Coordinates with individual gauge contracts for reward distribution
module leafsii::gauge_controller {
    use sui::coin::Self;
    use sui::clock::{Self, Clock};
    use sui::table::{Self, Table};
    use sui::event;
    use sui::dynamic_field as df;
    use std::bcs;
    use sui::balance::{Self, Balance};

    use sui::coin::TreasuryCap;
    use leafsii::lfs_token::{LFS, EmissionsCap};
    use leafsii::emissions::{Self, EmissionsState};
    use leafsii::ve_lfs::{Self, Lock};

    // Error codes
    const E_INVALID_GAUGE_KIND: u64 = 1;
    const E_GAUGE_NOT_FOUND: u64 = 2;
    const E_ALREADY_VOTED_THIS_EPOCH: u64 = 3;
    const E_WEIGHTS_SUM_INVALID: u64 = 5;
    const E_UNAUTHORIZED: u64 = 6;

    // Constants
    const GAUGE_KIND_SP: u8 = 0;
    const GAUGE_KIND_LP_ABSTRACT: u8 = 1;
    const GAUGE_KIND_VALIDATOR: u8 = 2;

    const BPS_TOTAL: u16 = 10000; // 100% = 10,000 basis points

    // Gauge identifier
    public struct GaugeId has copy, drop, store {
        raw: u64,
    }

    // Gauge information
    public struct GaugeInfo has store {
        id: GaugeId,
        kind: u8,
        addr: address,
    }

    // Vote record for a user in an epoch
    public struct VoteRecord has store {
        epoch: u64,
        total_ve_power: u64,
        votes: vector<GaugeVote>,
    }

    public struct GaugeVote has store, copy, drop {
        gauge_id: u64,
        weight_bps: u16,
    }

    // Controller state
    public struct Controller has key, store {
        id: UID,
        next_gauge_id: u64,
        gauges: Table<u64, GaugeInfo>,
        // Dynamic fields for epoch data to avoid table size limits:
        // - epoch_weights_{epoch} -> Table<u64, u64> (gauge_id -> total_ve_weight)
        // - user_votes_{epoch}_{user} -> VoteRecord
        current_epoch: u64,
        total_gauges: u64,
        treasury_balance: Balance<LFS>, // Treasury for unallocated emissions
    }

    // Events
    public struct ControllerCreated has copy, drop {
        controller_id: ID,
    }

    public struct GaugeRegistered has copy, drop {
        gauge_id: u64,
        kind: u8,
        addr: address,
    }

    public struct VoteCast has copy, drop {
        voter: address,
        epoch: u64,
        ve_power: u64,
        votes: vector<GaugeVote>,
    }

    public struct EpochCheckpointed has copy, drop {
        epoch: u64,
        total_gauges: u64,
    }

    public struct EmissionsDistributed has copy, drop {
        epoch: u64,
        total_emission: u64,
        distributions: vector<GaugeDistribution>,
    }

    public struct GaugeDistribution has copy, drop {
        gauge_id: u64,
        amount: u64,
    }

    // Initialize controller
    fun init(ctx: &mut TxContext) {
        let controller = Controller {
            id: object::new(ctx),
            next_gauge_id: 1,
            gauges: table::new<u64, GaugeInfo>(ctx),
            current_epoch: 0,
            total_gauges: 0,
            treasury_balance: balance::zero(),
        };

        let controller_id = object::id(&controller);

        event::emit(ControllerCreated {
            controller_id,
        });

        transfer::share_object(controller);
    }

    // Register a new gauge
    public fun register_gauge(
        controller: &mut Controller,
        kind: u8,
        addr: address,
        _ctx: &mut TxContext
    ): GaugeId {
        assert!(
            kind == GAUGE_KIND_SP ||
            kind == GAUGE_KIND_LP_ABSTRACT ||
            kind == GAUGE_KIND_VALIDATOR,
            E_INVALID_GAUGE_KIND
        );

        let gauge_id = controller.next_gauge_id;
        controller.next_gauge_id = gauge_id + 1;

        let gauge_info = GaugeInfo {
            id: GaugeId { raw: gauge_id },
            kind,
            addr,
        };

        table::add(&mut controller.gauges, gauge_id, gauge_info);
        controller.total_gauges = controller.total_gauges + 1;

        event::emit(GaugeRegistered {
            gauge_id,
            kind,
            addr,
        });

        GaugeId { raw: gauge_id }
    }

    // Helper struct for vote choices
    public struct VoteChoice has copy, drop {
        gauge_id: u64,
        weight_bps: u16,
    }

    // Cast votes for current epoch
    public fun vote(
        controller: &mut Controller,
        lock: &Lock,
        choices: vector<VoteChoice>, // Vector of vote choices
        clock: &Clock,
        ctx: &mut TxContext
    ) {
        let voter = tx_context::sender(ctx);
        let (lock_owner, _, _, _) = ve_lfs::get_lock_info(lock);
        assert!(voter == lock_owner, E_UNAUTHORIZED);

        let current_epoch = emissions::current_epoch(clock);
        let current_time = clock::timestamp_ms(clock);

        // Get ve power at current time
        let ve_power = ve_lfs::balance_of_at(lock, current_time);
        assert!(ve_power > 0, E_UNAUTHORIZED);

        // Check if already voted this epoch
        let vote_key = get_user_vote_key(current_epoch, voter);
        assert!(!df::exists_(&controller.id, vote_key), E_ALREADY_VOTED_THIS_EPOCH);

        // Validate vote weights
        let mut total_weight: u32 = 0;
        let mut gauge_votes = vector::empty<GaugeVote>();
        let mut i = 0;

        while (i < vector::length(&choices)) {
            let choice = vector::borrow(&choices, i);
            let gauge_id = choice.gauge_id;
            let weight_bps = choice.weight_bps;

            // Verify gauge exists
            assert!(table::contains(&controller.gauges, gauge_id), E_GAUGE_NOT_FOUND);

            total_weight = total_weight + (weight_bps as u32);

            vector::push_back(&mut gauge_votes, GaugeVote {
                gauge_id,
                weight_bps,
            });

            i = i + 1;
        };

        assert!(total_weight == (BPS_TOTAL as u32), E_WEIGHTS_SUM_INVALID);

        // Store user vote record
        let vote_record = VoteRecord {
            epoch: current_epoch,
            total_ve_power: ve_power,
            votes: gauge_votes,
        };

        df::add(&mut controller.id, vote_key, vote_record);

        // Update epoch weights
        let epoch_weights_key = get_epoch_weights_key(current_epoch);
        if (!df::exists_(&controller.id, epoch_weights_key)) {
            df::add(&mut controller.id, epoch_weights_key, table::new<u64, u64>(ctx));
        };

        let epoch_weights = df::borrow_mut<vector<u8>, Table<u64, u64>>(
            &mut controller.id,
            epoch_weights_key
        );

        // Apply votes to gauge weights
        let mut j = 0;
        while (j < vector::length(&gauge_votes)) {
            let vote = vector::borrow(&gauge_votes, j);
            let weighted_power = (ve_power as u128) * (vote.weight_bps as u128) / (BPS_TOTAL as u128);

            if (table::contains(epoch_weights, vote.gauge_id)) {
                let current_weight = table::remove(epoch_weights, vote.gauge_id);
                table::add(epoch_weights, vote.gauge_id, current_weight + (weighted_power as u64));
            } else {
                table::add(epoch_weights, vote.gauge_id, weighted_power as u64);
            };

            j = j + 1;
        };

        event::emit(VoteCast {
            voter,
            epoch: current_epoch,
            ve_power,
            votes: gauge_votes,
        });
    }

    // Checkpoint epoch (finalize voting for the epoch)
    public fun checkpoint_epoch(
        controller: &mut Controller,
        clock: &Clock
    ) {
        let current_epoch = emissions::current_epoch(clock);

        // Only checkpoint if we've moved to a new epoch
        if (current_epoch > controller.current_epoch) {
            controller.current_epoch = current_epoch;

            event::emit(EpochCheckpointed {
                epoch: current_epoch,
                total_gauges: controller.total_gauges,
            });
        };
    }

    // Distribute emissions for an epoch
    public fun distribute_epoch(
        controller: &mut Controller,
        emissions_state: &mut EmissionsState,
        emissions_cap: &mut EmissionsCap,
        treasury_cap: &mut TreasuryCap<LFS>,
        epoch: u64,
        clock: &Clock,
        ctx: &mut TxContext
    ) {
        // Mint epoch emission
        let epoch_emission = emissions::mint_epoch_emission(
            emissions_state,
            emissions_cap,
            treasury_cap,
            epoch,
            clock,
            ctx
        );

        let total_emission = coin::value(&epoch_emission);
        let mut distributions = vector::empty<GaugeDistribution>();

        // Get epoch weights
        let epoch_weights_key = get_epoch_weights_key(epoch);
        if (!df::exists_(&controller.id, epoch_weights_key)) {
            // No votes for this epoch - route to treasury
            balance::join(&mut controller.treasury_balance, coin::into_balance(epoch_emission));
            return
        };

        let epoch_weights = df::borrow<vector<u8>, Table<u64, u64>>(
            &controller.id,
            epoch_weights_key
        );

        // Calculate total weight
        let mut total_weight: u128 = 0;
        let mut gauge_ids = vector::empty<u64>();

        let mut i = 0;
        while (i < controller.total_gauges) {
            let gauge_id = i + 1; // Gauge IDs start from 1
            if (table::contains(&controller.gauges, gauge_id) &&
                table::contains(epoch_weights, gauge_id)) {
                let weight = *table::borrow(epoch_weights, gauge_id);
                total_weight = total_weight + (weight as u128);
                vector::push_back(&mut gauge_ids, gauge_id);
            };
            i = i + 1;
        };

        // Distribute proportionally
        let mut remaining_emission = epoch_emission;
        let mut distributed_amount = 0u64;

        let mut j = 0;
        while (j < vector::length(&gauge_ids)) {
            let gauge_id = *vector::borrow(&gauge_ids, j);
            let weight = *table::borrow(epoch_weights, gauge_id);

            let allocation = if (j == vector::length(&gauge_ids) - 1) {
                // Last gauge gets remaining to handle rounding
                coin::value(&remaining_emission)
            } else {
                (((weight as u128) * (total_emission as u128)) / total_weight) as u64
            };

            if (allocation > 0) {
                let gauge_emission = coin::split(&mut remaining_emission, allocation, ctx);

                // Transfer to gauge address - gauges will handle via their own notify_reward
                // when they receive the LFS tokens
                let gauge_info = table::borrow(&controller.gauges, gauge_id);
                transfer::public_transfer(gauge_emission, gauge_info.addr);

                distributed_amount = distributed_amount + allocation;

                vector::push_back(&mut distributions, GaugeDistribution {
                    gauge_id,
                    amount: allocation,
                });
            };

            j = j + 1;
        };

        // Handle any remaining emission (rounding remainder)
        if (coin::value(&remaining_emission) > 0) {
            balance::join(&mut controller.treasury_balance, coin::into_balance(remaining_emission));
        } else {
            coin::destroy_zero(remaining_emission);
        };

        event::emit(EmissionsDistributed {
            epoch,
            total_emission,
            distributions,
        });
    }

    // Helper functions for dynamic field keys
    fun get_epoch_weights_key(epoch: u64): vector<u8> {
        let mut key = b"epoch_weights_";
        let epoch_bytes = bcs::to_bytes(&epoch);
        vector::append(&mut key, epoch_bytes);
        key
    }

    fun get_user_vote_key(epoch: u64, user: address): vector<u8> {
        let mut key = b"user_votes_";
        let epoch_bytes = bcs::to_bytes(&epoch);
        vector::append(&mut key, epoch_bytes);
        vector::append(&mut key, b"_");
        let user_bytes = bcs::to_bytes(&user);
        vector::append(&mut key, user_bytes);
        key
    }

    // View functions
    public fun get_gauge_info(controller: &Controller, gauge_id: u64): (u8, address) {
        assert!(table::contains(&controller.gauges, gauge_id), E_GAUGE_NOT_FOUND);
        let gauge = table::borrow(&controller.gauges, gauge_id);
        (gauge.kind, gauge.addr)
    }

    public fun total_gauges(controller: &Controller): u64 {
        controller.total_gauges
    }

    public fun current_epoch(controller: &Controller): u64 {
        controller.current_epoch
    }

    /// Get treasury balance of unallocated LFS emissions
    public fun treasury_balance(controller: &Controller): u64 {
        balance::value(&controller.treasury_balance)
    }

    // Test-only functions
    #[test_only]
    public fun init_for_testing(ctx: &mut TxContext) {
        init(ctx);
    }

    #[test_only]
    public fun create_test_gauge_id(raw: u64): GaugeId {
        GaugeId { raw }
    }

    #[test_only]
    public fun create_vote_choice(gauge_id: u64, weight_bps: u16): VoteChoice {
        VoteChoice { gauge_id, weight_bps }
    }
}