/// Staking Helper Module for Sui Validator Staking Operations
///
/// This module manages the protocol's staked SUI assets through a three-tier architecture:
///
/// Tier 1: Pending StakedSui Buffer (pending_by_epoch)
///   - Holds StakedSui indexed by activation epoch
///   - Provides instant unstaking capability for redemptions
///   - Acts as "hot buffer" with ~5% target allocation
///   - Can be instantly unstaked without waiting for epoch completion
///
/// Tier 2: Active FungibleStakedSui (active_fss)
///   - Consolidated matured stakes for maximum efficiency
///   - Earns staking rewards from validators
///   - Can be redeemed via sui_system for SUI
///   - Provides deeper liquidity beyond the instant buffer
///
/// Tier 3: Redemption Tickets (FIFO Queue)
///   - Queued redemption requests when liquidity exhausted
///   - Fulfilled by keepers via sweep_and_pay operation
///   - Maintains fairness through first-in-first-out ordering
///
/// Key Operations:
/// - add_pending_stake: Add new StakedSui to pending buffer
/// - withdraw_from_pending: Extract StakedSui for instant redemptions (FIFO)
/// - convert_and_consolidate_matured_stakes: Convert matured StakedSui → FungibleStakedSui
/// - split_active_fss: Extract FungibleStakedSui for redemptions
///
/// Principal Tracking:
/// The module tracks total_principal separately from actual staked values to distinguish
/// between deposited amounts and accumulated staking rewards. This is critical for
/// accurate token pricing in the main protocol.
module leafsii::staking_helper {
    use sui::event;
    use sui::table::{Self, Table};
    use sui_system::staking_pool::{Self, StakedSui, FungibleStakedSui};
    use sui_system::sui_system::SuiSystemState;

    // ========================================================================
    // Error Codes
    // ========================================================================

    /// No active FungibleStakedSui available for splitting/redemption
    const E_NO_ACTIVE_STAKE: u64 = 4;

    // ========================================================================
    // Core Data Structures
    // ========================================================================

    /// Pool stake structure managing all staked assets for a single validator
    ///
    /// This structure consolidates three types of staked assets:
    /// 1. active_fss: Consolidated FungibleStakedSui earning rewards
    /// 2. pending_by_epoch: StakedSui buffer indexed by activation epoch
    /// 3. total_principal: Tracked principal amount (excludes rewards)
    /// 4. fee_principal: Tracked fee amount within total_principal
    ///
    /// The pending_epochs vector maintains epoch keys for efficient iteration.
    public struct PoolStake has store {
        /// Consolidated FungibleStakedSui for maximum efficiency
        active_fss: option::Option<FungibleStakedSui>,
        /// StakedSui indexed by activation epoch for instant unstaking
        pending_by_epoch: Table<u64, StakedSui>,
        /// Ordered list of epochs with pending stakes for FIFO processing
        pending_epochs: vector<u64>,
        /// Total principal amount deposited (excludes staking rewards)
        total_principal: u64,
        /// Fee principal amount within total_principal (subset)
        fee_principal: u64,
    }

    /// Redemption ticket for queued redemptions with expiration
    ///
    /// Created when instant liquidity is insufficient for a redemption request.
    /// Users can either self-claim or delegate to a keeper for a fee.
    ///
    /// Self-redeem: delegate_enabled = false, operation_fee = 0
    /// Delegate redeem: delegate_enabled = true, operation_fee > 0 (paid to keeper)
    public struct RedemptionTicket has key, store {
        id: UID,
        /// Address to receive SUI when ticket is fulfilled
        user: address,
        /// Amount of SUI owed to user (after operation fee)
        amount: u64,
        /// Timestamp (ms) when this ticket expires
        expiration_timestamp: u64,
        /// Operation fee paid to keeper (0 for self-redeem)
        operation_fee: u64,
        /// Whether keeper can execute this redemption
        delegate_enabled: bool,
    }

    /// Unstake intent tracking (legacy - may be removed in future versions)
    ///
    /// Tracks unstaking operations for audit and monitoring purposes.
    #[allow(unused_field)]
    public struct UnstakeIntent has store, copy, drop {
        ticket_id: ID,
        amount: u64,
    }

