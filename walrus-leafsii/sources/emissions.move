/// LFS Emissions Schedule Manager
///
/// This module implements a time-based emissions schedule for LFS token distribution
/// with a 10% annual decay rate. Emissions are distributed weekly to gauges based on
/// ve-LFS holder votes.
///
/// Emissions Model:
/// - Base Period: 1 week (604,800,000 milliseconds)
/// - Initial Rate: 98,000 LFS per week
/// - Decay Rate: 10% per year (0.9 multiplier annually)
/// - Start Date: January 1, 2025, 00:00:00 UTC
///
/// Calculation Formula:
/// ```
/// weekly_emission = 98,000 * (0.9 ^ year_index)
/// where year_index = (epoch - 1) / 52
/// ```
///
/// Example Schedule:
/// - Year 1 (Epochs 1-52): 98,000 LFS/week
/// - Year 2 (Epochs 53-104): 88,200 LFS/week  (98,000 * 0.9)
/// - Year 3 (Epochs 105-156): 79,380 LFS/week (88,200 * 0.9)
/// - And so on...
///
/// Key Features:
/// 1. Epoch-Based Distribution:
///    - Each epoch = 1 week
///    - Epochs are numbered sequentially from 0
///    - Epoch 0 has no emissions (initialization epoch)
///
/// 2. One-Time Minting:
///    - Each epoch can only be minted once
///    - Prevents double-spending and ensures predictable supply
///    - Enforced through EmissionsState tracking
///
/// 3. Flexible Timing:
///    - Emissions can be minted anytime during or after the target epoch
///    - No requirement to mint immediately at epoch start
///    - Allows for governance delays or system pauses
///
/// Integration:
/// - Works with gauge_controller for vote-directed distribution
/// - Uses EmissionsCap to enforce total supply cap
/// - Emits events for transparency and off-chain tracking
module leafsii::emissions {
    use sui::coin::{Coin, TreasuryCap};
    use sui::clock::{Self, Clock};
    use sui::event;
    use sui::object::{new, id};

    use leafsii::lfs_token::{Self, LFS, EmissionsCap};

    // Error codes
    const E_EPOCH_ALREADY_MINTED: u64 = 1;
    const E_INVALID_EPOCH: u64 = 2;

    // Constants
    const WEEK_MS: u64 = 604800000; // 7 * 24 * 3600 * 1000 milliseconds
    const INITIAL_WEEKLY_EMISSION: u64 = 98_000_000_000_000; // 98,000 LFS with 9 decimals
    const WEEKS_PER_YEAR: u64 = 52;

    // Fixed epoch start - set to a reasonable timestamp for testing
    // In production, this should be set to protocol launch time
    const EPOCH_START_MS: u64 = 1735689600000; // Jan 1, 2025 00:00:00 UTC

    // Decay factor: 0.9 represented as fraction 9/10
    // We'll use integer arithmetic: new_emission = old_emission * 9 / 10
    const DECAY_NUMERATOR: u64 = 9;
    const DECAY_DENOMINATOR: u64 = 10;

    // Global emissions state
    public struct EmissionsState has key, store {
        id: UID,
        epoch_last_minted: u64,
        total_emitted: u64,
    }

    // Events
    public struct EpochEmissionMinted has copy, drop {
        epoch: u64,
        amount: u64,
        total_emitted: u64,
    }

    public struct EmissionsStateCreated has copy, drop {
        state_id: ID,
        epoch_start_ms: u64,
    }

    // Initialize emissions state
    fun init(ctx: &mut TxContext) {
        let state = EmissionsState {
            id: new(ctx),
            epoch_last_minted: 0, // Start from epoch 0 (no emissions minted yet)
            total_emitted: 0,
        };

        let state_id = id(&state);

        event::emit(EmissionsStateCreated {
            state_id,
            epoch_start_ms: EPOCH_START_MS,
        });

        transfer::share_object(state);
    }

    // Get current epoch based on timestamp
    public fun current_epoch(clock: &Clock): u64 {
        let current_time = clock::timestamp_ms(clock);
        if (current_time < EPOCH_START_MS) {
            return 0
        };

        let elapsed_ms = current_time - EPOCH_START_MS;
        elapsed_ms / WEEK_MS
    }

