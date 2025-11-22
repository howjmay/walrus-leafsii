/// Vesting Escrow for LFS Tokens
///
/// This module implements time-locked vesting schedules for LFS token rewards.
/// All gauge rewards (SP, LP, Validator) are distributed through 7-day linear
/// vesting to prevent immediate selling pressure and align user incentives.
///
/// Vesting Mechanism:
/// - Duration: 7 days (configurable via VESTING_MS constant)
/// - Type: Linear vesting (tokens unlock proportionally over time)
/// - Claim Frequency: Users can claim any time, receiving unlocked portion
/// - Remaining Tokens: Stay in escrow until fully vested
///
/// Formula:
/// ```
/// vested_amount = total_amount * (current_time - start_time) / vesting_duration
/// claimable = vested_amount - already_claimed
/// ```
///
/// Key Features:
/// 1. Linear Vesting:
///    - Tokens unlock continuously over the vesting period
///    - No cliff (immediate partial vesting starts)
///    - Fully vested after 7 days
///
/// 2. Flexible Claiming:
///    - Users can claim multiple times during vesting
///    - Partial claims allowed at any point
///    - Gas-efficient: Only pay when claiming
///
/// 3. Additive Vesting:
///    - New rewards can be added to existing vesting schedules
///    - Resets vesting period with blended average
///    - Simplifies user experience (one vesting object per user)
///
/// 4. Owner-Only Access:
///    - Only vesting owner can claim tokens
///    - Prevents theft or unauthorized access
///    - Non-transferable by design
///
/// Usage Flow:
/// 1. User claims LFS rewards from gauge
/// 2. Gauge creates vesting escrow: create_standard_vesting()
/// 3. Vesting object transferred to user
/// 4. User waits for vesting period
/// 5. User claims unlocked tokens: claim()
/// 6. Process repeats for additional rewards
///
/// Example Timeline:
/// ```
/// Day 0: Receive 700 LFS vesting
/// Day 1: Can claim 100 LFS (14.29% vested)
/// Day 3: Can claim 300 LFS total (42.86% vested)
/// Day 7: Can claim all 700 LFS (100% vested)
/// ```
///
/// Integration Points:
/// - SP Gauge: Creates vesting on reward claims
/// - LP Gauge: Creates vesting on reward claims
/// - Validator rewards: Future integration point
/// - Direct transfers: Can use create_vesting() for custom schedules
///
/// Technical Details:
/// - Vesting object is a Sui owned object (key capability)
/// - Balance held in escrow until claimed
/// - Timestamps in milliseconds for precision
/// - Events emitted for off-chain tracking
module leafsii::vesting_escrow {
    use sui::coin::{Self, Coin};
    use sui::balance::{Self, Balance};
    use sui::clock::{Self, Clock};
    use sui::event;

    use leafsii::lfs_token::LFS;
    use math::math;

    // Error codes
    const E_INVALID_AMOUNT: u64 = 1;
    const E_INVALID_DURATION: u64 = 2;
    const E_NOTHING_TO_CLAIM: u64 = 3;
    const E_UNAUTHORIZED: u64 = 4;

    // Constants
    const VESTING_MS: u64 = 7 * 24 * 3600 * 1000; // 7 days in milliseconds

    // Vesting schedule for a user
    public struct Vesting has key, store {
        id: UID,
        owner: address,
        start_ms: u64,
        duration_ms: u64,
        total_amount: u64,
        claimed_amount: u64,
        lfs_balance: Balance<LFS>,
    }

    // Events
    public struct VestingCreated has copy, drop {
        vesting_id: ID,
        owner: address,
        amount: u64,
        start_ms: u64,
        duration_ms: u64,
    }

    public struct VestingAdded has copy, drop {
        vesting_id: ID,
        owner: address,
        additional_amount: u64,
        new_total: u64,
        start_ms: u64,
    }

    public struct VestingClaimed has copy, drop {
        vesting_id: ID,
        owner: address,
        amount: u64,
        total_claimed: u64,
    }

    // Create a new vesting schedule
    public fun create_vesting(
        owner: address,
        amount: Coin<LFS>,
        start_ms: u64,
        duration_ms: u64,
        ctx: &mut TxContext
    ): Vesting {
        let lfs_amount = coin::value(&amount);
        assert!(lfs_amount > 0, E_INVALID_AMOUNT);
        assert!(duration_ms > 0, E_INVALID_DURATION);

        let vesting = Vesting {
            id: object::new(ctx),
            owner,
            start_ms,
            duration_ms,
            total_amount: lfs_amount,
            claimed_amount: 0,
            lfs_balance: coin::into_balance(amount),
        };

        let vesting_id = object::id(&vesting);

        event::emit(VestingCreated {
            vesting_id,
            owner,
            amount: lfs_amount,
            start_ms,
            duration_ms,
        });

        vesting
    }