    // ========================================================================
    // Events
    // ========================================================================

    /// Emitted when new SUI is staked with a validator
    public struct StakePlaced has copy, drop {
        amount: u64,
        activation_epoch: u64,
    }

    /// Emitted when a new stake is merged with existing stake for same epoch
    public struct StakedMerged has copy, drop {
        activation_epoch: u64,
        amount: u64,
    }

    /// Emitted when StakedSui is converted to FungibleStakedSui
    public struct StakedConvertedToFungible has copy, drop {
        amount: u64,
    }

    /// Emitted when FungibleStakedSui portions are joined together
    #[allow(unused_field)]
    public struct FungibleStakedJoined has copy, drop {
        amount: u64,
    }

    /// Emitted when unstaking is requested (legacy event)
    #[allow(unused_field)]
    public struct UnstakeRequested has copy, drop {
        amount: u64,
        ticket_id: ID,
    }

    /// Create a new empty PoolStake structure
    ///
    /// Initializes all stake tracking with zero values.
    /// Used when setting up a new protocol instance.
    public(package) fun new_pool_stake(ctx: &mut TxContext): PoolStake {
        PoolStake {
            active_fss: option::none(),
            pending_by_epoch: table::new(ctx),
            pending_epochs: vector::empty(),
            total_principal: 0,
            fee_principal: 0,
        }
    }

    /// Create a new redemption ticket with expiration
    ///
    /// # Parameters
    /// - `user`: Address to receive payout when ticket is claimed
    /// - `amount`: SUI amount owed to user (after operation fee)
    /// - `expiration_timestamp`: Unix timestamp (ms) when ticket expires
    /// - `operation_fee`: Fee paid to keeper (0 for self-redeem)
    /// - `delegate_enabled`: Whether keeper can execute
    ///
    /// # Returns
    /// New RedemptionTicket object
    public(package) fun new_redemption_ticket(
        user: address,
        amount: u64,
        expiration_timestamp: u64,
        operation_fee: u64,
        delegate_enabled: bool,
        ctx: &mut TxContext
    ): RedemptionTicket {
        let id = object::new(ctx);
        RedemptionTicket { id, user, amount, expiration_timestamp, operation_fee, delegate_enabled }
    }

    /// Get total principal amount currently staked
    ///
    /// Returns the principal amount (excludes staking rewards).
    /// This is the basis for the protocol's invariant equation.
    public fun get_total_staked_amount(stake: &PoolStake): u64 {
        stake.total_principal
    }

    /// Update total principal tracking
    ///
    /// Increments or decrements total_principal when stakes are added/removed.
    ///
    /// # Parameters
    /// - `delta`: Amount to add or subtract
    /// - `is_increase`: true to add, false to subtract
    public(package) fun update_total_principal(stake: &mut PoolStake, delta: u64, is_increase: bool) {
        if (is_increase) {
            stake.total_principal = stake.total_principal + delta;
        } else {
            stake.total_principal = stake.total_principal - delta;
        }
    }

    /// Get current active FungibleStakedSui value
    ///
    /// Returns the total value of active FSS (principal + rewards).
    /// Returns 0 if no active FSS exists.
    public fun get_active_fss_amount(stake: &PoolStake): u64 {
        if (option::is_some(&stake.active_fss)) {
            staking_pool::fungible_staked_sui_value(option::borrow(&stake.active_fss))
        } else {
            0
        }
    }

    /// Get pending stake amount for a specific epoch
    ///
    /// # Parameters
    /// - `epoch`: Activation epoch to query
    ///
    /// # Returns
    /// StakedSui amount for that epoch, or 0 if none exists
    public fun get_pending_amount_for_epoch(stake: &PoolStake, epoch: u64): u64 {
        if (table::contains(&stake.pending_by_epoch, epoch)) {
            staking_pool::staked_sui_amount(table::borrow(&stake.pending_by_epoch, epoch))
        } else {
            0
        }
    }

