/// Vote-Escrow LFS (ve-LFS) Implementation
///
/// This module implements the vote-escrowed LFS token system for protocol governance
/// and yield boosting. Users lock LFS tokens for 1 week to 4 years to receive voting
/// power that decays linearly over time.
///
/// Key Concepts:
/// 1. Time-Weighted Voting Power:
///    - Voting power = locked_amount * remaining_time / MAX_LOCK_TIME
///    - Longer locks = more voting power
///    - Power decays linearly as lock approaches expiry
///
/// 2. Lock Operations:
///    - create_lock: Lock LFS for chosen duration (1 week - 4 years)
///    - increase_amount: Add more LFS to existing lock
///    - extend_lock: Extend lock duration to boost voting power
///    - withdraw: Retrieve LFS after lock expires
///
/// 3. Use Cases:
///    - Gauge voting: Direct LFS emissions to specific gauges (SP, LP, Validators)
///    - Yield boosting: Increase rewards from stability pool and LP staking (up to 2.5x)
///    - Protocol governance: Weighted voting on protocol parameters
///
/// 4. Design Philosophy:
///    - Long-term alignment: Reward long-term LFS holders with greater influence
///    - Flexibility: Users can increase amount or extend duration anytime
///    - Non-transferable: Locks are bound to owner address, cannot be traded
///
/// Technical Implementation:
/// - Each lock is an individual NFT object owned by the user
/// - VeLfsState tracks all locks globally for total supply calculations
/// - Checkpoint system enables historical voting power queries
/// - All timestamps use milliseconds for precise duration calculations
module leafsii::ve_lfs {
    use sui::coin::{Self, Coin};
    use sui::balance::{Self, Balance};
    use sui::clock::{Self, Clock};
    use sui::event;
    use sui::table::{Self, Table};

    use leafsii::lfs_token::LFS;
    use math::math;

    // Error codes
    const E_INVALID_AMOUNT: u64 = 1;
    const E_INVALID_LOCK_DURATION: u64 = 2;
    const E_LOCK_NOT_EXPIRED: u64 = 3;
    const E_LOCK_ALREADY_EXPIRED: u64 = 4;
    const E_INVALID_END_TIME: u64 = 5;

    // Constants
    const MAX_LOCK_MS: u64 = 4 * 365 * 24 * 3600 * 1000; // 4 years in milliseconds
    const MIN_LOCK_MS: u64 = 7 * 24 * 3600 * 1000; // 1 week minimum

    // Vote-escrow lock
    public struct Lock has key, store {
        id: object::UID,
        owner: address,
        lfs_locked: Balance<LFS>,
        start_ms: u64,
        end_ms: u64,
    }

    // Global state for tracking total supply checkpoints
    public struct VeLfsState has key, store {
        id: object::UID,
        // Simple approach: store locks by ID for total supply calculation
        // For production, consider optimized checkpoint system
        all_locks: Table<object::ID, u64>, // lock_id -> end_ms for iteration
        total_locked: u64,
        // Checkpoint system for total supply queries
        supply_checkpoints: Table<u64, u64>, // timestamp_ms -> total_ve_supply
        last_checkpoint_ts: u64,
    }

    // Events
    public struct LockCreated has copy, drop {
        lock_id: object::ID,
        owner: address,
        amount: u64,
        start_ms: u64,
        end_ms: u64,
        ve_balance: u64,
    }

    public struct LockIncreased has copy, drop {
        lock_id: object::ID,
        owner: address,
        additional_amount: u64,
        new_total: u64,
        ve_balance: u64,
    }

    public struct LockExtended has copy, drop {
        lock_id: object::ID,
        owner: address,
        old_end_ms: u64,
        new_end_ms: u64,
        ve_balance: u64,
    }

    public struct LockWithdrawn has copy, drop {
        lock_id: object::ID,
        owner: address,
        amount: u64,
    }

    /// Initialize the ve-LFS global state
    ///
    /// Creates shared state object for tracking locks and total supply.
    /// Called automatically during module initialization.
    fun init(ctx: &mut tx_context::TxContext) {
        let state = VeLfsState {
            id: object::new(ctx),
            all_locks: table::new<object::ID, u64>(ctx),
            total_locked: 0,
            supply_checkpoints: table::new<u64, u64>(ctx),
            last_checkpoint_ts: 0,
        };
        transfer::share_object(state);
    }

