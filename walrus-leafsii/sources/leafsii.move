/// Leafsii Protocol Core Implementation
///
/// A dual-token stablecoin protocol using Sui validator staking as collateral.
/// Implements a two-token system with automatic collateral rebalancing:
///
/// ## Token System
/// - **fToken (Stable)**: Fixed $1.00 value (Pf = 1e9), conservative yield-bearing token
/// - **xToken (Leverage)**: Variable price (Px), captures price volatility and staking rewards
///
/// ## Collateral Model
/// - Backed by staked SUI earning validator rewards
/// - Three-tier liquidity: Buffer (5%) → Active FSS → FIFO Redemption Queue
/// - Staking rewards accrue separately and don't affect CR calculations
///
/// ## Operational Modes (by Collateral Ratio)
/// - **Normal** (CR ≥ 130.6%): Standard operations, default 0.5% fees
/// - **L1 Stability** (120.6-130.6%): fToken mint disabled, xToken rebalancing incentivized
/// - **L2 User Rebalance** (114.4-120.6%): Enhanced bonuses for user-driven rebalancing
/// - **L3 Protocol Rebalance** (< 114.4%): Automated stability pool liquidations
///
/// ## Key Invariant
/// Reserve USD Value = fToken USD Value + xToken USD Value (principal-based)
/// Both CR and the invariant use principal only - staking rewards are tracked separately.
module leafsii::leafsii {
    use sui::coin::{Self, Coin, TreasuryCap};
    use sui::clock::{Clock, timestamp_ms};
    use sui::balance::{Self, Balance};
    use sui::event;

    use math::math;
    use oracle::oracle::{Self, MockOracle};
    use leafsii::stability_pool::{Self, StabilityPool, SPPosition};
    use leafsii::staking_helper::{Self, PoolStake, RedemptionTicket};
    use sui_system::staking_pool::{Self as staking_pool, FungibleStakedSui};
    use sui_system::sui_system::SuiSystemState;
    use sui::sui::SUI;


    // ========================================================================
    // Error Codes
    // ========================================================================

    /// Amount is zero or invalid for the requested operation
    const E_INVALID_AMOUNT: u64 = 1;
    /// Protocol has insufficient reserves to fulfill request (net of stability pool obligations)
    const E_INSUFFICIENT_RESERVE: u64 = 2;
    /// User actions (mint/redeem) are currently disabled by admin
    const E_MINT_BLOCKED: u64 = 3;
    /// Oracle data is too old (exceeds MAX_STALENESS_MS threshold)
    const E_ORACLE_STALE: u64 = 4;
    /// Oracle price change exceeds maximum allowed step size (20%)
    const E_ORACLE_STEP_TOO_LARGE: u64 = 5;
    /// Action not allowed in current collateral ratio level
    const E_ACTION_BLOCKED_BY_CR: u64 = 6;
    /// Admin capability doesn't match this protocol instance
    const E_INVALID_ADMIN: u64 = 7;
    /// Stability pool is not authorized for this protocol
    const E_UNAUTHORIZED_POOL: u64 = 8;

    // ========================================================================
    // Protocol Constants
    // ========================================================================

    /// Fixed price of fToken: $1.00 in nano-USD (1e9 scale)
    const PF_FIXED: u64 = 1_000_000_000;

    /// Maximum allowed oracle staleness: 1 hour in milliseconds
    const MAX_STALENESS_MS: u64 = 3600000;
    /// Maximum allowed relative price step: 20% (0.2 * 1e9)
    const MAX_REL_STEP: u64 = 200_000_000;

    /// General scaling factor for prices, CR, and calculations (1e9)
    const SCALE_FACTOR: u64 = 1_000_000_000;
    /// Small epsilon for rounding tolerance checks (1e-6 units)
    const EPS: u64 = 1000;

    // ========================================================================
    // Collateral Ratio Thresholds (1e9 scale)
    // ========================================================================

    /// CR ≥ 130.6%: Normal mode - all operations enabled, standard fees
    const CR_T_L1: u64 = 1_306_000_000;
    /// CR ≥ 120.6%: L1 Stability mode - fToken mint disabled, xToken incentivized
    const CR_T_L2: u64 = 1_206_000_000;
    /// CR ≥ 114.4%: L2 User Rebalance mode - enhanced bonuses for rebalancing
    const CR_T_L3: u64 = 1_144_000_000;
    // Note: CR < 114.4% triggers L3 Protocol Rebalance mode with automatic stability pool liquidations

    // ========================================================================
    // Operational Level Constants
    // ========================================================================

    /// Normal mode: CR ≥ 130.6% - all operations enabled, standard fees
    const LEVEL_NORMAL: u8 = 0;
    /// L1 Stability mode: 120.6% ≤ CR < 130.6% - fToken mint disabled, xToken incentivized
    const LEVEL_L1_STABILITY: u8 = 1;
    /// L2 User Rebalance mode: 114.4% ≤ CR < 120.6% - enhanced bonuses for rebalancing
    const LEVEL_L2_USER_REBALANCE: u8 = 2;
    /// L3 Protocol Rebalance mode: CR < 114.4% - automated stability pool liquidations
    const LEVEL_L3_PROTOCOL_REBALANCE: u8 = 3;

    // ========================================================================
    // Fee Configuration Constants
    // ========================================================================

    /// Basis points denominator: 100% = 10,000 basis points
    const FEE_BASIS_POINTS: u64 = 10_000;
    /// Default fee for normal mode operations: 0.5% (50 basis points)
    const DEFAULT_NORMAL_FEE_BPS: u64 = 50;
    /// Bonus rate for stability operations: 0.1% (10 basis points)
    const BONUS_RATE_BPS: u64 = 10;

    /// Default ticket expiration period: 7 days in milliseconds
    const DEFAULT_TICKET_EXPIRATION_MS: u64 = 604_800_000;

    /// Default operation fee for delegate redemptions: 1 SUI (1e9)
    const DEFAULT_DELEGATE_OPERATION_FEE: u64 = 1_000_000_000;

    // ========================================================================
    // Core Types and Capabilities
    // ========================================================================

    /// Witness type for stable token (fToken)
    /// Used in one-time-witness pattern for coin creation
    public struct FToken<phantom R> has drop {}

    /// Witness type for leverage token (xToken)
    /// Used in one-time-witness pattern for coin creation
    public struct XToken<phantom R> has drop {}

    /// Administrative capability for protocol management
    ///
    /// Bound to a specific Protocol instance via protocol_id for security.
    /// Grants authority to:
    /// - Update oracle prices
    /// - Modify fee configuration
    /// - Enable/disable user actions
    /// - Configure staking parameters
    /// - Withdraw fee treasury
    /// - Execute L3 protocol rebalancing
    public struct AdminCap has key, store {
        id: UID,
        /// ID of the Protocol instance this AdminCap controls
        protocol_id: ID,
    }

    /// Fee configuration for all protocol operations
    ///
    /// Fees vary by operational level (CR-based) and action type.
    /// All fees are in basis points (1 bps = 0.01%).
    public struct FeeConfig has store {
        /// Normal mode fToken mint fee (default 50 bps = 0.5%)
        normal_mint_f_fee_bps: u64,
        /// Normal mode xToken mint fee (default 50 bps = 0.5%)
        normal_mint_x_fee_bps: u64,
        /// Normal mode fToken redeem fee (default 50 bps = 0.5%)
        normal_redeem_f_fee_bps: u64,
        /// Normal mode xToken redeem fee (default 50 bps = 0.5%)
        normal_redeem_x_fee_bps: u64,
        /// L1 mode xToken redeem fee - higher to discourage redemptions (default 100 bps = 1.0%)
        l1_redeem_x_fee_bps: u64,
        /// Bonus rate for stability-enhancing actions (default 10 bps = 0.1%)
        stability_bonus_rate_bps: u64,
        /// Address receiving protocol fees (set by admin)
        fee_recipient: address,
    }

    /// Configuration for SUI staking operations
    ///
    /// Manages the protocol's staking strategy with validator pools.
    public struct StakingConfig has store {
        /// Target liquid buffer as basis points of total reserve (default 500 = 5%)
        target_buffer_bps: u64,
        /// ID of the validator gauge determining pool selection (updated by admin)
        validator_gauge_id: ID,
    }

    /// Core protocol state managing dual-token system and staked collateral
    ///
    /// This is the main shared object representing a protocol instance.
    /// Manages all reserves, token supplies, fees, and staking operations.
    ///
    /// ## Staking Model (Three-Tier Liquidity)
    /// 1. **reserve_buffer_sui**: Liquid SUI (~5%) for instant redemptions
    /// 2. **stake.active_fss**: Staked FSS earning validator rewards
    /// 3. **pending_redemptions**: FIFO queue for redemptions exceeding liquidity
    ///
    /// ## Accounting Model
    /// - reserve_balance_sui tracks principal only (basis for invariant and CR)
    /// - Staking rewards are tracked separately and don't affect CR or invariant
    /// - Fee treasury accumulates FSS separately from main reserves
    public struct Protocol<phantom CoinTypeF, phantom CoinTypeX> has key, store {
        id: UID,
        /// ID of the authorized stability pool for this protocol
        authorized_pool_id: ID,
        /// Admin capability for stability pool operations (held internally)
        pool_admin_cap: stability_pool::StabilityPoolAdminCap,

        // ========================================================================
        // Staking-Based Reserve Model
        // ========================================================================

        /// Liquid SUI buffer for instant redemptions (~5% target)
        reserve_buffer_sui: Balance<SUI>,
        /// Current validator pool ID selected by gauge
        current_pool_id: ID,
        /// Current validator address for staking operations
        current_validator_address: address,
        /// Consolidated stake tracking for the current validator
        stake: PoolStake,
        /// Tracked reserve balance (principal only, excludes staking rewards)
        reserve_balance_sui: u64,
        /// Principal amount that has converted to active FSS (for reward calculation)
        active_principal_sui: u64,

        // ========================================================================
        // Redemption Configuration
        // ========================================================================

        /// Ticket expiration period in milliseconds (configurable by admin)
        ticket_expiration_ms: u64,
        /// Operation fee for delegate redemptions (fixed SUI amount, e.g., 1000000000 = 1 SUI)
        delegate_redeem_operation_fee: u64,

        // ========================================================================
        // Token Management
        // ========================================================================

        /// Treasury capability for minting/burning fTokens
        stable_treasury_cap: TreasuryCap<CoinTypeF>,
        /// Treasury capability for minting/burning xTokens
        leverage_treasury_cap: TreasuryCap<CoinTypeX>,

        /// Last observed SUI price from oracle (1e9 scale)
        last_reserve_price: u64,
        /// fToken price: fixed at $1.00 (1e9 scale)
        pf: u64,
        /// xToken price: calculated via invariant (1e9 scale)
        px: u64,

        // ========================================================================
        // Fee Management
        // ========================================================================

        /// Fee treasury holding accumulated fees as FungibleStakedSui
        fee_treasury_balance: option::Option<FungibleStakedSui>,
        /// Temporary tracking of fees in SUI before batch staking
        fees_collected_sui: u64,
        /// Fee rate configuration for all operations
        fee_config: FeeConfig,

        // ========================================================================
        // Staking Configuration
        // ========================================================================

