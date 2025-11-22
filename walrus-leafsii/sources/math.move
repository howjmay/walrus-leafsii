/// Math Utilities for Safe Arithmetic Operations
///
/// This module provides overflow-safe mathematical operations critical for DeFi protocols.
/// All functions use u256 intermediate values to prevent overflow, then safely downcast
/// results with explicit overflow checks.
///
/// Core Capabilities:
/// 1. Safe Multiplication and Division:
///    - Prevents overflow by using u256 for intermediate calculations
///    - Explicit overflow checks when downcasting to u64/u128
///    - Rounds down by default, with explicit ceiling variants
///
/// 2. Basis Points (BPS) Operations:
///    - 1 BPS = 0.01% (10,000 BPS = 100%)
///    - Used for fees, percentages, and fractional calculations
///    - Example: 500 BPS = 5%
///
/// 3. Fixed-Point Arithmetic:
///    - Price scaling: 1e6 for oracle prices
///    - Reward scaling: 1e18 for cumulative reward integrals
///    - Enables sub-unit precision without floating point
///
/// 4. Proportional Division:
///    - Calculate (a * b) / c without intermediate overflow
///    - Essential for price conversions and reward distribution
///    - Maintains precision across scaling boundaries
///
/// Key Functions:
/// - mul_div(a, b, c): Returns (a * b) / c, rounded down
/// - mul_div_ceil(a, b, c): Returns ceil((a * b) / c), rounded up
/// - apply_bps(amount, bps): Returns amount * bps / 10000
/// - safe_sub(a, b): Returns max(a - b, 0), never underflows
///
/// Safety Guarantees:
/// - All operations check for division by zero
/// - Explicit overflow assertions on downcast operations
/// - Intermediate calculations use u256 to prevent overflow
/// - Clear error codes for debugging
///
/// Usage Examples:
/// ```move
/// // Calculate 5% fee on 1000 tokens
/// let fee = apply_bps(1000, 500); // 500 BPS = 5% → returns 50
///
/// // Convert USD to native tokens
/// let tokens = mul_div(usd_amount, 1e6, price_per_token);
///
/// // Proportional reward distribution
/// let user_reward = mul_div(total_rewards, user_share, total_shares);
/// ```
///
/// Scaling Constants:
/// - BPS_SCALE: 10,000 (100% in basis points)
/// - PRICE_SCALE_E6: 1,000,000 (6 decimals for prices)
/// - REWARD_SCALE_E18: 1e18 (18 decimals for reward integrals)
module math::math {
    // Error codes
    const E_DIVISION_BY_ZERO: u64 = 1;
    const E_OVERFLOW: u64 = 2;

    // Constants
    const BPS_SCALE: u64 = 10_000;           // 1 BPS = 0.01%
    const PRICE_SCALE_E6: u64 = 1_000_000;  // Price oracle scaling
    const REWARD_SCALE_E18: u128 = 1_000_000_000_000_000_000; // Rewards per share scaling
    const MAX_U64: u64 = 18_446_744_073_709_551_615;
    const MAX_U128: u128 = 340_282_366_920_938_463_463_374_607_431_768_211_455;

    /// Multiply two u64 values and return u256 to prevent overflow
    public fun mul_u64_to_u256(a: u64, b: u64): u256 {
        (a as u256) * (b as u256)
    }

    /// Multiply u64 by u256 and return u256
    public fun mul_u64_u256(a: u64, b: u256): u256 {
        (a as u256) * b
    }

    /// Safe division with u256 operands, returning u64
    /// Rounds down (truncates)
    public fun div_u256_to_u64(a: u256, b: u256): u64 {
        assert!(b > 0, E_DIVISION_BY_ZERO);
        let result = a / b;
        assert!(result <= (MAX_U64 as u256), E_OVERFLOW);
        (result as u64)
    }

    /// Safe division with u256 operands, returning u128
    public fun div_u256_to_u128(a: u256, b: u256): u128 {
        assert!(b > 0, E_DIVISION_BY_ZERO);
        let result = a / b;
        assert!(result <= (MAX_U128 as u256), E_OVERFLOW);
        (result as u128)
    }

    /// Multiply and divide in one operation to prevent intermediate overflow
    /// Returns (a * b) / c, rounded down
    public fun mul_div(a: u64, b: u64, c: u64): u64 {
        assert!(c > 0, E_DIVISION_BY_ZERO);
        let numerator = mul_u64_to_u256(a, b);
        div_u256_to_u64(numerator, (c as u256))
    }

    /// Multiply and divide in one operation, rounded up
    /// Returns ceil((a * b) / c)
    public fun mul_div_ceil(a: u64, b: u64, c: u64): u64 {
        assert!(c > 0, E_DIVISION_BY_ZERO);
        let numerator = mul_u64_to_u256(a, b);
        let mut result = numerator / (c as u256);
        
        // Add 1 if there's a remainder
        if (numerator % (c as u256) > 0) {
            result = result + 1;
        };
        
        assert!(result <= (MAX_U64 as u256), E_OVERFLOW);
        (result as u64)
    }

    /// Multiply and divide with u128 operands for higher precision
    /// Returns (a * b) / c where all operands are u128
    public fun mul_div_u128(a: u128, b: u128, c: u128): u128 {
        assert!(c > 0, E_DIVISION_BY_ZERO);
        let numerator = (a as u256) * (b as u256);
        div_u256_to_u128(numerator, c as u256)
    }