    /// Destroy an empty PoolStake structure
    ///
    /// Can only be called if all stakes have been withdrawn.
    /// Used for cleanup when migrating validators or closing positions.
    ///
    /// # Aborts
    /// - If active_fss is not empty
    /// - If pending_by_epoch is not empty
    /// - If pending_epochs is not empty
    public(package) fun destroy_pool_stake_if_empty(stake: PoolStake) {
        let PoolStake { active_fss, pending_by_epoch, pending_epochs, total_principal: _, fee_principal: _ } = stake;

        assert!(option::is_none(&active_fss), 0);
        assert!(table::is_empty(&pending_by_epoch), 0);
        assert!(vector::is_empty(&pending_epochs), 0);

        option::destroy_none(active_fss);
        table::destroy_empty(pending_by_epoch);
        vector::destroy_empty(pending_epochs);
    }

    /// Remove an epoch from pending_epochs tracking vector
    ///
    /// Internal helper for consolidation operations.
    /// Maintains sorted order of pending_epochs.
    fun remove_epoch_from_pending(pending_epochs: &mut vector<u64>, epoch_to_remove: u64) {
        let (found, index) = vector::index_of(pending_epochs, &epoch_to_remove);
        if (found) {
            vector::remove(pending_epochs, index);
        };
    }

    /// Add a new pending stake to the structure
    ///
    /// Adds StakedSui to pending buffer indexed by activation epoch.
    /// If a stake for that epoch already exists, merges them.
    ///
    /// # Parameters
    /// - `staked_sui`: The StakedSui object from staking operation
    /// - `activation_epoch`: Epoch when this stake becomes active
    public(package) fun add_pending_stake(
        stake: &mut PoolStake,
        staked_sui: StakedSui,
        activation_epoch: u64,
    ) {
        add_pending_stake_with_fee_tracking(stake, staked_sui, activation_epoch, false);
    }

    /// Add a pending stake with fee tracking
    ///
    /// Same as add_pending_stake but also tracks fee principal separately.
    /// Used when staking collected fees to enable fee treasury splitting.
    ///
    /// # Parameters
    /// - `is_fee`: true if this stake comes from fees, false for user deposits
    public(package) fun add_pending_stake_with_fee_tracking(
        stake: &mut PoolStake,
        staked_sui: StakedSui,
        activation_epoch: u64,
        is_fee: bool,
    ) {
        let amount = staking_pool::staked_sui_amount(&staked_sui);

        // Update fee_principal if this is a fee stake
        if (is_fee) {
            stake.fee_principal = stake.fee_principal + amount;
        };

        if (table::contains(&stake.pending_by_epoch, activation_epoch)) {
            let mut existing = table::remove(&mut stake.pending_by_epoch, activation_epoch);
            let amount_before = staking_pool::staked_sui_amount(&existing);
            existing.join(staked_sui);
            table::add(&mut stake.pending_by_epoch, activation_epoch, existing);

            event::emit(StakedMerged {
                activation_epoch,
                amount: amount_before + amount,
            });
        } else {
            table::add(&mut stake.pending_by_epoch, activation_epoch, staked_sui);

            // Track this epoch in pending_epochs vector for iteration
            vector::push_back(&mut stake.pending_epochs, activation_epoch);

            event::emit(StakePlaced { amount, activation_epoch });
        }
    }

    /// Get redemption ticket ID
    public fun get_ticket_id(ticket: &RedemptionTicket): ID {
        object::id(ticket)
    }

    /// Get SUI amount owed for this redemption ticket
    public fun get_ticket_amount(ticket: &RedemptionTicket): u64 {
        ticket.amount
    }

    /// Get user address for this redemption ticket
    public fun get_ticket_user(ticket: &RedemptionTicket): address {
        ticket.user
    }

    /// Get expiration timestamp for this ticket
    public fun get_ticket_expiration(ticket: &RedemptionTicket): u64 {
        ticket.expiration_timestamp
    }

    /// Get operation fee for this ticket
    public fun get_ticket_operation_fee(ticket: &RedemptionTicket): u64 {
        ticket.operation_fee
    }

    /// Check if keeper delegation is enabled
    public fun is_delegate_enabled(ticket: &RedemptionTicket): bool {
        ticket.delegate_enabled
    }

    /// Check if ticket is expired
    public fun is_ticket_expired(ticket: &RedemptionTicket, current_timestamp: u64): bool {
        current_timestamp > ticket.expiration_timestamp
    }