        /// Staking strategy parameters
        staking_config: StakingConfig,

        // ========================================================================
        // Control Flags
        // ========================================================================

        /// Whether user mint/redeem actions are enabled (admin controlled)
        allow_user_actions: bool,

        // ========================================================================
        // Oracle Tracking
        // ========================================================================

        /// Timestamp of last oracle price update (milliseconds)
        last_oracle_ts: u64,

        // ========================================================================
        // Supply Tracking
        // ========================================================================

        /// Total fToken supply (tracked manually for test compatibility)
        stable_supply: u64,
        /// Total xToken supply (tracked manually for test compatibility)
        leverage_supply: u64,
    }

    // Events
    public struct MintF has copy, drop {
        user: address,
        reserve_in: u64,
        f_minted: u64,
        fee_charged: u64,
        bonus_earned: u64,
        cr_level: u8,
    }

    public struct MintX has copy, drop {
        user: address,
        reserve_in: u64,
        x_minted: u64,
        fee_charged: u64,
        bonus_earned: u64,
        cr_level: u8,
    }

    public struct RedeemF has copy, drop {
        user: address,
        f_burned: u64,
        reserve_out: u64,
        fee_charged: u64,
        bonus_earned: u64,
        cr_level: u8,
    }

    public struct RedeemX has copy, drop {
        user: address,
        x_burned: u64,
        reserve_out: u64,
        fee_charged: u64,
        bonus_earned: u64,
        cr_level: u8,
    }

    public struct PriceUpdate has copy, drop {
        old_price: u64,
        new_price: u64,
        timestamp: u64,
    }

    public struct FeeCharged has copy, drop {
        user: address,
        action: vector<u8>, // "mint_f", "mint_x", "redeem_f", "redeem_x"
        fee_amount: u64,
        cr_level: u8,
    }

    public struct BonusPaid has copy, drop {
        user: address,
        action: vector<u8>,
        bonus_amount: u64,
        cr_level: u8,
    }

    public struct RedeemQueued has copy, drop {
        user: address,
        ticket_id: ID,
        amount: u64,
        expiration: u64,
    }

    public struct RedemptionClaimed has copy, drop {
        user: address,
        amount: u64,
    }

    public struct TicketExpired has copy, drop {
        user: address,
        amount: u64,
        expiration: u64,
        reclaimed_at: u64,
    }


    /// Initialize a new Leafsii protocol instance with initial collateral split 50/50
    ///
    /// This function creates the core protocol state, mints initial tokens, and links to a stability pool.
    /// The reserve is split equally by USD value into fToken and xToken supplies.
    ///
    /// # Parameters
    /// - `stable_treasury_cap`: Treasury capability for minting/burning stable tokens (F)
    /// - `leverage_treasury_cap`: Treasury capability for minting/burning leverage tokens (X)
    /// - `initial_price`: Initial price of reserve token in micro-USD (1e6 scale)
    /// - `reserve_in`: Initial reserve token deposit
    /// - `pool`: Stability pool to authorize for this protocol
    /// - `pool_admin_cap`: Admin capability for the stability pool (will be stored in Protocol)
    /// - `clock`: Clock object for timestamps
    ///
    /// # Returns
    /// - Initial fToken coins
    /// - Initial xToken coins
    /// - AdminCap for protocol administration
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If reserve_in is zero
    public fun init_protocol<CoinTypeF, CoinTypeX>(
        stable_treasury_cap: TreasuryCap<CoinTypeF>,
        leverage_treasury_cap: TreasuryCap<CoinTypeX>,
        initial_price: u64,
        reserve_in: Coin<SUI>,
        pool: &mut StabilityPool<CoinTypeF>,
        pool_admin_cap: stability_pool::StabilityPoolAdminCap,
        clock: &Clock,
        ctx: &mut TxContext
    ): (Coin<CoinTypeF>, Coin<CoinTypeX>, AdminCap) {
        let reserve_amount = coin::value(&reserve_in);
        assert!(reserve_amount > 0, E_INVALID_AMOUNT);

        let fee_config = FeeConfig {
            normal_mint_f_fee_bps: DEFAULT_NORMAL_FEE_BPS,
            normal_mint_x_fee_bps: DEFAULT_NORMAL_FEE_BPS,
            normal_redeem_f_fee_bps: DEFAULT_NORMAL_FEE_BPS,
            normal_redeem_x_fee_bps: DEFAULT_NORMAL_FEE_BPS,
            l1_redeem_x_fee_bps: 100, // 1.0% increased fee
            stability_bonus_rate_bps: BONUS_RATE_BPS,
            fee_recipient: @0x0, // Default to zero, admin can update
        };

        let staking_config = StakingConfig {
            target_buffer_bps: 500,  // 5% target buffer
            validator_gauge_id: object::id_from_address(@0x0),  // Will be set by admin
        };

        // Create protocol with new staking model
        let reserve_amount = coin::value(&reserve_in);
        let protocol_uid = object::new(ctx);
        let mut protocol = Protocol {
            id: protocol_uid,
            authorized_pool_id: stability_pool::pool_id(pool),
            pool_admin_cap,                                     // Store the admin cap internally
            // New staking-based reserve model
            reserve_buffer_sui: coin::into_balance(reserve_in), // Initially all in buffer
            current_pool_id: object::id_from_address(@0x0),    // Will be set by validator gauge  // TODO pass a default one in the beginning
            current_validator_address: @0x0,                    // Will be set by validator gauge  // TODO pass a default one in the beginning
            stake: staking_helper::new_pool_stake(ctx),         // Empty initial stake
            reserve_balance_sui: reserve_amount,                // Initialize with reserve amount
            active_principal_sui: 0,                            // No active stakes initially
            // Redemption configuration
            ticket_expiration_ms: DEFAULT_TICKET_EXPIRATION_MS, // 7 days default
            delegate_redeem_operation_fee: DEFAULT_DELEGATE_OPERATION_FEE, // 1 SUI default
            stable_treasury_cap,
            leverage_treasury_cap,
            last_reserve_price: initial_price,
            pf: PF_FIXED,
            px: initial_price,
            fee_treasury_balance: option::none(),  // No FSS initially
            fees_collected_sui: 0,  // Initialize fee tracking
            fee_config,
            staking_config,
            allow_user_actions: true,
            last_oracle_ts: timestamp_ms(clock),
            stable_supply: 0,
            leverage_supply: 0,
        };

        // Split USD value 50/50 into fToken and xToken
        let usd_in = math::mul_to_u128(reserve_amount, initial_price);
        let usd_f = usd_in / 2;
        let usd_x = usd_in - usd_f;
        
        // Mint fToken
        let f_mint = math::div_back_u64(usd_f, PF_FIXED);
        let f_coin = coin::mint(&mut protocol.stable_treasury_cap, f_mint, ctx);
        protocol.stable_supply = f_mint;
        
        // Mint xToken - initialize px if Nx == 0
        let x_mint = math::div_back_u64(usd_x, protocol.px);
        let x_coin = coin::mint(&mut protocol.leverage_treasury_cap, x_mint, ctx);
        protocol.leverage_supply = x_mint;
        
        // Update px via invariant
        update_px(&mut protocol);

        // Create AdminCap bound to this Protocol instance
        let admin_cap = AdminCap {
            id: object::new(ctx),
            protocol_id: object::id(&protocol),
        };

        transfer::public_share_object(protocol);

        (f_coin, x_coin, admin_cap)
    }