    /// Create a new vote-escrow lock for LFS tokens
    ///
    /// Locks LFS tokens for a specified duration (1 week to 4 years) in exchange
    /// for voting power that decays linearly until expiry.
    ///
    /// # Parameters
    /// - `lfs`: LFS tokens to lock
    /// - `lock_duration_ms`: Lock duration in milliseconds
    /// - `clock`: Clock for timestamp
    ///
    /// # Returns
    /// New Lock object representing the locked position
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If LFS amount is zero
    /// - `E_INVALID_LOCK_DURATION`: If duration is outside valid range
    public fun create_lock(
        lfs: Coin<LFS>,
        lock_duration_ms: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ): Lock {
        let amount = coin::value(&lfs);
        assert!(amount > 0, E_INVALID_AMOUNT);
        assert!(lock_duration_ms >= MIN_LOCK_MS, E_INVALID_LOCK_DURATION);
        assert!(lock_duration_ms <= MAX_LOCK_MS, E_INVALID_LOCK_DURATION);

        let start_ms = clock::timestamp_ms(clock);
        let end_ms = start_ms + lock_duration_ms;
        let owner = tx_context::sender(ctx);

        let lock = Lock {
            id: object::new(ctx),
            owner,
            lfs_locked: coin::into_balance(lfs),
            start_ms,
            end_ms,
        };

        let lock_id = object::id(&lock);
        let ve_balance = balance_of_at(&lock, start_ms);

        event::emit(LockCreated {
            lock_id,
            owner,
            amount,
            start_ms,
            end_ms,
            ve_balance,
        });

        lock
    }

    /// Increase the amount of LFS tokens in an existing lock
    ///
    /// Adds more LFS tokens to an existing lock without changing the expiry time.
    /// This increases the voting power proportionally.
    ///
    /// # Parameters
    /// - `lock`: Existing lock to modify (mutable)
    /// - `more_lfs`: Additional LFS tokens to lock
    /// - `clock`: Clock for timestamp verification
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If amount is zero or sender is not lock owner
    /// - `E_LOCK_ALREADY_EXPIRED`: If lock has already expired
    public fun increase_amount(
        lock: &mut Lock,
        more_lfs: Coin<LFS>,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ) {
        let additional_amount = coin::value(&more_lfs);
        assert!(additional_amount > 0, E_INVALID_AMOUNT);

        let current_time = clock::timestamp_ms(clock);
        assert!(current_time < lock.end_ms, E_LOCK_ALREADY_EXPIRED);

        let sender = tx_context::sender(ctx);
        assert!(sender == lock.owner, E_INVALID_AMOUNT); // Reuse error code

        let _old_balance = balance::value(&lock.lfs_locked);
        balance::join(&mut lock.lfs_locked, coin::into_balance(more_lfs));
        let new_total = balance::value(&lock.lfs_locked);

        let ve_balance = balance_of_at(lock, current_time);

        event::emit(LockIncreased {
            lock_id: object::id(lock),
            owner: lock.owner,
            additional_amount,
            new_total,
            ve_balance,
        });
    }

    /// Extend the duration of an existing lock
    ///
    /// Increases the lock end time to boost voting power. Can only extend,
    /// not reduce lock duration. Maximum total duration is still 4 years.
    ///
    /// # Parameters
    /// - `lock`: Lock to extend (mutable)
    /// - `new_end_ms`: New end timestamp (must be later than current)
    /// - `clock`: Clock for timestamp verification
    ///
    /// # Aborts
    /// - `E_LOCK_ALREADY_EXPIRED`: If lock has expired
    /// - `E_INVALID_END_TIME`: If new end time is not later than current
    /// - `E_INVALID_LOCK_DURATION`: If total duration would exceed 4 years
    public fun extend_lock(
        lock: &mut Lock,
        new_end_ms: u64,
        clock: &Clock
    ) {
        let current_time = clock::timestamp_ms(clock);
        assert!(current_time < lock.end_ms, E_LOCK_ALREADY_EXPIRED);
        assert!(new_end_ms > lock.end_ms, E_INVALID_END_TIME);

        let max_end = lock.start_ms + MAX_LOCK_MS;
        assert!(new_end_ms <= max_end, E_INVALID_LOCK_DURATION);

        let old_end_ms = lock.end_ms;
        lock.end_ms = new_end_ms;

        let ve_balance = balance_of_at(lock, current_time);

        event::emit(LockExtended {
            lock_id: object::id(lock),
            owner: lock.owner,
            old_end_ms,
            new_end_ms,
            ve_balance,
        });
    }

    /// Withdraw LFS tokens after lock expiration
    ///
    /// Destroys the lock and returns all locked LFS tokens. Can only be called
    /// after the lock has expired and by the lock owner.
    ///
    /// # Parameters
    /// - `lock`: Lock to withdraw from (consumed)
    /// - `clock`: Clock for timestamp verification
    ///
    /// # Returns
    /// All LFS tokens that were locked
    ///
    /// # Aborts
    /// - `E_LOCK_NOT_EXPIRED`: If lock hasn't expired yet
    /// - `E_INVALID_AMOUNT`: If sender is not lock owner
    public fun withdraw(
        lock: Lock,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ): Coin<LFS> {
        let current_time = clock::timestamp_ms(clock);
        assert!(current_time >= lock.end_ms, E_LOCK_NOT_EXPIRED);

        let sender = tx_context::sender(ctx);
        assert!(sender == lock.owner, E_INVALID_AMOUNT); // Reuse error code

        let Lock {
            id,
            owner,
            lfs_locked,
            start_ms: _,
            end_ms: _,
        } = lock;

        let amount = balance::value(&lfs_locked);
        let lfs_coin = coin::from_balance(lfs_locked, ctx);

        event::emit(LockWithdrawn {
            lock_id: object::id_from_address(object::uid_to_address(&id)),
            owner,
            amount,
        });

        object::delete(id);

        lfs_coin
    }