    /// Destroy redemption ticket and extract its data
    ///
    /// Called when ticket is claimed or expired.
    ///
    /// # Returns
    /// Tuple of (user_address, sui_amount, expiration_timestamp)
    public(package) fun destroy_ticket(ticket: RedemptionTicket): (address, u64, u64) {
        let RedemptionTicket { id, user, amount, expiration_timestamp, operation_fee: _, delegate_enabled: _ } = ticket;
        object::delete(id);
        (user, amount, expiration_timestamp)
    }

    /// Calculate how much SUI should be staked to reach target buffer
    ///
    /// Used to determine stake amount when buffer exceeds target.
    ///
    /// # Parameters
    /// - `current_buffer`: Current liquid SUI buffer amount
    /// - `incoming_amount`: New SUI being added to buffer
    /// - `target_buffer_bps`: Target buffer percentage in basis points
    /// - `total_reserve`: Total protocol reserves
    ///
    /// # Returns
    /// Amount to stake, or 0 if buffer below target
    public fun calculate_stake_amount(
        current_buffer: u64,
        incoming_amount: u64,
        target_buffer_bps: u64,
        total_reserve: u64
    ): u64 {
        let target_buffer = (total_reserve * target_buffer_bps) / 10000;
        let new_buffer_total = current_buffer + incoming_amount;

        if (new_buffer_total > target_buffer) {
            new_buffer_total - target_buffer
        } else {
            0
        }
    }

    /// Get total protocol reserve (buffer + staked principal)
    ///
    /// # Parameters
    /// - `buffer_amount`: Current liquid buffer
    /// - `stake`: PoolStake tracking all staked amounts
    ///
    /// # Returns
    /// Total reserves (excludes staking rewards)
    public fun get_total_reserve(
        buffer_amount: u64,
        stake: &PoolStake
    ): u64 {
        buffer_amount + stake.total_principal
    }

    /// Get total principal staked (excludes rewards)
    public fun get_total_principal(stake: &PoolStake): u64 {
        stake.total_principal
    }

    /// Get fee principal amount within total principal
    ///
    /// This is a subset of total_principal representing fees.
    /// Used to split fee portion when converting to FSS.
    public fun get_fee_principal(stake: &PoolStake): u64 {
        stake.fee_principal
    }

    /// Get total value of pending stakes across all epochs
    ///
    /// Sums up all StakedSui amounts waiting to become active.
    /// This represents the "hot buffer" available for instant redemptions.
    ///
    /// # Returns
    /// Total StakedSui value in pending buffer
    public fun get_pending_stakes_value(stake: &PoolStake): u64 {
        let mut total = 0;
        let mut i = 0;
        let len = vector::length(&stake.pending_epochs);

        while (i < len) {
            let epoch = *vector::borrow(&stake.pending_epochs, i);
            let staked = table::borrow(&stake.pending_by_epoch, epoch);
            total = total + staking_pool::staked_sui_amount(staked);
            i = i + 1;
        };

        total
    }

    /// Withdraw StakedSui from pending stakes in FIFO order
    ///
    /// Extracts StakedSui for instant redemptions. Processes epochs
    /// in order, taking entire stakes or splitting as needed.
    ///
    /// This provides instant liquidity without waiting for epoch maturity.
    ///
    /// # Parameters
    /// - `amount`: SUI amount to withdraw
    ///
    /// # Returns
    /// Option<StakedSui> with requested amount, or None if insufficient
    public(package) fun withdraw_from_pending(
        stake: &mut PoolStake,
        amount: u64,
        ctx: &mut TxContext
    ): option::Option<StakedSui> {
        if (vector::is_empty(&stake.pending_epochs) || amount == 0) {
            return option::none()
        };

        let mut remaining = amount;
        let mut result: option::Option<StakedSui> = option::none();

        // Iterate through pending epochs in order (FIFO)
        while (!vector::is_empty(&stake.pending_epochs) && remaining > 0) {
            let first_epoch = *vector::borrow(&stake.pending_epochs, 0);
            let staked = table::borrow(&stake.pending_by_epoch, first_epoch);
            let available = staking_pool::staked_sui_amount(staked);

            if (available <= remaining) {
                // Take entire stake from this epoch
                let taken = table::remove(&mut stake.pending_by_epoch, first_epoch);
                remove_epoch_from_pending(&mut stake.pending_epochs, first_epoch);

                if (option::is_none(&result)) {
                    option::fill(&mut result, taken);
                } else {
                    let mut existing = option::extract(&mut result);
                    existing.join(taken);
                    option::fill(&mut result, existing);
                };

                remaining = remaining - available;
            } else {
                // Split from this epoch
                let staked_mut = table::borrow_mut(&mut stake.pending_by_epoch, first_epoch);
                let split_stake = staked_mut.split(remaining, ctx);

                if (option::is_none(&result)) {
                    option::fill(&mut result, split_stake);
                } else {
                    let mut existing = option::extract(&mut result);
                    existing.join(split_stake);
                    option::fill(&mut result, existing);
                };

                remaining = 0;
            };
        };

        result
    }

