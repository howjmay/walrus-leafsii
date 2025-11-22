/// Oracle Interface and Mock Implementation for Price Feeds
///
/// This module provides price oracle functionality for the Leafsii protocol, specifically
/// for fetching SUI/USD price data. Currently implements a mock oracle for testing and
/// development, with the interface designed for easy replacement with production oracles.
///
/// Mock Oracle Features:
/// - Manual price setting for testing different scenarios
/// - Staleness checks: Prices older than 1 hour are rejected
/// - Price bounds: Enforces min ($0.001) and max ($1M) price limits
/// - Pause mechanism: Admin can pause oracle to simulate failures
/// - Timestamp tracking: Records when price was last updated
///
/// Price Format:
/// - Scale: 1e6 (6 decimal places, compatible with most USD oracles)
/// - Example: $1.50 = 1,500,000 (1.5 * 1e6)
/// - Range: $0.001 to $1,000,000
///
/// Safety Features:
/// 1. Staleness Protection:
///    Prices older than 1 hour are rejected to prevent stale data usage
///
/// 2. Price Bounds:
///    Min/max limits prevent unrealistic prices from breaking protocol invariants
///
/// 3. Validity Flag:
///    Oracle can be marked invalid (paused) to halt operations during issues
///
/// 4. Timestamp Verification:
///    All price fetches check age against current clock
///
/// Production Oracle Integration:
/// This module defines the interface for production oracles:
/// - Pyth Network: High-frequency, decentralized price feeds
/// - Switchboard: Community-curated oracle network
/// - Supra: Fast finality oracle aggregator
/// - Custom aggregators: Multi-source price averaging
///
/// Key Functions:
/// - get_price_e9(): Fetch current price with staleness check (primary interface)
/// - get_raw_price_e6(): Fetch price without checks (testing/inspection only)
/// - is_price_fresh(): Check if price is recent and valid
/// - update_mock_price(): Admin function to set new price (testing only)
///
/// Integration:
/// - Protocol calls get_price_e9() for all pricing operations
/// - Price updates trigger invariant recalculation and token repricing
/// - Staleness causes operations to abort, protecting users from bad prices
module oracle::oracle {
    use sui::clock::{Clock, timestamp_ms};
    use sui::object::new;

    // Error codes
    const E_STALE_PRICE: u64 = 1;
    const E_INVALID_PRICE: u64 = 2;
    const E_ORACLE_PAUSED: u64 = 3;

    // Constants
    const STALENESS_THRESHOLD_MS: u64 = 3600000; // 1 hour
    const MIN_PRICE_E6: u64 = 1_000; // $0.001 minimum price
    const MAX_PRICE_E6: u64 = 1_000_000_000_000; // $1M maximum price

    /// Generic price oracle trait
    /// R represents the reserve asset type
    public struct PriceData has store, drop {
        price_e6: u64,        // Price in USD with 6 decimal places
        timestamp_ms: u64,    // When price was last updated
        is_valid: bool,       // Whether price feed is working
    }

    /// Mock oracle for testing - allows manual price setting
    public struct MockOracle<phantom R> has key, store {
        id: UID,
        price_data: PriceData,
        is_paused: bool,
    }

    /// Create a new mock oracle with initial price
    public fun create_mock_oracle<R>(
        initial_price_e6: u64,
        clock: &Clock,
        ctx: &mut TxContext
    ): MockOracle<R> {
        assert!(initial_price_e6 >= MIN_PRICE_E6, E_INVALID_PRICE);
        assert!(initial_price_e6 <= MAX_PRICE_E6, E_INVALID_PRICE);

        MockOracle {
            id: new(ctx),
            price_data: PriceData {
                price_e6: initial_price_e6,
                timestamp_ms: timestamp_ms(clock),
                is_valid: true,
            },
            is_paused: false,
        }
    }

    /// Update mock oracle price (for testing)
    public fun update_mock_price<R>(
        oracle: &mut MockOracle<R>,
        new_price_e6: u64,
        clock: &Clock,
    ) {
        assert!(!oracle.is_paused, E_ORACLE_PAUSED);
        assert!(new_price_e6 >= MIN_PRICE_E6, E_INVALID_PRICE);
        assert!(new_price_e6 <= MAX_PRICE_E6, E_INVALID_PRICE);

        oracle.price_data.price_e6 = new_price_e6;
        oracle.price_data.timestamp_ms = timestamp_ms(clock);
        oracle.price_data.is_valid = true;
    }