    /// Multiply and divide u128 values, rounded up
    public fun mul_div_ceil_u128(a: u128, b: u128, c: u128): u128 {
        assert!(c > 0, E_DIVISION_BY_ZERO);
        let numerator = (a as u256) * (b as u256);
        let mut result = numerator / (c as u256);
        
        // Add 1 if there's a remainder
        if (numerator % (c as u256) > 0) {
            result = result + 1;
        };
        
        assert!(result <= (MAX_U128 as u256), E_OVERFLOW);
        (result as u128)
    }

    /// Convert basis points to actual value
    /// Example: apply_bps(1000, 500) = 50 (5% of 1000)
    public fun apply_bps(amount: u64, bps: u64): u64 {
        mul_div(amount, bps, BPS_SCALE)
    }

    /// Calculate basis points from two values
    /// Returns (numerator * BPS_SCALE) / denominator
    public fun to_bps(numerator: u64, denominator: u64): u64 {
        assert!(denominator > 0, E_DIVISION_BY_ZERO);
        mul_div(numerator, BPS_SCALE, denominator)
    }

    /// Convert USD amount (6 decimal places) to native units
    public fun usd_e6_to_native(usd_amount_e6: u64, price_per_unit_e6: u64): u64 {
        assert!(price_per_unit_e6 > 0, E_DIVISION_BY_ZERO);
        mul_div(usd_amount_e6, PRICE_SCALE_E6, price_per_unit_e6)
    }

    /// Convert native units to USD (6 decimal places)
    public fun native_to_usd_e6(native_amount: u64, price_per_unit_e6: u64): u64 {
        mul_div(native_amount, price_per_unit_e6, PRICE_SCALE_E6)
    }

    /// Safe subtraction that returns 0 instead of underflowing
    public fun safe_sub(a: u64, b: u64): u64 {
        if (a >= b) a - b else 0
    }

    /// Safe subtraction for u256 that returns 0 instead of underflowing
    public fun safe_sub_u256(a: u256, b: u256): u256 {
        if (a >= b) a - b else 0
    }

    /// Check if adding two u64 values would overflow
    public fun would_overflow_add(a: u64, b: u64): bool {
        a > MAX_U64 - b
    }

    /// Check if multiplying two u64 values would overflow u64
    public fun would_overflow_mul(a: u64, b: u64): bool {
        if (a == 0 || b == 0) return false;
        a > MAX_U64 / b
    }

    /// Constants getters for testing and external use
    public fun bps_scale(): u64 { BPS_SCALE }
    public fun price_scale_e6(): u64 { PRICE_SCALE_E6 }
    public fun reward_scale_e18(): u128 { REWARD_SCALE_E18 }
    public fun reward_scale_e18_u64(): u64 { (REWARD_SCALE_E18 as u64) }

    public fun mul_to_u128(x: u64, y: u64): u128 {
        return (x as u128) * (y as u128)
    }

    public fun div_back_u64(x: u128, y: u64): u64 {
        return (x / (y as u128)) as u64
    }

    public fun mul_div_back_u64(a: u128, b: u128, c: u128): u64 {
        assert!(c > 0, E_DIVISION_BY_ZERO);
        let numerator = (a as u256) * (b as u256);
        div_u256_to_u64(numerator, (c as u256))
    }

    /// Calculate scaled division for reward distribution
    /// Returns (amount * scale_factor) / divisor as u128
    public fun scaled_div_u128(amount: u64, divisor: u64, scale_factor: u128): u128 {
        assert!(divisor > 0, E_DIVISION_BY_ZERO);
        ((amount as u128) * scale_factor) / (divisor as u128)
    }

    /// Calculate proportional division and return u64
    /// Returns (a * b) / c as u64, where a, b, c are u64
    public fun proportional_div_u64(a: u64, b: u64, c: u64): u64 {
        assert!(c > 0, E_DIVISION_BY_ZERO);
        (((a as u128) * (b as u128)) / (c as u128)) as u64
    }

    #[test]
    fun test_mul_div() {
        assert!(mul_div(100, 50, 10) == 500, 0);
        assert!(mul_div(1000, 500, 10000) == 50, 0);
        assert!(mul_div(0, 100, 1) == 0, 0);
    }

    #[test]
    fun test_mul_div_ceil() {
        assert!(mul_div_ceil(100, 50, 10) == 500, 0);
        // Use values that produce a fractional result to test ceiling
        assert!(mul_div_ceil(101, 3, 10) == 31, 0); // 303/10 = 30.3 -> 31
        assert!(mul_div_ceil(100, 30, 10) == 300, 0); // Exact division
    }

    #[test]
    fun test_bps_operations() {
        assert!(apply_bps(10000, 500) == 500, 0); // 5%
        assert!(to_bps(500, 10000) == 500, 0); // 5%
        assert!(apply_bps(0, 500) == 0, 0);
    }

    #[test]
    fun test_safe_sub() {
        assert!(safe_sub(100, 50) == 50, 0);
        assert!(safe_sub(50, 100) == 0, 0);
        assert!(safe_sub(0, 100) == 0, 0);
    }

    #[test]
    fun test_overflow_checks() {
        assert!(!would_overflow_add(100, 200), 0);
        assert!(!would_overflow_mul(100, 200), 0);
        assert!(!would_overflow_mul(0, MAX_U64), 0);
    }
}