    /// Split FungibleStakedSui from active pool for redemptions
    ///
    /// Extracts FSS when buffer and pending stakes are insufficient.
    /// This is the deepest tier of liquidity.
    ///
    /// # Parameters
    /// - `amount`: FSS amount to split (in SUI equivalent)
    ///
    /// # Returns
    /// FungibleStakedSui that can be redeemed via sui_system
    ///
    /// # Aborts
    /// - `E_NO_ACTIVE_STAKE`: If no active FSS exists
    public(package) fun split_active_fss(stake: &mut PoolStake, amount: u64, ctx: &mut TxContext): FungibleStakedSui {
        assert!(option::is_some(&stake.active_fss), E_NO_ACTIVE_STAKE);
        let active = option::borrow_mut(&mut stake.active_fss);
        staking_pool::split_fungible_staked_sui(active, amount, ctx)
    }

    // Convert matured StakedSui to FungibleStakedSui and consolidate (Procedure B)
    // Returns number of items processed
    public(package) fun convert_and_consolidate_matured_stakes(
        wrapper: &mut SuiSystemState,
        stake: &mut PoolStake,
        current_epoch: u64,
        max_items: u64,
        ctx: &mut TxContext
    ): u64 {
        let (processed, mut _fee_fss) = convert_and_consolidate_matured_stakes_with_fees(
            wrapper,
            stake,
            current_epoch,
            max_items,
            ctx
        );

        // For backward compatibility, join fee portion back into active_fss
        if (option::is_some(&_fee_fss)) {
            let fee_portion = option::extract(&mut _fee_fss);
            if (option::is_some(&stake.active_fss)) {
                let active = option::borrow_mut(&mut stake.active_fss);
                staking_pool::join_fungible_staked_sui(active, fee_portion);
            } else {
                option::fill(&mut stake.active_fss, fee_portion);
            };
        };
        option::destroy_none(_fee_fss);

        processed
    }