    // Calculate emission amount for a specific epoch
    public fun emission_for_epoch(epoch: u64): u64 {
        if (epoch == 0) {
            return 0 // No emissions for epoch 0
        };

        let year_index = (epoch - 1) / WEEKS_PER_YEAR; // Epochs 1-52 are year 0

        // Calculate 98,000 * (0.9^year_index)
        // Use integer arithmetic to avoid floating point
        let mut emission = INITIAL_WEEKLY_EMISSION;
        let mut year = 0;

        while (year < year_index) {
            emission = emission * DECAY_NUMERATOR / DECAY_DENOMINATOR;
            year = year + 1;
        };

        emission
    }

    // Mint emission for a specific epoch (can only be called once per epoch)
    public(package) fun mint_epoch_emission(
        state: &mut EmissionsState,
        cap: &mut EmissionsCap,
        treasury_cap: &mut TreasuryCap<LFS>,
        epoch: u64,
        clock: &Clock,
        ctx: &mut TxContext
    ): Coin<LFS> {
        // Verify we're in the correct epoch or later
        let current_epoch_num = current_epoch(clock);
        assert!(epoch <= current_epoch_num, E_INVALID_EPOCH);
        assert!(epoch > 0, E_INVALID_EPOCH); // No emissions for epoch 0

        // Ensure this epoch hasn't been minted yet
        assert!(epoch > state.epoch_last_minted, E_EPOCH_ALREADY_MINTED);

        // Calculate emission for this epoch
        let emission_amount = emission_for_epoch(epoch);

        // Update state
        state.epoch_last_minted = epoch;
        state.total_emitted = state.total_emitted + emission_amount;

        // Mint the tokens
        let emission_coin = lfs_token::mint_emissions(
            cap,
            emission_amount,
            treasury_cap,
            ctx
        );

        // Emit event
        event::emit(EpochEmissionMinted {
            epoch,
            amount: emission_amount,
            total_emitted: state.total_emitted,
        });

        emission_coin
    }

    // Get epoch start timestamp
    public fun epoch_start_time(): u64 {
        EPOCH_START_MS
    }

    // Get epoch duration in milliseconds
    public fun epoch_duration(): u64 {
        WEEK_MS
    }

    // Get timestamp for a specific epoch start
    public fun epoch_start_time_for(epoch: u64): u64 {
        EPOCH_START_MS + (epoch * WEEK_MS)
    }

    // Get timestamp for a specific epoch end
    public fun epoch_end_time_for(epoch: u64): u64 {
        epoch_start_time_for(epoch + 1) - 1
    }

    // Check if an epoch has been minted
    public fun is_epoch_minted(state: &EmissionsState, epoch: u64): bool {
        epoch <= state.epoch_last_minted
    }

    // View functions
    public fun get_emissions_info(state: &EmissionsState): (u64, u64) {
        (state.epoch_last_minted, state.total_emitted)
    }

    public fun initial_weekly_emission(): u64 {
        INITIAL_WEEKLY_EMISSION
    }

    public fun weeks_per_year(): u64 {
        WEEKS_PER_YEAR
    }

    // Calculate year index for an epoch
    public fun year_index_for_epoch(epoch: u64): u64 {
        if (epoch == 0) {
            return 0
        };
        (epoch - 1) / WEEKS_PER_YEAR
    }

    // Test-only functions
    #[test_only]
    public fun init_for_testing(ctx: &mut TxContext) {
        init(ctx);
    }

    #[test_only]
    public fun create_test_emissions_state(ctx: &mut TxContext): EmissionsState {
        EmissionsState {
            id: new(ctx),
            epoch_last_minted: 0,
            total_emitted: 0,
        }
    }

    #[test_only]
    public fun test_epoch_calculation(timestamp_ms: u64): u64 {
        if (timestamp_ms < EPOCH_START_MS) {
            return 0
        };
        let elapsed_ms = timestamp_ms - EPOCH_START_MS;
        elapsed_ms / WEEK_MS
    }
}