    /// Verifies that the provided stability pool is authorized for this protocol
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance to check
    /// - `pool`: The stability pool to verify
    ///
    /// # Aborts
    /// - `E_UNAUTHORIZED_POOL`: If pool ID doesn't match protocol's authorized pool
    fun assert_authorized_pool<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>
    ) {
        assert!(stability_pool::pool_id(pool) == protocol.authorized_pool_id, E_UNAUTHORIZED_POOL);
    }

    /// Calculate total USD value of protocol's reserve tokens
    ///
    /// This is based on reserve_balance_sui (principal only, excludes staking rewards).
    /// The invariant Reserve USD = fToken USD + xToken USD is based on principal,
    /// because tokens were minted based on principal amounts.
    ///
    /// Staking rewards are tracked separately and don't affect the invariant or CR.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance
    ///
    /// # Returns
    /// Total USD value as u128 (using 1e9 scale)
    fun reserve_usd<CoinTypeF, CoinTypeX>(protocol: &Protocol<CoinTypeF, CoinTypeX>): u128 {
        // Use tracked reserve balance (principal only, excludes staking rewards)
        let p = protocol.last_reserve_price;
        return math::mul_to_u128(protocol.reserve_balance_sui, p)
    }
    
    /// Calculate actual total reserve including staking rewards
    ///
    /// This represents the true economic value of reserves but is NOT used
    /// for the invariant equation, which is based on principal only.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance
    ///
    /// # Returns
    /// Total reserve SUI amount (principal + rewards)
    #[allow(unused_function)]
    fun reserve_total_with_rewards<CoinTypeF, CoinTypeX>(protocol: &Protocol<CoinTypeF, CoinTypeX>): u64 {
        // Get active FSS value (includes principal + rewards)
        let active_fss_value = staking_helper::get_active_fss_amount(&protocol.stake);

        // Calculate rewards: FSS value - original principal staked
        let rewards_in_sui = if (active_fss_value > protocol.active_principal_sui) {
            active_fss_value - protocol.active_principal_sui
        } else {
            0
        };

        // Total reserve = tracked principal + rewards
        protocol.reserve_balance_sui + rewards_in_sui
    }

    /// Calculate net USD value of protocol reserves after stability pool obligations
    ///
    /// This is based on principal only (excludes staking rewards) to be consistent
    /// with the invariant equation and token minting logic.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance
    /// - `pool`: The authorized stability pool
    ///
    /// # Returns
    /// Net reserve USD value as u128
    fun reserve_net_usd<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>
    ): u128 {
        assert_authorized_pool(protocol, pool);
        // Use tracked reserve balance (excludes staking rewards)
        let reserve_amount = protocol.reserve_balance_sui;
        let sp_obligation = stability_pool::get_sp_obligation_amount(pool);
        let reserve_net = if (reserve_amount > sp_obligation) {
            reserve_amount - sp_obligation
        } else {
            0
        };
        math::mul_to_u128(reserve_net, protocol.last_reserve_price)
    }
    
    /// Calculate the current collateral ratio of the protocol
    ///
    /// CR = (Net Reserve USD Value) / (Total fToken USD Value)
    /// Result is scaled by SCALE_FACTOR (1e9) for precision.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance
    /// - `pool`: The authorized stability pool
    ///
    /// # Returns
    /// Collateral ratio scaled by 1e9 (e.g., 1.5 = 1,500,000,000)
    public fun collateral_ratio<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>
    ): u64 {
        assert_authorized_pool(protocol, pool);
        let nf = get_total_stable_supply(protocol);
        let total_val_f = math::mul_to_u128(nf, protocol.pf);
        if (total_val_f <= (EPS as u128)) {
            return CR_T_L1 + 1
        };
        let reserve_net = reserve_net_usd(protocol, pool);
        math::mul_div_back_u64(reserve_net, SCALE_FACTOR as u128, total_val_f)
    }
    
    /// Determine the current operational level based on collateral ratio
    ///
    /// Levels:
    /// - 0: Normal mode (CR >= 130.6%)
    /// - 1: L1 Stability mode (120.6% <= CR < 130.6%)
    /// - 2: L2 User Rebalance mode (114.4% <= CR < 120.6%)
    /// - 3: L3 Protocol Rebalance mode (CR < 114.4%)
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance
    /// - `pool`: The authorized stability pool
    ///
    /// # Returns
    /// Current operational level (0-3)
    public fun current_level<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>
    ): u8 {
        assert_authorized_pool(protocol, pool);
        let cr = collateral_ratio(protocol, pool);
        if (cr >= CR_T_L1) LEVEL_NORMAL                      // Normal mode
        else if (cr >= CR_T_L2) LEVEL_L1_STABILITY           // L1 - Stability mode
        else if (cr >= CR_T_L3) LEVEL_L2_USER_REBALANCE      // L2 - User Rebalance mode
        else LEVEL_L3_PROTOCOL_REBALANCE                     // L3 - Protocol Rebalance mode
    }
    
    /// Check if fToken minting is allowed based on current CR level
    fun require_ftoken_mint_allowed<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>
    ) {
        assert_authorized_pool(protocol, pool);
        let level = current_level(protocol, pool);
        assert!(level == LEVEL_NORMAL, E_ACTION_BLOCKED_BY_CR);  // Only allow in Normal mode
    }

    /// Get total circulating stable token supply
    ///
    /// Uses treasury capability to query total minted supply.
    /// This represents all fTokens in circulation.
    public fun get_total_stable_supply<CoinTypeF, CoinTypeX>(protocol: &Protocol<CoinTypeF, CoinTypeX>): u64 {
        protocol.stable_treasury_cap.total_supply()
    }

    /// Get total circulating leverage token supply
    ///
    /// Uses treasury capability to query total minted supply.
    /// This represents all xTokens in circulation.
    public fun get_total_leverage_supply<CoinTypeF, CoinTypeX>(protocol: &Protocol<CoinTypeF, CoinTypeX>): u64 {
        protocol.leverage_treasury_cap.total_supply()
    }

    /// Calculate fee amount from base amount and fee rate in basis points
    ///
    /// # Parameters
    /// - `amount`: Base amount to calculate fee on
    /// - `fee_bps`: Fee rate in basis points (e.g., 50 = 0.5%)
    ///
    /// # Returns
    /// Fee amount (amount * fee_bps / 10000)
    fun calculate_fee_amount(amount: u64, fee_bps: u64): u64 {
        math::mul_div(amount, fee_bps, FEE_BASIS_POINTS)
    }

    /// Calculate bonus amount from base amount and bonus rate in basis points
    ///
    /// # Parameters
    /// - `amount`: Base amount to calculate bonus on
    /// - `bonus_bps`: Bonus rate in basis points (e.g., 10 = 0.1%)
    ///
    /// # Returns
    /// Bonus amount (amount * bonus_bps / 10000)
    fun calculate_bonus_amount(amount: u64, bonus_bps: u64): u64 {
        math::mul_div(amount, bonus_bps, FEE_BASIS_POINTS)
    }

    /// Get fee rate for fToken minting based on operational level
    ///
    /// Returns 0 in L1+ modes because fToken minting is disabled when CR < 130.6%
    fun get_mint_f_fee_rate<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        level: u8
    ): u64 {
        if (level == LEVEL_NORMAL) protocol.fee_config.normal_mint_f_fee_bps
        else 0 // fToken mint disabled in L1+
    }

    /// Get fee rate for xToken minting
    ///
    /// Returns the same fee rate across all operational levels.
    /// xToken minting is always allowed as it improves collateralization.
    fun get_mint_x_fee_rate<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        _level: u8
    ): u64 {
        protocol.fee_config.normal_mint_x_fee_bps
    }

    /// Get fee rate for fToken redemption based on operational level
    ///
    /// Returns 0% in L1+ modes to encourage fToken redemption,
    /// which helps restore healthy collateralization.
    fun get_redeem_f_fee_rate<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        level: u8
    ): u64 {
        if (level >= LEVEL_L1_STABILITY) 0 // 0% fee in L1, L2, L3 to encourage burning
        else protocol.fee_config.normal_redeem_f_fee_bps
    }

    /// Get fee rate for xToken redemption based on operational level
    ///
    /// Returns higher fee in L1 mode (default 1.0%) to discourage xToken redemption
    /// when protocol is in stability mode. Normal fee otherwise.
    fun get_redeem_x_fee_rate<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        level: u8
    ): u64 {
        if (level == LEVEL_L1_STABILITY) protocol.fee_config.l1_redeem_x_fee_bps // Increased fee in L1
        else protocol.fee_config.normal_redeem_x_fee_bps
    }

    /// Get bonus rate for xToken minting based on operational level
    ///
    /// Returns stability bonus in L1+ modes to incentivize xToken minting,
    /// which adds collateral and improves CR. Returns 0 in Normal mode.
    fun get_mint_x_bonus_rate<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        level: u8
    ): u64 {
        if (level >= LEVEL_L1_STABILITY) protocol.fee_config.stability_bonus_rate_bps // Bonus in L1, L2, L3
        else 0
    }

    /// Get bonus rate for fToken redemption based on operational level
    ///
    /// Returns stability bonus in L2+ modes to incentivize fToken burning,
    /// which reduces debt and improves CR. Returns 0 in Normal and L1 modes.
    fun get_redeem_f_bonus_rate<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        level: u8
    ): u64 {
        if (level >= LEVEL_L2_USER_REBALANCE) protocol.fee_config.stability_bonus_rate_bps // Bonus in L2, L3
        else 0
    }

    /// Update leverage token price based on protocol invariant
    ///
    /// Implements the core invariant equation:
    /// ```
    /// Reserve USD = fToken USD + xToken USD
    /// ```
    ///
    /// Solving for px (xToken price):
    /// ```
    /// px = (Reserve USD - fToken USD) / xToken Supply
    /// ```
    ///
    /// The xToken price captures all remaining value after accounting for
    /// fToken obligations. This makes xTokens leveraged - they absorb both
    /// upside and downside of reserve price movements.
    ///
    /// Sets px to 0 if:
    /// - No xTokens in circulation
    /// - Reserve value < fToken obligations
    fun update_px<CoinTypeF, CoinTypeX>(protocol: &mut Protocol<CoinTypeF, CoinTypeX>) {
        let nx = get_total_leverage_supply(protocol);
        if (nx <= EPS) {
            protocol.px = 0;
            return
        };

        let reserve_usd_val = reserve_usd(protocol);
        let nf = get_total_stable_supply(protocol);
        let nf_usd_val = math::mul_to_u128(nf, protocol.pf);

        if (reserve_usd_val > nf_usd_val) {
            let remaining_usd = reserve_usd_val - nf_usd_val;
            protocol.px = math::div_back_u64(remaining_usd, nx);
        } else {
            protocol.px = 0;
        }
    }

    /// Update protocol prices from oracle with staleness and step size checks
    ///
    /// This function updates the reserve token price and recalculates token values.
    /// Includes safety checks for oracle freshness and maximum price movements.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance (mutable)
    /// - `oracle`: The oracle providing price data
    /// - `clock`: Clock for timestamp verification
    /// - `admin`: Admin capability for authorization
    ///
    /// # Aborts
    /// - `E_INVALID_ADMIN`: If admin cap doesn't match protocol
    /// - `E_ORACLE_STALE`: If oracle data is too old
    /// - `E_ORACLE_STEP_TOO_LARGE`: If price change exceeds maximum allowed
    public fun update_from_oracle<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        oracle: &MockOracle<SUI>,
        clock: &Clock,
        admin: &AdminCap
    ) {
        // Verify AdminCap is bound to this Protocol instance
        assert!(admin.protocol_id == object::id(protocol), E_INVALID_ADMIN);
        let now_ts = timestamp_ms(clock);
        let new_price = oracle::get_price_e9(oracle, clock);
        let old_price = protocol.last_reserve_price;
        
        // Check staleness
        if (protocol.last_oracle_ts != 0) {
            let age = now_ts - protocol.last_oracle_ts;
            assert!(age <= MAX_STALENESS_MS, E_ORACLE_STALE);
            
            // Check max step
            let rel_change = if (old_price > new_price) {
                math::mul_div(old_price - new_price, SCALE_FACTOR, old_price)
            } else {
                math::mul_div(new_price - old_price, SCALE_FACTOR, old_price)
            };
            assert!(rel_change <= MAX_REL_STEP, E_ORACLE_STEP_TOO_LARGE);
        };

        protocol.last_reserve_price = new_price;
        protocol.last_oracle_ts = now_ts;
        protocol.pf = PF_FIXED;
        
        update_px(protocol);

        event::emit(PriceUpdate {
            old_price,
            new_price,
            timestamp: now_ts,
        });
    }

    /// Mint stable tokens (fTokens) in exchange for reserve tokens
    ///
    /// Only available in Normal mode (level 0). Charges fees and adds reserve to protocol.
    /// fTokens maintain stable $1.00 value and are backed by protocol reserves.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance (mutable)
    /// - `pool`: The authorized stability pool
    /// - `reserve_in`: Reserve tokens to deposit
    ///
    /// # Returns
    /// Minted fToken coins
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If deposit amount is zero
    /// - `E_MINT_BLOCKED`: If user actions are disabled
    /// - `E_ACTION_BLOCKED_BY_CR`: If not in Normal mode
    public fun mint_f<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>,
        reserve_in: Coin<SUI>,
        ctx: &mut TxContext
    ): Coin<CoinTypeF> {
        assert_authorized_pool(protocol, pool);
        let deposit_amount = coin::value(&reserve_in);
        assert!(deposit_amount > 0, E_INVALID_AMOUNT);
        assert!(protocol.allow_user_actions, E_MINT_BLOCKED);
        
        let level = current_level(protocol, pool);
        
        // Check if fToken mint is allowed - only in Normal mode
        require_ftoken_mint_allowed(protocol, pool);

        // Calculate fee and net deposit
        let fee_rate = get_mint_f_fee_rate(protocol, level);
        let fee_amount = calculate_fee_amount(deposit_amount, fee_rate);
        let net_deposit = deposit_amount - fee_amount;

        // Add net deposit to reserve, fee to fee treasury
        let reserve_balance = coin::into_balance(reserve_in);

        // Fees remain in SUI in reserve buffer and will be batch-staked during rebalance_buffer()
        // See rebalance_buffer() for SUI->FSS conversion implementation
        if (fee_amount > 0) {
            // Fee remains in reserve_balance and will be staked
            // Track fees collected in SUI temporarily
            protocol.fees_collected_sui = protocol.fees_collected_sui + fee_amount;
            // Proper implementation: convert SUI fee to FSS via staking
            event::emit(FeeCharged {
                user: tx_context::sender(ctx),
                action: b"mint_f",
                fee_amount,
                cr_level: level,
            });
        };
        // Apply new staking flow: buffer first, then stake excess (includes fees for now)
        apply_staking_flow(protocol, reserve_balance);

        // Update reserve balance tracking (excludes staking rewards and fees)
        protocol.reserve_balance_sui = protocol.reserve_balance_sui + net_deposit;

        // Issue fTokens at Pf based on net deposit
        let usd_value = math::mul_to_u128(net_deposit, protocol.last_reserve_price);
        let f_to_mint = math::div_back_u64(usd_value, protocol.pf);
        
        let f_coin = coin::mint(&mut protocol.stable_treasury_cap, f_to_mint, ctx);
        protocol.stable_supply = protocol.stable_supply + f_to_mint;
        
        update_px(protocol);

        event::emit(MintF {
            user: tx_context::sender(ctx),
            reserve_in: deposit_amount,
            f_minted: f_to_mint,
            fee_charged: fee_amount,
            bonus_earned: 0, // No bonus for fToken mint
            cr_level: level,
        });

        return f_coin
    }

    /// Apply incoming balance from user flows: add to buffer for later staking.
    ///
    /// User deposit flows only update the buffer - they do not perform staking operations.
    /// All staking is handled asynchronously by keeper functions (rebalance_buffer).
    ///
    /// This approach:
    /// - Keeps user transactions fast and simple
    /// - Prevents double-counting of staked amounts
    /// - Allows batched, optimized staking via keepers
    fun apply_staking_flow<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        incoming_balance: Balance<SUI>,
    ) {
        // Simply join all incoming funds to buffer
        // Staking will be handled asynchronously by rebalance_buffer keeper function
        balance::join(&mut protocol.reserve_buffer_sui, incoming_balance);
    }

    /// Process redemption using three-tier liquidity model
    ///
    /// Implements the redemption flow with immediate payout when possible:
    ///
    /// 1. **Immediate Payout**: If buffer has sufficient SUI, pay immediately
    /// 2. **Partial + Ticket**: If buffer insufficient, pay what's available and return ticket for remainder
    /// 3. **Ticket Only**: If buffer empty, return ticket for entire amount
    ///
    /// Tickets are returned to users as owned objects that they can claim later.
    /// This prevents keeper-based dust attacks and gives users control over claim timing.
    ///
    /// # Parameters
    /// - `amount_needed`: Total SUI amount required for redemption
    /// - `user`: Address to receive future payout for queued amount
    /// - `delegate_enabled`: Whether to allow keeper delegation (false = self-redeem only)
    /// - `clock`: Clock for setting expiration timestamp
    ///
    /// # Returns
    /// Tuple of (immediate_sui_coin, optional_redemption_ticket)
    fun apply_redemption_flow<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        amount_needed: u64,
        user: address,
        delegate_enabled: bool,
        clock: &Clock,
        ctx: &mut TxContext
    ): (Coin<SUI>, option::Option<RedemptionTicket>) {
        let buffer_amount = balance::value(&protocol.reserve_buffer_sui);

        if (buffer_amount >= amount_needed) {
            // Sufficient buffer - pay immediately
            let reserve_balance = balance::split(&mut protocol.reserve_buffer_sui, amount_needed);
            (coin::from_balance(reserve_balance, ctx), option::none())
        } else {
            // Insufficient buffer - pay what we can and create redemption ticket
            let immediate_payout = if (buffer_amount > 0) {
                let immediate_balance = balance::split(&mut protocol.reserve_buffer_sui, buffer_amount);
                coin::from_balance(immediate_balance, ctx)
            } else {
                coin::zero<SUI>(ctx)
            };

            // Create redemption ticket for shortfall with expiration
            let shortfall = amount_needed - buffer_amount;
            let current_time = timestamp_ms(clock);
            let expiration = current_time + protocol.ticket_expiration_ms;

            // Determine operation fee based on delegation setting
            let operation_fee = if (delegate_enabled) {
                protocol.delegate_redeem_operation_fee
            } else {
                0
            };

            let ticket = staking_helper::new_redemption_ticket(
                user,
                shortfall,
                expiration,
                operation_fee,
                delegate_enabled,
                ctx
            );
            let ticket_id = object::id(&ticket);

            // Emit RedeemQueued event
            event::emit(RedeemQueued {
                user,
                ticket_id,
                amount: shortfall,
                expiration,
            });

            (immediate_payout, option::some(ticket))
        }
    }

    /// Mint leverage tokens (xTokens) in exchange for reserve tokens
    ///
    /// Available in all modes. xTokens capture upside/downside of reserve price movements.
    /// Provides stability bonuses in risk modes (L1+) to incentivize system rebalancing.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance (mutable)
    /// - `pool`: The authorized stability pool
    /// - `reserve_in`: Reserve tokens to deposit
    ///
    /// # Returns
    /// Minted xToken coins
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If deposit amount is zero
    /// - `E_MINT_BLOCKED`: If user actions are disabled
    /// - `E_INSUFFICIENT_RESERVE`: If insufficient fee treasury for bonus
    public fun mint_x<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>,
        reserve_in: Coin<SUI>,
        ctx: &mut TxContext
    ): Coin<CoinTypeX> {
        assert_authorized_pool(protocol, pool);
        let deposit_amount = coin::value(&reserve_in);
        assert!(deposit_amount > 0, E_INVALID_AMOUNT);
        assert!(protocol.allow_user_actions, E_MINT_BLOCKED);
        
        let level = current_level(protocol, pool);

        // Calculate fee and bonus
        let fee_rate = get_mint_x_fee_rate(protocol, level);
        let bonus_rate = get_mint_x_bonus_rate(protocol, level);
        let fee_amount = calculate_fee_amount(deposit_amount, fee_rate);

        // Phase 4: Enable bonus for mint_x in L1+ modes
        // Bonus increases minting power without requiring FSS->SUI conversion
        let bonus_amount = calculate_bonus_amount(deposit_amount, bonus_rate);

        // Net deposit after fee, plus bonus
        let net_deposit = deposit_amount - fee_amount + bonus_amount;

        // Handle reserve and fee accounting
        let reserve_balance = coin::into_balance(reserve_in);

        // Fees remain in SUI in reserve buffer and will be batch-staked during rebalance_buffer()
        // See rebalance_buffer() for SUI->FSS conversion implementation
        if (fee_amount > 0) {
            // Fee remains in reserve_balance and will be staked
            // Track fees collected in SUI temporarily
            protocol.fees_collected_sui = protocol.fees_collected_sui + fee_amount;
            // Proper implementation: convert SUI fee to FSS via staking
            event::emit(FeeCharged {
                user: tx_context::sender(ctx),
                action: b"mint_x",
                fee_amount,
                cr_level: level,
            });
        };

        // Apply new staking flow: buffer first, then stake excess (includes fees for now)
        apply_staking_flow(protocol, reserve_balance);

        // Update reserve balance tracking (excludes staking rewards and fees)
        protocol.reserve_balance_sui = protocol.reserve_balance_sui + (deposit_amount - fee_amount);

        // Phase 4: Bonus for mint_x is virtual (increases minting power)
        // No actual SUI transfer needed, just emit event
        if (bonus_amount > 0) {
            event::emit(BonusPaid {
                user: tx_context::sender(ctx),
                action: b"mint_x",
                bonus_amount,
                cr_level: level,
            });
        };

        // Initialize px if Nx == 0
        let nx = get_total_leverage_supply(protocol);
        if (nx == 0) {
            protocol.px = protocol.last_reserve_price;
        };

        // Calculate x tokens to mint based on net value
        let usd_value = math::mul_to_u128(net_deposit, protocol.last_reserve_price);
        let x_to_mint = math::div_back_u64(usd_value, protocol.px);

        let x_coin = coin::mint(&mut protocol.leverage_treasury_cap, x_to_mint, ctx);
        protocol.leverage_supply = protocol.leverage_supply + x_to_mint;
        
        update_px(protocol);

        event::emit(MintX {
            user: tx_context::sender(ctx),
            reserve_in: deposit_amount,
            x_minted: x_to_mint,
            fee_charged: fee_amount,
            bonus_earned: bonus_amount,
            cr_level: level,
        });

        x_coin
    }

    /// Redeem stable tokens (fTokens) for reserve tokens
    ///
    /// Burns fTokens and returns equivalent reserve value. Provides stability bonuses
    /// in L2+ modes to incentivize fToken burning when protocol is undercollateralized.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance (mutable)
    /// - `pool`: The authorized stability pool
    /// - `f_in`: fTokens to redeem
    /// - `clock`: Clock for setting ticket expiration
    ///
    /// # Returns
    /// Tuple of (immediate_sui_coin, optional_redemption_ticket)
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If redeem amount is zero
    /// - `E_MINT_BLOCKED`: If user actions are disabled
    /// - `E_INSUFFICIENT_RESERVE`: If insufficient net reserves or fee treasury
    public fun redeem_f<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>,
        f_in: Coin<CoinTypeF>,
        clock: &Clock,
        ctx: &mut TxContext
    ): (Coin<SUI>, option::Option<RedemptionTicket>) {
        assert_authorized_pool(protocol, pool);
        let f_amount = coin::value(&f_in);
        assert!(f_amount > 0, E_INVALID_AMOUNT);
        assert!(protocol.allow_user_actions, E_MINT_BLOCKED);
        
        let level = current_level(protocol, pool);

        // Calculate base reserve to pay out
        let usd_value = math::mul_to_u128(f_amount, protocol.pf);
        let base_reserve_out = math::div_back_u64(usd_value, protocol.last_reserve_price);

        // Calculate fee and bonus
        let fee_rate = get_redeem_f_fee_rate(protocol, level);
        let bonus_rate = get_redeem_f_bonus_rate(protocol, level);
        let fee_amount = calculate_fee_amount(base_reserve_out, fee_rate);

        // Phase 4: Enable bonus for redeem_f in L2+ modes
        // Bonus is actual SUI payout to incentivize fToken burning
        let mut bonus_amount = calculate_bonus_amount(base_reserve_out, bonus_rate);

        // Check reserve sufficiency net-of-obligations
        // Use tracked reserve balance (includes buffer + staked principal + collected fees)
        let reserve_amount = protocol.reserve_balance_sui + protocol.fees_collected_sui;
        let sp_obligation = stability_pool::get_sp_obligation_amount(pool);
        let reserve_net = if (reserve_amount > sp_obligation) {
            reserve_amount - sp_obligation
        } else {
            0
        };

        // Calculate base payout after fee
        let base_minus_fee = base_reserve_out - fee_amount;

        // Must have enough reserve for at least the base payout
        assert!(base_minus_fee <= reserve_net, E_INSUFFICIENT_RESERVE);

        // Cap bonus to fit within remaining reserve
        if (base_minus_fee + bonus_amount > reserve_net) {
            bonus_amount = reserve_net - base_minus_fee;
        };

        // Final reserve out after fee deduction and bonus addition (bonus may be capped)
        let final_reserve_out = base_minus_fee + bonus_amount;

        // Burn f tokens
        coin::burn(&mut protocol.stable_treasury_cap, f_in);
        protocol.stable_supply = protocol.stable_supply - f_amount;

        // Apply new redemption flow: buffer first, return ticket if insufficient
        // Default to self-redeem (delegate_enabled = false)
        let user = tx_context::sender(ctx);
        let (reserve_coin, ticket_option) = apply_redemption_flow(protocol, final_reserve_out, user, false, clock, ctx);

        // Update reserve balance tracking (excludes staking rewards)
        // Decrease by base amount; fee stays in reserve, bonus temporarily from reserve (should be from fee treasury)
        protocol.reserve_balance_sui = protocol.reserve_balance_sui - (base_reserve_out - bonus_amount);

        // Handle fee - remains in reserve buffer and will be batch-staked during rebalance_buffer()
        // See rebalance_buffer() for SUI->FSS conversion implementation
        if (fee_amount > 0) {
            // Fee amount is already deducted from payout, stays in reserve buffer
            // Track fees collected in SUI temporarily
            protocol.fees_collected_sui = protocol.fees_collected_sui + fee_amount;
            event::emit(FeeCharged {
                user: tx_context::sender(ctx),
                action: b"redeem_f",
                fee_amount,
                cr_level: level,
            });
        };

        // Phase 4: Bonus for redeem_f is real SUI payout
        // Bonus is included in final_reserve_out and paid via redemption flow
        // Note: Bonus ideally comes from fee treasury FSS->SUI conversion,
        // but for now it's drawn from the buffer along with base redemption
        if (bonus_amount > 0) {
            event::emit(BonusPaid {
                user: tx_context::sender(ctx),
                action: b"redeem_f",
                bonus_amount,
                cr_level: level,
            });
        };

        update_px(protocol);

        event::emit(RedeemF {
            user: tx_context::sender(ctx),
            f_burned: f_amount,
            reserve_out: final_reserve_out,
            fee_charged: fee_amount,
            bonus_earned: bonus_amount,
            cr_level: level,
        });

        (reserve_coin, ticket_option)
    }

    /// Redeem leverage tokens (xTokens) for reserve tokens
    ///
    /// Burns xTokens and returns current market value in reserves. Fees vary by
    /// operational level - higher fees in L1 mode to discourage xToken redemption.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance (mutable)
    /// - `pool`: The authorized stability pool
    /// - `x_in`: xTokens to redeem
    /// - `clock`: Clock for setting ticket expiration
    ///
    /// # Returns
    /// Tuple of (immediate_sui_coin, optional_redemption_ticket)
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If redeem amount is zero
    /// - `E_MINT_BLOCKED`: If user actions are disabled
    /// - `E_INSUFFICIENT_RESERVE`: If insufficient net reserves
    public fun redeem_x<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>,
        x_in: Coin<CoinTypeX>,
        clock: &Clock,
        ctx: &mut TxContext
    ): (Coin<SUI>, option::Option<RedemptionTicket>) {
        assert_authorized_pool(protocol, pool);
        let x_amount = coin::value(&x_in);
        assert!(x_amount > 0, E_INVALID_AMOUNT);
        assert!(protocol.allow_user_actions, E_MINT_BLOCKED);

        let level = current_level(protocol, pool);

        // Calculate base reserve to pay out using current px
        let usd_value = math::mul_to_u128(x_amount, protocol.px);
        let base_reserve_out = math::div_back_u64(usd_value, protocol.last_reserve_price);

        // Calculate fee (no bonus for xToken redeem)
        let fee_rate = get_redeem_x_fee_rate(protocol, level);
        let fee_amount = calculate_fee_amount(base_reserve_out, fee_rate);
        let final_reserve_out = base_reserve_out - fee_amount;

        // Check reserve sufficiency net-of-obligations
        // Use tracked reserve balance (includes buffer + staked principal + collected fees)
        let reserve_amount = protocol.reserve_balance_sui + protocol.fees_collected_sui;
        let sp_obligation = stability_pool::get_sp_obligation_amount(pool);
        let reserve_net = if (reserve_amount > sp_obligation) {
            reserve_amount - sp_obligation
        } else {
            0
        };
        assert!(final_reserve_out <= reserve_net, E_INSUFFICIENT_RESERVE);

        // Burn x tokens
        coin::burn(&mut protocol.leverage_treasury_cap, x_in);
        protocol.leverage_supply = protocol.leverage_supply - x_amount;

        // Apply new redemption flow: buffer first, return ticket if insufficient
        // Default to self-redeem (delegate_enabled = false)
        let user = tx_context::sender(ctx);
        let (reserve_coin, ticket_option) = apply_redemption_flow(protocol, final_reserve_out, user, false, clock, ctx);

        // Update reserve balance tracking (excludes staking rewards)
        // Decrease by base amount; fee stays in reserve (no bonus for xToken redeem)
        protocol.reserve_balance_sui = protocol.reserve_balance_sui - base_reserve_out;

        // Handle fee - remains in reserve buffer and will be batch-staked during rebalance_buffer()
        // See rebalance_buffer() for SUI->FSS conversion implementation
        if (fee_amount > 0) {
            // Fee amount is already deducted from payout, stays in reserve buffer
            // Track fees collected in SUI temporarily
            protocol.fees_collected_sui = protocol.fees_collected_sui + fee_amount;
            event::emit(FeeCharged {
                user: tx_context::sender(ctx),
                action: b"redeem_x",
                fee_amount,
                cr_level: level,
            });
        };

        update_px(protocol);

        event::emit(RedeemX {
            user: tx_context::sender(ctx),
            x_burned: x_amount,
            reserve_out: final_reserve_out,
            fee_charged: fee_amount,
            bonus_earned: 0, // No bonus for xToken redeem
            cr_level: level,
        });

        (reserve_coin, ticket_option)
    }

    /// Redeem stable tokens (fTokens) with delegation to keeper
    ///
    /// Same as `redeem_f` but creates tickets with delegation enabled.
    /// User pays an operation fee upfront, and keepers can execute redemption
    /// once liquidity becomes available.
    ///
    /// # Parameters
    /// - Same as `redeem_f`
    ///
    /// # Returns
    /// Tuple of (immediate_sui_coin, optional_redemption_ticket)
    /// If ticket is returned, it has delegate_enabled = true
    public fun redeem_f_delegate<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>,
        f_in: Coin<CoinTypeF>,
        clock: &Clock,
        ctx: &mut TxContext
    ): (Coin<SUI>, option::Option<RedemptionTicket>) {
        assert_authorized_pool(protocol, pool);
        let f_amount = coin::value(&f_in);
        assert!(f_amount > 0, E_INVALID_AMOUNT);
        assert!(protocol.allow_user_actions, E_MINT_BLOCKED);

        let level = current_level(protocol, pool);

        // Calculate base reserve to pay out
        let usd_value = math::mul_to_u128(f_amount, protocol.pf);
        let base_reserve_out = math::div_back_u64(usd_value, protocol.last_reserve_price);

        // Calculate fee and bonus
        let fee_rate = get_redeem_f_fee_rate(protocol, level);
        let bonus_rate = get_redeem_f_bonus_rate(protocol, level);
        let fee_amount = calculate_fee_amount(base_reserve_out, fee_rate);
        let mut bonus_amount = calculate_bonus_amount(base_reserve_out, bonus_rate);

        // Check reserve sufficiency
        let reserve_amount = protocol.reserve_balance_sui + protocol.fees_collected_sui;
        let sp_obligation = stability_pool::get_sp_obligation_amount(pool);
        let reserve_net = if (reserve_amount > sp_obligation) {
            reserve_amount - sp_obligation
        } else {
            0
        };

        let base_minus_fee = base_reserve_out - fee_amount;
        assert!(base_minus_fee <= reserve_net, E_INSUFFICIENT_RESERVE);

        if (base_minus_fee + bonus_amount > reserve_net) {
            bonus_amount = reserve_net - base_minus_fee;
        };

        let final_reserve_out = base_minus_fee + bonus_amount;

        // Burn f tokens
        coin::burn(&mut protocol.stable_treasury_cap, f_in);
        protocol.stable_supply = protocol.stable_supply - f_amount;

        // Apply redemption flow with delegation enabled
        let user = tx_context::sender(ctx);
        let (reserve_coin, ticket_option) = apply_redemption_flow(protocol, final_reserve_out, user, true, clock, ctx);

        // Update reserve balance tracking
        protocol.reserve_balance_sui = protocol.reserve_balance_sui - (base_reserve_out - bonus_amount);

        if (fee_amount > 0) {
            protocol.fees_collected_sui = protocol.fees_collected_sui + fee_amount;
            event::emit(FeeCharged {
                user: tx_context::sender(ctx),
                action: b"redeem_f",
                fee_amount,
                cr_level: level,
            });
        };

        if (bonus_amount > 0) {
            event::emit(BonusPaid {
                user: tx_context::sender(ctx),
                action: b"redeem_f",
                bonus_amount,
                cr_level: level,
            });
        };

        update_px(protocol);

        event::emit(RedeemF {
            user: tx_context::sender(ctx),
            f_burned: f_amount,
            reserve_out: final_reserve_out,
            fee_charged: fee_amount,
            bonus_earned: bonus_amount,
            cr_level: level,
        });

        (reserve_coin, ticket_option)
    }

    /// Redeem leverage tokens (xTokens) with delegation to keeper
    ///
    /// Same as `redeem_x` but creates tickets with delegation enabled.
    /// User pays an operation fee upfront, and keepers can execute redemption
    /// once liquidity becomes available.
    ///
    /// # Parameters
    /// - Same as `redeem_x`
    ///
    /// # Returns
    /// Tuple of (immediate_sui_coin, optional_redemption_ticket)
    /// If ticket is returned, it has delegate_enabled = true
    public fun redeem_x_delegate<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        pool: &StabilityPool<CoinTypeF>,
        x_in: Coin<CoinTypeX>,
        clock: &Clock,
        ctx: &mut TxContext
    ): (Coin<SUI>, option::Option<RedemptionTicket>) {
        assert_authorized_pool(protocol, pool);
        let x_amount = coin::value(&x_in);
        assert!(x_amount > 0, E_INVALID_AMOUNT);
        assert!(protocol.allow_user_actions, E_MINT_BLOCKED);

        let level = current_level(protocol, pool);

        // Calculate base reserve to pay out
        let usd_value = math::mul_to_u128(x_amount, protocol.px);
        let base_reserve_out = math::div_back_u64(usd_value, protocol.last_reserve_price);

        // Calculate fee
        let fee_rate = get_redeem_x_fee_rate(protocol, level);
        let fee_amount = calculate_fee_amount(base_reserve_out, fee_rate);
        let final_reserve_out = base_reserve_out - fee_amount;

        // Check reserve sufficiency
        let reserve_amount = protocol.reserve_balance_sui + protocol.fees_collected_sui;
        let sp_obligation = stability_pool::get_sp_obligation_amount(pool);
        let reserve_net = if (reserve_amount > sp_obligation) {
            reserve_amount - sp_obligation
        } else {
            0
        };
        assert!(final_reserve_out <= reserve_net, E_INSUFFICIENT_RESERVE);

        // Burn x tokens
        coin::burn(&mut protocol.leverage_treasury_cap, x_in);
        protocol.leverage_supply = protocol.leverage_supply - x_amount;

        // Apply redemption flow with delegation enabled
        let user = tx_context::sender(ctx);
        let (reserve_coin, ticket_option) = apply_redemption_flow(protocol, final_reserve_out, user, true, clock, ctx);

        // Update reserve balance tracking
        protocol.reserve_balance_sui = protocol.reserve_balance_sui - base_reserve_out;

        if (fee_amount > 0) {
            protocol.fees_collected_sui = protocol.fees_collected_sui + fee_amount;
            event::emit(FeeCharged {
                user: tx_context::sender(ctx),
                action: b"redeem_x",
                fee_amount,
                cr_level: level,
            });
        };

        update_px(protocol);

        event::emit(RedeemX {
            user: tx_context::sender(ctx),
            x_burned: x_amount,
            reserve_out: final_reserve_out,
            fee_charged: fee_amount,
            bonus_earned: 0,
            cr_level: level,
        });

        (reserve_coin, ticket_option)
    }

    /// Keeper executes a delegated redemption ticket
    ///
    /// Allows keepers to execute redemptions on behalf of users who have opted in
    /// to delegation. The keeper receives the operation fee as compensation.
    ///
    /// # Parameters
    /// - `ticket`: Redemption ticket with delegation enabled
    /// - `clock`: Clock for expiration checking
    ///
    /// # Returns
    /// Tuple of (sui_for_user, sui_for_keeper_fee)
    ///
    /// # Aborts
    /// - `E_INVALID_ADMIN`: If ticket doesn't have delegation enabled (reusing error code)
    /// - `E_ORACLE_STALE`: If ticket has expired
    /// - `E_INSUFFICIENT_RESERVE`: If insufficient liquidity
    public fun keeper_execute_redemption<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        wrapper: &mut SuiSystemState,
        ticket: RedemptionTicket,
        clock: &Clock,
        ctx: &mut TxContext
    ): (Coin<SUI>, Coin<SUI>) {
        let current_time = timestamp_ms(clock);

        // Check if ticket is expired
        assert!(!staking_helper::is_ticket_expired(&ticket, current_time), E_ORACLE_STALE);

        // Check if delegation is enabled
        assert!(staking_helper::is_delegate_enabled(&ticket), E_INVALID_ADMIN);

        // Extract ticket data
        let user = staking_helper::get_ticket_user(&ticket);
        let amount = staking_helper::get_ticket_amount(&ticket);
        let operation_fee = staking_helper::get_ticket_operation_fee(&ticket);

        // Destroy ticket
        let (_user, _amount, _expiration) = staking_helper::destroy_ticket(ticket);

        let mut buffer_amount = balance::value(&protocol.reserve_buffer_sui);

        // If buffer insufficient, try redeeming FSS
        if (buffer_amount < amount + operation_fee) {
            let total_needed = amount + operation_fee;
            let deficit = total_needed - buffer_amount;
            let active_fss_value = staking_helper::get_active_fss_amount(&protocol.stake);

            assert!(active_fss_value >= deficit, E_INSUFFICIENT_RESERVE);

            // Redeem FSS to cover deficit
            let fss_to_redeem = staking_helper::split_active_fss(&mut protocol.stake, deficit, ctx);
            let redeemed_balance = sui_system::sui_system::redeem_fungible_staked_sui(
                wrapper,
                fss_to_redeem,
                ctx
            );

            balance::join(&mut protocol.reserve_buffer_sui, redeemed_balance);
            buffer_amount = balance::value(&protocol.reserve_buffer_sui);
        };

        // Now buffer must have sufficient amount
        assert!(buffer_amount >= amount + operation_fee, E_INSUFFICIENT_RESERVE);

        // Pay user amount
        let user_payout_balance = balance::split(&mut protocol.reserve_buffer_sui, amount);
        let user_coin = coin::from_balance(user_payout_balance, ctx);

        // Pay keeper operation fee
        let keeper_fee_balance = balance::split(&mut protocol.reserve_buffer_sui, operation_fee);
        let keeper_coin = coin::from_balance(keeper_fee_balance, ctx);

        // Emit event
        event::emit(RedemptionClaimed {
            user,
            amount,
        });

        (user_coin, keeper_coin)
    }

    // ========================================================================
    // Admin Functions
    // ========================================================================

    /// Enable or disable user mint/redeem actions
    ///
    /// Admin emergency function to pause protocol operations if needed.
    /// Does not affect keeper functions or admin operations.
    ///
    /// # Aborts
    /// - `E_INVALID_ADMIN`: If admin cap doesn't match this protocol
    public fun set_user_actions_allowed<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        allowed: bool,
        admin: &AdminCap
    ) {
        assert!(admin.protocol_id == object::id(protocol), E_INVALID_ADMIN);
        protocol.allow_user_actions = allowed;
    }

    /// Update protocol fee configuration
    ///
    /// Sets fee rates (in basis points) for all protocol operations.
    /// Fees apply based on operational level (Normal vs L1/L2/L3 modes).
    ///
    /// # Parameters
    /// - All fee rates in basis points (e.g., 50 = 0.5%)
    ///
    /// # Aborts
    /// - `E_INVALID_ADMIN`: If admin cap doesn't match this protocol
    public fun set_fee_config<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        normal_mint_f_fee_bps: u64,
        normal_mint_x_fee_bps: u64,
        normal_redeem_f_fee_bps: u64,
        normal_redeem_x_fee_bps: u64,
        l1_redeem_x_fee_bps: u64,
        stability_bonus_rate_bps: u64,
        admin: &AdminCap
    ) {
        assert!(admin.protocol_id == object::id(protocol), E_INVALID_ADMIN);
        protocol.fee_config.normal_mint_f_fee_bps = normal_mint_f_fee_bps;
        protocol.fee_config.normal_mint_x_fee_bps = normal_mint_x_fee_bps;
        protocol.fee_config.normal_redeem_f_fee_bps = normal_redeem_f_fee_bps;
        protocol.fee_config.normal_redeem_x_fee_bps = normal_redeem_x_fee_bps;
        protocol.fee_config.l1_redeem_x_fee_bps = l1_redeem_x_fee_bps;
        protocol.fee_config.stability_bonus_rate_bps = stability_bonus_rate_bps;
    }

    /// Set the recipient address for protocol fees
    ///
    /// Fees are accumulated in fee_treasury_balance as FungibleStakedSui.
    /// Admin can withdraw to this recipient address.
    ///
    /// # Aborts
    /// - `E_INVALID_ADMIN`: If admin cap doesn't match this protocol
    public fun set_fee_recipient<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        recipient: address,
        admin: &AdminCap
    ) {
        assert!(admin.protocol_id == object::id(protocol), E_INVALID_ADMIN);
        protocol.fee_config.fee_recipient = recipient;
    }

    /// Withdraw accumulated fees from fee treasury
    ///
    /// Extracts FungibleStakedSui from the fee treasury.
    /// Can withdraw partial amount or entire treasury.
    ///
    /// # Parameters
    /// - `amount`: Amount of FSS to withdraw
    ///
    /// # Returns
    /// FungibleStakedSui that can be redeemed for SUI
    ///
    /// # Aborts
    /// - `E_INVALID_ADMIN`: If admin cap doesn't match this protocol
    /// - `E_INSUFFICIENT_RESERVE`: If amount exceeds treasury balance
    public fun withdraw_fees<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        amount: u64,
        admin: &AdminCap,
        ctx: &mut TxContext
    ): FungibleStakedSui {
        assert!(admin.protocol_id == object::id(protocol), E_INVALID_ADMIN);
        assert!(option::is_some(&protocol.fee_treasury_balance), E_INSUFFICIENT_RESERVE);

        let treasury_value = staking_pool::fungible_staked_sui_value(option::borrow(&protocol.fee_treasury_balance));
        assert!(amount <= treasury_value, E_INSUFFICIENT_RESERVE);

        let treasury = option::borrow_mut(&mut protocol.fee_treasury_balance);

        if (amount == treasury_value) {
            option::extract(&mut protocol.fee_treasury_balance)
        } else {
            staking_pool::split_fungible_staked_sui(treasury, amount, ctx)
        }
    }

    /// Set the operation fee for delegate redemptions
    ///
    /// Configures the fixed fee amount (in SUI) that users pay when they
    /// opt into delegate redemptions. This fee is paid to keepers who
    /// execute the redemption on behalf of the user.
    ///
    /// # Parameters
    /// - `new_fee`: Fee amount in SUI (e.g., 1_000_000_000 = 1 SUI)
    ///
    /// # Aborts
    /// - `E_INVALID_ADMIN`: If admin cap doesn't match this protocol
    public fun set_delegate_operation_fee<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        new_fee: u64,
        admin: &AdminCap
    ) {
        assert!(admin.protocol_id == object::id(protocol), E_INVALID_ADMIN);
        protocol.delegate_redeem_operation_fee = new_fee;
    }

    // View functions
    public fun get_protocol_state<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>
    ): (u64, u64, u64, u64, u64, u64, u64, bool) {
        let nf_supply = get_total_stable_supply(protocol);
        let nx_supply = get_total_leverage_supply(protocol);
        (
            nf_supply,
            nx_supply,
            protocol.pf,
            protocol.px,
            protocol.last_reserve_price,
            protocol.reserve_balance_sui,  // Return tracked reserve balance instead of buffer
            protocol.fees_collected_sui,  // Return SUI fees collected (temporary until FSS conversion)
            protocol.allow_user_actions
        )
    }

    public fun get_fee_config<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>
    ): (u64, u64, u64, u64, u64, u64, address) {
        (
            protocol.fee_config.normal_mint_f_fee_bps,
            protocol.fee_config.normal_mint_x_fee_bps,
            protocol.fee_config.normal_redeem_f_fee_bps,
            protocol.fee_config.normal_redeem_x_fee_bps,
            protocol.fee_config.l1_redeem_x_fee_bps,
            protocol.fee_config.stability_bonus_rate_bps,
            protocol.fee_config.fee_recipient
        )
    }

    /// Execute L3 protocol rebalancing to restore target collateral ratio
    ///
    /// In L3 mode (CR < 114.4%), this function burns fTokens from the stability pool
    /// to reduce total fToken supply and restore healthy collateral ratios.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance (mutable)
    /// - `pool`: The stability pool (mutable)
    /// - `target_cr`: Target collateral ratio to achieve
    /// - `admin`: Protocol admin capability
    public fun protocol_rebalance_l3_to_target<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        pool: &mut StabilityPool<CoinTypeF>,
        target_cr: u64,
        admin: &AdminCap,
        ctx: &mut TxContext
    ) {
        assert_authorized_pool(protocol, pool);
        // Verify AdminCap is bound to this Protocol instance
        assert!(admin.protocol_id == object::id(protocol), E_INVALID_ADMIN);
        let nf = get_total_stable_supply(protocol);
        if (nf <= EPS) return;

        // Compute f_burn needed for target CR
        let reserve_net = reserve_net_usd(protocol, pool);
        let nf_target_usd = math::mul_div_back_u64(reserve_net, SCALE_FACTOR as u128, target_cr as u128);
        let nf_target = math::mul_div(nf_target_usd, SCALE_FACTOR, protocol.pf);

        if (nf_target >= nf) return;  // Already at target or better

        let f_burn_needed = nf - nf_target;
        let f_burn_cap = stability_pool::sp_quote_burn_cap(pool);
        let f_burn = if (f_burn_needed > f_burn_cap) f_burn_cap else f_burn_needed;

        if (f_burn <= EPS) return;

        // Calculate payout amount
        let payout_r_amount = math::mul_div(f_burn, protocol.pf, protocol.last_reserve_price);

        // Call SP controller - use internally stored capability
        let (actual_burn, _actual_payout) = stability_pool::sp_controller_rebalance(
            pool,
            &protocol.pool_admin_cap,
            f_burn,
            payout_r_amount,
        );

        if (actual_burn > 0) {
            // Burn from SP pool - now done after scale update for proper coupling
            stability_pool::burn_from_pool(pool, &protocol.pool_admin_cap, actual_burn, &mut protocol.stable_treasury_cap, ctx);

            // Update protocol state
            protocol.stable_supply = protocol.stable_supply - actual_burn;
            update_px(protocol);
        };
    }
    
    /// Claim stability pool rewards for a user position (public entry function)
    ///
    /// This is the permissionless entry point that allows any user to claim
    /// their own stability pool rewards without needing the admin capability.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance (mutable)
    /// - `pool`: The stability pool (mutable)
    /// - `position`: User's stability pool position (mutable)
    ///
    /// # Returns
    /// Reserve token coins representing claimed rewards
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If no rewards to claim
    /// - `E_INSUFFICIENT_RESERVE`: If protocol lacks reserves to pay
    public fun claim_sp_rewards<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        pool: &mut StabilityPool<CoinTypeF>,
        position: &mut SPPosition<CoinTypeF>,
        ctx: &mut TxContext
    ): Coin<SUI> {
        assert_authorized_pool(protocol, pool);
        let owed = stability_pool::settle_user(pool, position);
        assert!(owed > 0, E_INVALID_AMOUNT);

        // Check reserve sufficiency
        assert!(owed <= balance::value(&protocol.reserve_buffer_sui), E_INSUFFICIENT_RESERVE);

        // Pay out from reserve
        let reserve_balance = balance::split(&mut protocol.reserve_buffer_sui, owed);
        let reserve_coin = coin::from_balance(reserve_balance, ctx);

        // Update tracked reserve balance
        protocol.reserve_balance_sui = protocol.reserve_balance_sui - owed;

        // Decrease SP obligation - use internally stored capability
        stability_pool::decrease_obligation(pool, &protocol.pool_admin_cap, owed);

        reserve_coin
    }

    /// Verify the protocol's accounting invariant holds
    /// Reserve USD = fToken USD + xToken USD (within epsilon tolerance)
    public(package) fun check_invariant<CoinTypeF, CoinTypeX>(protocol: &Protocol<CoinTypeF, CoinTypeX>): bool {
        let reserve_usd_val = reserve_usd(protocol);
        let nf_supply = get_total_stable_supply(protocol);
        let nx_supply = get_total_leverage_supply(protocol);
        let nf_usd = math::mul_to_u128(nf_supply, protocol.pf);
        let nx_usd = math::mul_to_u128(nx_supply, protocol.px);
        let rhs = nf_usd + nx_usd;
        
        // Allow small tolerance for rounding
        let diff = if (reserve_usd_val > rhs) {
            reserve_usd_val - rhs
        } else {
            rhs - reserve_usd_val
        };
        
        diff <= (EPS as u128)
    }

    /// Get current reserve token balance
    public fun get_reserve_balance<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>
    ): u64 {
        protocol.reserve_balance_sui
    }

    // ========================================================================
    // Maintenance Entry Functions (Idempotent, Public)
    // ========================================================================

    /// Convert matured stakes to FungibleStakedSui and update principal tracking
    ///
    /// This keeper function processes pending stakes that have become active:
    /// 1. Identifies StakedSui entries with activation_epoch <= current_epoch
    /// 2. Converts them to FungibleStakedSui for efficient management
    /// 3. Consolidates into protocol's active FSS pool
    /// 4. Updates active_principal tracking for reward calculation
    ///
    /// Keeper function - callable by anyone to maintain protocol health.
    /// Designed to be called regularly (e.g., every epoch).
    ///
    /// # Parameters
    /// - `current_epoch`: Current Sui epoch number
    /// - `max_items`: Maximum stakes to process (for gas limiting)
    public fun settle_and_consolidate<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        wrapper: &mut SuiSystemState,
        current_epoch: u64,
        max_items: u64,
        ctx: &mut TxContext
    ) {
        // Convert matured StakedSui to FungibleStakedSui and consolidate
        let processed = staking_helper::convert_and_consolidate_matured_stakes(
            wrapper,
            &mut protocol.stake,
            current_epoch,
            max_items,
            ctx
        );

        // Update active_principal_sui to track principal that's now active FSS
        // This allows us to calculate staking rewards accurately
        if (processed > 0) {
            protocol.active_principal_sui = staking_helper::get_total_principal(&protocol.stake);
        };
    }

    /// User claims their redemption ticket when liquidity is available
    ///
    /// Allows users to claim their queued redemption without relying on keepers.
    /// This prevents dust attack vectors where attackers create many small tickets.
    ///
    /// # Parameters
    /// - `ticket`: User's redemption ticket (will be destroyed after claim)
    /// - `clock`: Clock for timestamp verification
    ///
    /// # Returns
    /// SUI coin with the redemption amount
    ///
    /// # Aborts
    /// - `E_INVALID_ADMIN`: If caller is not the ticket owner (reusing error code)
    /// - `E_ORACLE_STALE`: If ticket has expired (reusing error code)
    /// - `E_INSUFFICIENT_RESERVE`: If insufficient buffer/FSS to fulfill redemption
    public fun claim_redemption<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        wrapper: &mut SuiSystemState,
        ticket: RedemptionTicket,
        clock: &Clock,
        ctx: &mut TxContext
    ): Coin<SUI> {
        let current_time = timestamp_ms(clock);

        // Check if ticket is expired
        assert!(!staking_helper::is_ticket_expired(&ticket, current_time), E_ORACLE_STALE);

        let (user, amount, _expiration) = staking_helper::destroy_ticket(ticket);

        // Verify caller is the ticket owner
        assert!(user == tx_context::sender(ctx), E_INVALID_ADMIN);

        let mut buffer_amount = balance::value(&protocol.reserve_buffer_sui);

        // If buffer insufficient, try redeeming FSS
        if (buffer_amount < amount) {
            let deficit = amount - buffer_amount;
            let active_fss_value = staking_helper::get_active_fss_amount(&protocol.stake);

            assert!(active_fss_value >= deficit, E_INSUFFICIENT_RESERVE);

            // Redeem FSS to cover deficit
            let fss_to_redeem = staking_helper::split_active_fss(&mut protocol.stake, deficit, ctx);
            let redeemed_balance = sui_system::sui_system::redeem_fungible_staked_sui(
                wrapper,
                fss_to_redeem,
                ctx
            );

            balance::join(&mut protocol.reserve_buffer_sui, redeemed_balance);
            buffer_amount = balance::value(&protocol.reserve_buffer_sui);
        };

        // Now buffer must have sufficient amount
        assert!(buffer_amount >= amount, E_INSUFFICIENT_RESERVE);

        // Pay from buffer
        let payout_balance = balance::split(&mut protocol.reserve_buffer_sui, amount);

        // Note: reserve_balance_sui was already decremented when ticket was created
        // No need to adjust it again here

        event::emit(RedemptionClaimed {
            user,
            amount,
        });

        coin::from_balance(payout_balance, ctx)
    }

    /// Reclaim funds from expired redemption tickets
    ///
    /// Anyone can call this to clean up expired tickets and return funds to protocol.
    /// This prevents indefinite capital lock-up from abandoned tickets.
    ///
    /// # Parameters
    /// - `ticket`: An expired redemption ticket
    /// - `clock`: Clock for timestamp verification
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If ticket has not expired yet
    public fun reclaim_expired_ticket<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        ticket: RedemptionTicket,
        clock: &Clock,
    ) {
        let current_time = timestamp_ms(clock);

        // Verify ticket is actually expired
        assert!(staking_helper::is_ticket_expired(&ticket, current_time), E_INVALID_AMOUNT);

        let (user, amount, expiration) = staking_helper::destroy_ticket(ticket);

        // Funds are already in reserve, just update accounting
        // The amount was already deducted from reserve_balance_sui when ticket was created,
        // so we add it back now that the ticket is expired
        protocol.reserve_balance_sui = protocol.reserve_balance_sui + amount;

        // Emit event for tracking
        event::emit(TicketExpired {
            user,
            amount,
            expiration,
            reclaimed_at: current_time,
        });
    }


    /// Rebalance liquid buffer to maintain target ratio
    ///
    /// This keeper function manages the buffer-staking balance:
    ///
    /// **When Buffer > Target:**
    /// 1. Stakes excess SUI as principal (improves yield)
    /// 2. Stakes collected fees separately (enables fee tracking)
    /// 3. Converts matured stakes to FSS
    /// 4. Splits fee portion into fee_treasury_balance
    ///
    /// **When Buffer < Target:**
    /// - Redeems FSS from fee treasury to restore buffer liquidity
    ///
    /// This maintains ~5% liquid buffer for instant redemptions while
    /// maximizing staked amount for yield generation.
    ///
    /// Keeper function - callable by anyone to maintain protocol health.
    /// Should be called after user operations or when buffer deviates from target.
    ///
    /// # Parameters
    /// - `current_epoch`: Current Sui epoch number
    /// - `max_stake`: Maximum SUI to stake per call (for gas limiting)
    public fun rebalance_buffer<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        wrapper: &mut SuiSystemState,
        current_epoch: u64,
        max_stake: u64,
        ctx: &mut TxContext
    ) {
        // 1. Calculate current buffer vs target
        let current_buffer = balance::value(&protocol.reserve_buffer_sui);
        let total_reserve = staking_helper::get_total_reserve(current_buffer, &protocol.stake);
        let target_buffer = (total_reserve * protocol.staking_config.target_buffer_bps) / 10000;

        if (current_buffer > target_buffer) {
            // 2. Buffer exceeds target - stake excess up to max_stake (principal path)
            let excess = current_buffer - target_buffer;
            let stake_amount = if (excess > max_stake) max_stake else excess;

            if (stake_amount > 0 && protocol.current_validator_address != @0x0) {
                // Split SUI from buffer and create coin
                let stake_balance = balance::split(&mut protocol.reserve_buffer_sui, stake_amount);
                let stake_coin = coin::from_balance(stake_balance, ctx);

                // Use Sui system to create StakedSui
                let staked_sui = sui_system::sui_system::request_add_stake_non_entry(
                    wrapper,
                    stake_coin,
                    protocol.current_validator_address,
                    ctx
                );
                let activation_epoch = current_epoch + 1;

                // Record in pending stakes
                staking_helper::add_pending_stake(&mut protocol.stake, staked_sui, activation_epoch);

                // Update total principal tracking
                staking_helper::update_total_principal(&mut protocol.stake, stake_amount, true);
            };

            // 3. Stake collected fees separately (fee path)
            if (protocol.fees_collected_sui > 0 && protocol.current_validator_address != @0x0) {
                let fee_amount = protocol.fees_collected_sui;
                let fee_balance = balance::split(&mut protocol.reserve_buffer_sui, fee_amount);
                let fee_coin = coin::from_balance(fee_balance, ctx);

                // Stake fee SUI to create StakedSui for fee path
                let fee_staked_sui = sui_system::sui_system::request_add_stake_non_entry(
                    wrapper,
                    fee_coin,
                    protocol.current_validator_address,
                    ctx
                );
                let fee_activation_epoch = current_epoch + 1;

                // Add fee stakes with tracking (is_fee = true)
                staking_helper::add_pending_stake_with_fee_tracking(
                    &mut protocol.stake,
                    fee_staked_sui,
                    fee_activation_epoch,
                    true  // Mark as fee stake
                );

                // Update total principal for fee stakes
                staking_helper::update_total_principal(&mut protocol.stake, fee_amount, true);

                // Reset fees_collected_sui since they're now staked
                protocol.fees_collected_sui = 0;
            };

            // 4. Convert matured stakes to FSS and split out fee portion
            let (_, mut fee_fss_option) = staking_helper::convert_and_consolidate_matured_stakes_with_fees(
                wrapper,
                &mut protocol.stake,
                current_epoch,
                max_stake,  // Use same limit for conversion
                ctx
            );

            // 5. Add fee portion to treasury
            // Note: This solves Blocker #1 (SUI->FSS fee conversion)
            if (option::is_some(&fee_fss_option)) {
                let fee_fss = option::extract(&mut fee_fss_option);

                // Join into treasury or create new treasury
                if (option::is_some(&protocol.fee_treasury_balance)) {
                    let treasury_ref = option::borrow_mut(&mut protocol.fee_treasury_balance);
                    staking_pool::join_fungible_staked_sui(treasury_ref, fee_fss);
                } else {
                    option::fill(&mut protocol.fee_treasury_balance, fee_fss);
                };
            };
            option::destroy_none(fee_fss_option);
        } else if (current_buffer < target_buffer) {
            // 6. Buffer below target - redeem FSS from fee treasury to top up
            let deficit = target_buffer - current_buffer;

            if (option::is_some(&protocol.fee_treasury_balance)) {
                let treasury_value = staking_pool::fungible_staked_sui_value(option::borrow(&protocol.fee_treasury_balance));

                if (treasury_value > 0) {
                    // Redeem up to deficit amount from treasury
                    let redeem_amount = if (treasury_value >= deficit) { deficit } else { treasury_value };

                    // Get FSS to redeem
                    let redeem_fss = if (redeem_amount == treasury_value) {
                        // Extract entire treasury if redeeming all
                        option::extract(&mut protocol.fee_treasury_balance)
                    } else {
                        // Split partial amount from treasury
                        let treasury_ref = option::borrow_mut(&mut protocol.fee_treasury_balance);
                        staking_pool::split_fungible_staked_sui(treasury_ref, redeem_amount, ctx)
                    };

                    // Redeem FSS for SUI via sui_system
                    let sui_balance = sui_system::sui_system::redeem_fungible_staked_sui(
                        wrapper,
                        redeem_fss,
                        ctx
                    );

                    // Add redeemed SUI to buffer
                    balance::join(&mut protocol.reserve_buffer_sui, sui_balance);
                };
            };
        };
    }

    // ========================================================================
    // Admin Functions for Staking Model
    // ========================================================================

    /// Update target buffer percentage
    ///
    /// Sets the target percentage of reserves to keep liquid (not staked).
    /// Default is 500 bps = 5%.
    ///
    /// # Parameters
    /// - `new_target_bps`: New target in basis points (e.g., 500 = 5%)
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If new_target_bps > 10000 (100%)
    public fun update_target_buffer_bps<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        _admin_cap: &AdminCap,
        new_target_bps: u64
    ) {
        assert!(new_target_bps <= 10000, E_INVALID_AMOUNT);
        protocol.staking_config.target_buffer_bps = new_target_bps;
    }

    /// Update the validator gauge ID
    ///
    /// Sets the ID of the gauge contract that determines validator pool selection.
    /// Used for dynamic validator selection based on performance/voting.
    ///
    /// # Parameters
    /// - `new_gauge_id`: ID of the new validator gauge contract
    public fun update_validator_gauge<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        _admin_cap: &AdminCap,
        new_gauge_id: ID
    ) {
        protocol.staking_config.validator_gauge_id = new_gauge_id;
    }

    /// Update ticket expiration period
    ///
    /// Sets how long users have to claim their redemption tickets.
    /// Recommended: 7 days (default) to balance user convenience and capital efficiency.
    ///
    /// # Parameters
    /// - `new_expiration_ms`: New expiration period in milliseconds
    ///
    /// # Aborts
    /// - `E_INVALID_ADMIN`: If admin cap doesn't match this protocol
    /// - `E_INVALID_AMOUNT`: If expiration too short (< 1 hour) or too long (> 30 days)
    public fun set_ticket_expiration<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        new_expiration_ms: u64,
        admin: &AdminCap
    ) {
        assert!(admin.protocol_id == object::id(protocol), E_INVALID_ADMIN);

        // Enforce reasonable bounds: 1 hour minimum, 30 days maximum
        assert!(new_expiration_ms >= 3_600_000, E_INVALID_AMOUNT); // 1 hour
        assert!(new_expiration_ms <= 2_592_000_000, E_INVALID_AMOUNT); // 30 days

        protocol.ticket_expiration_ms = new_expiration_ms;
    }

    /// Get current ticket expiration period
    public fun get_ticket_expiration_period<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>
    ): u64 {
        protocol.ticket_expiration_ms
    }

    /// Update the current staking pool ID
    ///
    /// Sets which validator pool to use for new stakes.
    /// Used during pool rotation or validator changes.
    ///
    /// # Parameters
    /// - `new_pool_id`: ID of the new validator pool
    public fun update_current_pool_id<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        _admin_cap: &AdminCap,
        new_pool_id: ID
    ) {
        protocol.current_pool_id = new_pool_id;
    }

    /// Get current staking configuration (read-only)
    public fun get_staking_config<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>
    ): (u64, ID, ID) {
        (
            protocol.staking_config.target_buffer_bps,
            protocol.staking_config.validator_gauge_id,
            protocol.current_pool_id
        )
    }

    /// Get staking statistics (read-only)
    public fun get_staking_stats<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>
    ): (u64, u64, u64, u64, bool) {
        let buffer_amount = balance::value(&protocol.reserve_buffer_sui);
        let staked_amount = staking_helper::get_total_staked_amount(&protocol.stake);
        let pending_count = staking_helper::get_pending_stakes_count(&protocol.stake);
        let active_fss_amount = staking_helper::get_active_fss_amount(&protocol.stake);
        let has_active = staking_helper::has_active_fss(&protocol.stake);

        (buffer_amount, staked_amount, pending_count, active_fss_amount, has_active)
    }

    #[test_only]
    public fun test_create_admin_cap_for<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        ctx: &mut TxContext
    ): AdminCap {
        AdminCap {
            id: object::new(ctx),
            protocol_id: object::id(protocol),
        }
    }

    /// Test helper to drain buffer for testing ticket creation
    ///
    /// This function artificially removes SUI from the reserve buffer to simulate
    /// a scenario where most reserves are staked and buffer is depleted.
    /// This allows tests to verify ticket creation behavior during redemptions.
    ///
    /// # Parameters
    /// - `protocol`: The protocol instance (mutable)
    /// - `amount`: Amount of SUI to drain from buffer
    /// - `ctx`: Transaction context
    ///
    /// # Returns
    /// Coin containing the drained SUI
    ///
    /// # Note
    /// This is for testing only. In production, buffer depletion happens naturally
    /// as `rebalance_buffer()` moves excess SUI to staking.
    #[test_only]
    public fun test_drain_buffer<CoinTypeF, CoinTypeX>(
        protocol: &mut Protocol<CoinTypeF, CoinTypeX>,
        amount: u64,
        ctx: &mut TxContext
    ): Coin<SUI> {
        let drain_balance = balance::split(&mut protocol.reserve_buffer_sui, amount);
        coin::from_balance(drain_balance, ctx)
    }

    // ========================================================================
    // Frontend View Functions
    // ========================================================================

    /// Get the current px (leverage token price)
    public fun get_px<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>
    ): u64 {
        protocol.px
    }

    /// Get the current collateralization ratio
    /// Returns CR scaled by 1e9 (e.g., 1.5 = 1,500,000,000)
    public fun get_collateralization_ratio<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        oracle: &MockOracle<SUI>,
        clock: &Clock
    ): u64 {
        let current_buffer = balance::value(&protocol.reserve_buffer_sui);
        let total_reserve = staking_helper::get_total_reserve(current_buffer, &protocol.stake);
        // reserve_value_usd at 1e9 scale: SUI amount * price_e9
        let reserve_value_usd = math::mul_to_u128(total_reserve, oracle::get_price_e9(oracle, clock));

        // Debt values at 1e9 scale: supply * price (both pf and px are at 1e9)
        let stable_debt_usd = math::mul_to_u128(protocol.stable_supply, protocol.pf);
        let leverage_debt_usd = math::mul_to_u128(protocol.leverage_supply, protocol.px);
        let total_debt_usd = stable_debt_usd + leverage_debt_usd;

        if (total_debt_usd == 0) {
            // Infinite CR when no debt
            return 10_000_000_000 // Return a very large number to represent "infinite" at 1e9 scale
        };

        // CR = (reserve / debt) * SCALE_FACTOR to get result at 1e9 scale
        math::mul_div_back_u64(reserve_value_usd, SCALE_FACTOR as u128, total_debt_usd)
    }

    /// Get parameters for APY calculation
    /// Returns values needed by frontend to compute APY
    public fun get_apr_params<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>
    ): (u64, u64, u64, u64) {
        let current_buffer = balance::value(&protocol.reserve_buffer_sui);
        let total_reserve = staking_helper::get_total_reserve(current_buffer, &protocol.stake);
        let staked_amount = total_reserve - current_buffer;

        (
            total_reserve,      // Total protocol reserves
            staked_amount,      // Amount currently staked
            current_buffer,     // Liquid buffer amount
            protocol.staking_config.target_buffer_bps  // Target buffer percentage
        )
    }

    /// Get comprehensive protocol stats for UI
    public fun get_protocol_stats<CoinTypeF, CoinTypeX>(
        protocol: &Protocol<CoinTypeF, CoinTypeX>,
        oracle: &MockOracle<SUI>,
        clock: &Clock
    ): (u64, u64, u64, u64, u64, u64, u64) {
        let current_buffer = balance::value(&protocol.reserve_buffer_sui);
        let total_reserve = staking_helper::get_total_reserve(current_buffer, &protocol.stake);
        let cr = get_collateralization_ratio(protocol, oracle, clock);

        (
            protocol.px,                    // Current leverage token price
            cr,                            // Collateralization ratio
            total_reserve,                 // Total reserves
            current_buffer,                // Buffer amount
            protocol.stable_supply,        // Total stable token supply
            protocol.leverage_supply,      // Total leverage token supply
            if (option::is_some(&protocol.fee_treasury_balance)) {
                staking_pool::fungible_staked_sui_value(option::borrow(&protocol.fee_treasury_balance))
            } else {
                0
            }  // Fee treasury balance
        )
    }
}