    /// Convert matured stakes to FSS and split out fee portion
    ///
    /// Advanced version of conversion that tracks fees separately.
    /// This enables proper fee treasury accounting:
    ///
    /// 1. Converts matured StakedSui → FungibleStakedSui
    /// 2. Calculates fee proportion based on fee_principal / total_principal
    /// 3. Splits fee portion (including its share of rewards) from active FSS
    /// 4. Returns fee portion separately for fee_treasury_balance
    ///
    /// This solves the fee conversion problem by converting SUI fees → FSS
    /// during the normal stake maturation cycle.
    ///
    /// # Parameters
    /// - `current_epoch`: Current Sui epoch
    /// - `max_items`: Maximum stakes to process (gas limiting)
    ///
    /// # Returns
    /// Tuple of (items_processed, Option<fee_fss>)
    public(package) fun convert_and_consolidate_matured_stakes_with_fees(
        wrapper: &mut SuiSystemState,
        stake: &mut PoolStake,
        current_epoch: u64,
        max_items: u64,
        ctx: &mut TxContext
    ): (u64, option::Option<FungibleStakedSui>) {
        let mut processed = 0;
        let mut epochs_to_remove = vector::empty<u64>();
        let mut total_converted_principal = 0;

        // Iterate through pending_epochs to find matured stakes
        let mut i = 0;
        let len = vector::length(&stake.pending_epochs);

        while (i < len && processed < max_items) {
            let epoch = *vector::borrow(&stake.pending_epochs, i);

            // Check if this stake is matured (activation_epoch <= current_epoch)
            if (can_convert_stake(epoch, current_epoch)) {
                // Remove the StakedSui from table
                let staked_sui = table::remove(&mut stake.pending_by_epoch, epoch);
                let amount = staking_pool::staked_sui_amount(&staked_sui);

                // Convert to FungibleStakedSui using sui_system wrapper
                let fss = sui_system::sui_system::convert_to_fungible_staked_sui(wrapper, staked_sui, ctx);

                // Join into active_fss (create if doesn't exist)
                if (option::is_some(&stake.active_fss)) {
                    let active = option::borrow_mut(&mut stake.active_fss);
                    staking_pool::join_fungible_staked_sui(active, fss);
                } else {
                    option::fill(&mut stake.active_fss, fss);
                };

                // Track this epoch for removal from pending_epochs
                vector::push_back(&mut epochs_to_remove, epoch);

                // Track converted principal for fee calculation
                total_converted_principal = total_converted_principal + amount;

                // Emit event
                event::emit(StakedConvertedToFungible { amount });

                processed = processed + 1;
            };

            i = i + 1;
        };

        // Remove processed epochs from pending_epochs vector
        let mut j = 0;
        let remove_len = vector::length(&epochs_to_remove);
        while (j < remove_len) {
            let epoch_to_remove = *vector::borrow(&epochs_to_remove, j);
            remove_epoch_from_pending(&mut stake.pending_epochs, epoch_to_remove);
            j = j + 1;
        };

        // Calculate and split fee portion from active_fss
        let mut fee_fss_option = option::none<FungibleStakedSui>();
        if (total_converted_principal > 0 && stake.fee_principal > 0 && option::is_some(&stake.active_fss)) {
            // Calculate fee proportion: fee_principal / total_principal
            let fee_ratio = (stake.fee_principal * SCALE_FACTOR) / stake.total_principal;

            // Get total active FSS value (includes rewards)
            let total_fss_value = staking_pool::fungible_staked_sui_value(option::borrow(&stake.active_fss));

            // Calculate fee portion including its share of rewards
            let fee_fss_amount = (total_fss_value * fee_ratio) / SCALE_FACTOR;

            if (fee_fss_amount > 0) {
                let active = option::borrow_mut(&mut stake.active_fss);
                let fee_fss = staking_pool::split_fungible_staked_sui(active, fee_fss_amount, ctx);
                option::fill(&mut fee_fss_option, fee_fss);

                // Reduce fee_principal since we've split it out
                stake.fee_principal = 0;
            };
        };

        (processed, fee_fss_option)
    }

    /// Scale factor for fee proportion calculations (1e9)
    /// Must match SCALE_FACTOR in leafsii.move
    const SCALE_FACTOR: u64 = 1_000_000_000;

    /// Check if a stake is matured and ready for conversion
    ///
    /// Stakes become active (convertible to FSS) when current epoch
    /// reaches or exceeds their activation epoch.
    ///
    /// # Returns
    /// true if stake can be converted, false otherwise
    public fun can_convert_stake(activation_epoch: u64, current_epoch: u64): bool {
        activation_epoch <= current_epoch
    }

    /// Get count of distinct pending stake epochs
    ///
    /// # Returns
    /// Number of different epochs with pending stakes
    public fun get_pending_stakes_count(stake: &PoolStake): u64 {
        table::length(&stake.pending_by_epoch)
    }

    /// Check if active FungibleStakedSui exists
    ///
    /// # Returns
    /// true if active_fss is populated, false if None
    public fun has_active_fss(stake: &PoolStake): bool {
        option::is_some(&stake.active_fss)
    }

    // Test-only helpers for testing without actual StakingPool
    #[test_only]
    public fun test_maintenance_without_pool() {
        // For testing purposes, we can validate that our functions compile
        // and handle the basic logic without requiring an actual StakingPool
        // This demonstrates that the maintenance functions are ready for
        // integration once proper StakingPool access is available
    }
}