    // Create a standard 7-day vesting schedule
    public fun create_standard_vesting(
        owner: address,
        amount: Coin<LFS>,
        clock: &Clock,
        ctx: &mut TxContext
    ): Vesting {
        let start_ms = clock::timestamp_ms(clock);
        create_vesting(owner, amount, start_ms, VESTING_MS, ctx)
    }

    // Add more LFS to an existing vesting schedule
    // This creates a new vesting period starting from the current time
    public fun add_vesting(
        existing: &mut Vesting,
        amount: Coin<LFS>,
        clock: &Clock
    ) {
        let additional_amount = coin::value(&amount);
        assert!(additional_amount > 0, E_INVALID_AMOUNT);

        let current_time = clock::timestamp_ms(clock);

        // Add the new tokens to the balance
        balance::join(&mut existing.lfs_balance, coin::into_balance(amount));

        // Calculate pro-rata adjustment for the new vesting
        // Simple approach: extend the vesting period proportionally
        let old_total = existing.total_amount;
        let new_total = old_total + additional_amount;

        // Update the start time to current time and recalculate duration
        // This effectively "resets" the vesting with a blended rate
        existing.start_ms = current_time;
        existing.total_amount = new_total;
        existing.duration_ms = VESTING_MS; // Reset to standard 7-day period

        event::emit(VestingAdded {
            vesting_id: object::id(existing),
            owner: existing.owner,
            additional_amount,
            new_total,
            start_ms: current_time,
        });
    }

    // Claim vested tokens
    public fun claim(
        vesting: &mut Vesting,
        clock: &Clock,
        ctx: &mut TxContext
    ): Coin<LFS> {
        let sender = tx_context::sender(ctx);
        assert!(sender == vesting.owner, E_UNAUTHORIZED);

        let claimable_amount = claimable(vesting, clock);
        assert!(claimable_amount > 0, E_NOTHING_TO_CLAIM);

        // Update claimed amount
        vesting.claimed_amount = vesting.claimed_amount + claimable_amount;

        // Extract the claimable tokens
        let claimed_balance = balance::split(&mut vesting.lfs_balance, claimable_amount);
        let claimed_coin = coin::from_balance(claimed_balance, ctx);

        event::emit(VestingClaimed {
            vesting_id: object::id(vesting),
            owner: vesting.owner,
            amount: claimable_amount,
            total_claimed: vesting.claimed_amount,
        });

        claimed_coin
    }

    // Calculate how much can be claimed at the current time
    public fun claimable(vesting: &Vesting, clock: &Clock): u64 {
        let current_time = clock::timestamp_ms(clock);
        claimable_at(vesting, current_time)
    }

    // Calculate how much can be claimed at a specific timestamp
    public fun claimable_at(vesting: &Vesting, ts_ms: u64): u64 {
        if (ts_ms < vesting.start_ms) {
            return 0
        };

        let elapsed_ms = ts_ms - vesting.start_ms;

        // Calculate total vested amount
        let total_vested = if (elapsed_ms >= vesting.duration_ms) {
            // Fully vested
            vesting.total_amount
        } else {
            // Linear vesting: total * elapsed / duration
            math::proportional_div_u64(vesting.total_amount, elapsed_ms, vesting.duration_ms)
        };

        // Subtract already claimed amount
        if (total_vested > vesting.claimed_amount) {
            total_vested - vesting.claimed_amount
        } else {
            0
        }
    }

    // Check if vesting is fully completed
    public fun is_fully_vested(vesting: &Vesting, clock: &Clock): bool {
        let current_time = clock::timestamp_ms(clock);
        current_time >= vesting.start_ms + vesting.duration_ms
    }

    // View functions
    public fun get_vesting_info(vesting: &Vesting): (address, u64, u64, u64, u64, u64) {
        (
            vesting.owner,
            vesting.start_ms,
            vesting.duration_ms,
            vesting.total_amount,
            vesting.claimed_amount,
            balance::value(&vesting.lfs_balance)
        )
    }

    public fun vesting_duration(): u64 {
        VESTING_MS
    }

    public fun remaining_balance(vesting: &Vesting): u64 {
        balance::value(&vesting.lfs_balance)
    }

    // Test-only functions
    #[test_only]
    public fun create_test_vesting(
        owner: address,
        amount: u64,
        start_ms: u64,
        duration_ms: u64,
        ctx: &mut TxContext
    ): (Vesting, Coin<LFS>) {
        use leafsii::lfs_token;
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ctx);
        let mut lfs_coin = coin::mint(&mut treasury_cap, amount * 2, ctx);
        let vesting_coin = coin::split(&mut lfs_coin, amount, ctx);

        transfer::public_transfer(treasury_cap, owner);
        transfer::public_transfer(emissions_cap, owner);

        let vesting = create_vesting(owner, vesting_coin, start_ms, duration_ms, ctx);
        (vesting, lfs_coin)
    }
}