    /// Pause/unpause mock oracle
    public fun set_mock_oracle_paused<R>(oracle: &mut MockOracle<R>, paused: bool) {
        oracle.is_paused = paused;
        oracle.price_data.is_valid = !paused;
    }

    /// Get price from mock oracle with staleness check
    public fun get_price_e9<R>(oracle: &MockOracle<R>, clock: &Clock): u64 {
        assert!(!oracle.is_paused, E_ORACLE_PAUSED);
        assert!(oracle.price_data.is_valid, E_INVALID_PRICE);

        let current_time = timestamp_ms(clock);
        let age = current_time - oracle.price_data.timestamp_ms;
        assert!(age <= STALENESS_THRESHOLD_MS, E_STALE_PRICE);

        oracle.price_data.price_e6
    }

    /// Get price without staleness check (for testing edge cases)
    public fun get_raw_price_e6<R>(oracle: &MockOracle<R>): u64 {
        oracle.price_data.price_e6
    }

    /// Check if oracle price is fresh
    public fun is_price_fresh<R>(oracle: &MockOracle<R>, clock: &Clock): bool {
        if (!oracle.price_data.is_valid || oracle.is_paused) return false;
        
        let current_time = timestamp_ms(clock);
        let age = current_time - oracle.price_data.timestamp_ms;
        age <= STALENESS_THRESHOLD_MS
    }

    /// Get price data for inspection
    public fun get_price_data<R>(oracle: &MockOracle<R>): (u64, u64, bool) {
        (
            oracle.price_data.price_e6,
            oracle.price_data.timestamp_ms,
            oracle.price_data.is_valid && !oracle.is_paused
        )
    }

    /// Constants for external use
    public fun staleness_threshold_ms(): u64 { STALENESS_THRESHOLD_MS }
    public fun min_price_e6(): u64 { MIN_PRICE_E6 }
    public fun max_price_e6(): u64 { MAX_PRICE_E6 }

    #[test_only]
    use sui::test_scenario::{Self as ts};
    #[test_only]
    use sui::object::delete;
    
    #[test]
    fun test_mock_oracle_creation() {
        let mut scenario = ts::begin(@0x1);
        let ctx = ts::ctx(&mut scenario);
        
        let mock_clock = sui::clock::create_for_testing(ctx);
        let oracle = create_mock_oracle<sui::sui::SUI>(1_500_000, &mock_clock, ctx); // $1.50
        
        let (price, _, is_valid) = get_price_data(&oracle);
        assert!(price == 1_500_000, 0);
        assert!(is_valid == true, 0);
        
        sui::clock::destroy_for_testing(mock_clock);
        let MockOracle { id, price_data: _, is_paused: _ } = oracle;
        delete(id);
        ts::end(scenario);
    }

    #[test]
    fun test_price_update() {
        let mut scenario = ts::begin(@0x1);
        let ctx = ts::ctx(&mut scenario);
        
        let mock_clock = sui::clock::create_for_testing(ctx);
        let mut oracle = create_mock_oracle<sui::sui::SUI>(1_500_000, &mock_clock, ctx);
        
        update_mock_price(&mut oracle, 2_000_000, &mock_clock); // $2.00
        
        let (price, _, is_valid) = get_price_data(&oracle);
        assert!(price == 2_000_000, 0);
        assert!(is_valid == true, 0);
        
        sui::clock::destroy_for_testing(mock_clock);
        let MockOracle { id, price_data: _, is_paused: _ } = oracle;
        delete(id);
        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = E_ORACLE_PAUSED)]
    fun test_paused_oracle() {
        let mut scenario = ts::begin(@0x1);
        let ctx = ts::ctx(&mut scenario);
        
        let mock_clock = sui::clock::create_for_testing(ctx);
        let mut oracle = create_mock_oracle<sui::sui::SUI>(1_500_000, &mock_clock, ctx);
        
        set_mock_oracle_paused(&mut oracle, true);
        get_price_e9(&oracle, &mock_clock); // Should fail
        
        sui::clock::destroy_for_testing(mock_clock);
        let MockOracle { id, price_data: _, is_paused: _ } = oracle;
        delete(id);
        ts::end(scenario);
    }
}