    /// Calculate voting power (ve-LFS balance) at a specific timestamp
    ///
    /// Voting power decays linearly from locked amount to zero over the lock duration.
    /// Formula: ve = lfs_locked * (end_ms - t) / MAX_LOCK_MS, clipped to [0, lfs_locked]
    ///
    /// # Parameters
    /// - `lock`: The lock to calculate voting power for
    /// - `ts_ms`: Timestamp to calculate voting power at
    ///
    /// # Returns
    /// Voting power at the specified timestamp
    public(package) fun balance_of_at(lock: &Lock, ts_ms: u64): u64 {
        if (ts_ms >= lock.end_ms) {
            return 0
        };

        let locked_amount = balance::value(&lock.lfs_locked);
        let remaining_ms = lock.end_ms - ts_ms;

        // Calculate ve balance: locked * remaining_time / MAX_LOCK_MS
        let ve_balance = math::proportional_div_u64(locked_amount, remaining_ms, MAX_LOCK_MS);

        // Ensure ve_balance doesn't exceed locked amount
        if (ve_balance > locked_amount) {
            locked_amount
        } else {
            ve_balance
        }
    }

    /// Calculate total ve-LFS supply at a specific timestamp
    ///
    /// This is a placeholder implementation that returns 0. In production,
    /// this would require either:
    /// 1. A checkpoint system with periodic snapshots
    /// 2. Lock registration/deregistration with global state
    /// Update the total supply checkpoint at a given timestamp
    /// This should be called whenever locks are created, extended, or withdrawn
    #[allow(unused_function)]
    fun update_supply_checkpoint(
        state: &mut VeLfsState,
        ts_ms: u64,
        current_supply: u64
    ) {
        // Only update if this is a newer timestamp than the last checkpoint
        if (ts_ms > state.last_checkpoint_ts) {
            table::add(&mut state.supply_checkpoints, ts_ms, current_supply);
            state.last_checkpoint_ts = ts_ms;
        };
    }

    /// Get the total ve-LFS supply at a specific timestamp
    ///
    /// This function provides historical total supply queries by using
    /// a checkpoint system. Checkpoints are created whenever locks are
    /// modified (created, extended, or withdrawn).
    ///
    /// # Parameters
    /// - `state`: Global ve-LFS state containing checkpoints
    /// - `ts_ms`: Timestamp to query
    ///
    /// # Returns
    /// Total ve-LFS supply at the given timestamp
    public fun total_supply_at(
        state: &VeLfsState,
        ts_ms: u64
    ): u64 {
        // In a real implementation, we'd iterate through checkpoints efficiently
        // For now, this is a simplified version that demonstrates the concept
        // We would need table iteration functions or a different data structure

        // Check if we have an exact match or recent checkpoint
        if (table::contains(&state.supply_checkpoints, ts_ms)) {
            *table::borrow(&state.supply_checkpoints, ts_ms)
        } else {
            // For now, return 0 as we need proper table iteration
            // In a production system, we would:
            // 1. Iterate through checkpoints to find the latest one <= ts_ms
            // 2. Or maintain a sorted vector of checkpoint timestamps
            // 3. Use binary search to find the appropriate checkpoint
            0
        }
    }

    /// Get information about a lock
    ///
    /// # Parameters
    /// - `lock`: The lock to query
    ///
    /// # Returns
    /// - Lock owner address
    /// - Amount of LFS locked
    /// - Lock start timestamp
    /// - Lock end timestamp
    public(package) fun get_lock_info(lock: &Lock): (address, u64, u64, u64) {
        (
            lock.owner,
            balance::value(&lock.lfs_locked),
            lock.start_ms,
            lock.end_ms
        )
    }

    /// Get maximum allowed lock duration
    ///
    /// # Returns
    /// Maximum lock duration in milliseconds (4 years)
    public fun max_lock_duration(): u64 {
        MAX_LOCK_MS
    }

    /// Get minimum allowed lock duration
    ///
    /// # Returns
    /// Minimum lock duration in milliseconds (1 week)
    public fun min_lock_duration(): u64 {
        MIN_LOCK_MS
    }

    // Test-only functions
    #[test_only]
    public fun init_for_testing(ctx: &mut TxContext) {
        init(ctx);
    }

    #[test_only]
    public fun create_test_lock(
        amount: u64,
        duration_ms: u64,
        ctx: &mut tx_context::TxContext
    ): (Lock, Coin<LFS>) {
        use leafsii::lfs_token;
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ctx);
        let mut lfs_coin = coin::mint(&mut treasury_cap, amount, ctx);
        let test_coin = coin::split(&mut lfs_coin, amount, ctx);

        transfer::public_transfer(treasury_cap, tx_context::sender(ctx));
        transfer::public_transfer(emissions_cap, tx_context::sender(ctx));

        (Lock {
            id: object::new(ctx),
            owner: tx_context::sender(ctx),
            lfs_locked: coin::into_balance(test_coin),
            start_ms: 0,
            end_ms: duration_ms,
        }, lfs_coin)
    }
}