/// LFS Governance Token for Leafsii Protocol
///
/// LFS is the native governance and utility token of the Leafsii ecosystem with a
/// fixed maximum supply of 2,000,000 LFS (2M * 1e9 base units).
///
/// Token Economics:
/// - Hard Cap: 2,000,000 LFS total supply (immutable)
/// - Decimals: 9 (standard for Move tokens)
/// - Emissions: Controlled through EmissionsCap, separate from general treasury
///
/// Key Features:
/// 1. Supply Cap Enforcement:
///    All minting operations check against the 2M hard cap to ensure it cannot be exceeded
///
/// 2. Dual-Capability System:
///    - TreasuryCap: Standard Sui coin treasury for minting/burning operations
///    - EmissionsCap: Specialized cap for emissions-only minting with tracking
///
/// 3. Emissions Tracking:
///    The EmissionsCap tracks total emissions separately, enabling:
///    - Transparent emissions auditing
///    - Verification that emissions follow the defined schedule
///    - Clear separation between initial distribution and ongoing emissions
///
/// Token Utility:
/// - Governance: Vote on protocol parameters and upgrades
/// - ve-LFS: Lock for voting power and yield boosting (up to 2.5x)
/// - Gauge Voting: Direct weekly emissions to preferred gauges
/// - Staking Rewards: Earn from protocol fees and staking yields
///
/// Distribution:
/// - Weekly emissions following a 10% annual decay schedule
/// - Initial emission: 98,000 LFS per week
/// - Emissions directed by ve-LFS gauge votes
/// - Additional allocations via treasury for team, investors, ecosystem development
module leafsii::lfs_token {
    use sui::coin::{Self, Coin, TreasuryCap};
    use sui::event;
    use sui::object::new;

    // Error codes
    const E_INVALID_AMOUNT: u64 = 1;
    const E_TOTAL_SUPPLY_CAP_EXCEEDED: u64 = 3;

    // Constants
    const TOTAL_SUPPLY_CAP: u64 = 2_000_000_000_000_000; // 2M LFS with 9 decimals (2M * 1e9)

    // LFS Token witness type
    public struct LFS has drop {}

    // Capability to mint emissions (separate from initial treasury mint)
    public struct EmissionsCap has key, store {
        id: UID,
        emissions_minted: u64,
    }

    // Events
    public struct LFSInitialized has copy, drop {
        total_supply: u64,
        treasury_owner: address,
    }

    public struct EmissionsMinted has copy, drop {
        amount: u64,
        total_emissions_to_date: u64,
    }

    /// Initialize the LFS governance token with hard supply cap
    ///
    /// Creates the LFS token with metadata and returns capabilities for treasury
    /// management and emissions control. The token has a hard cap of 2M LFS.
    ///
    /// # Parameters
    /// - `treasury_owner`: Address to receive the token metadata
    ///
    /// # Returns
    /// - Treasury capability for minting/burning
    /// - Emissions capability for controlled minting
    public fun init_lfs(
        treasury_owner: address,
        ctx: &mut TxContext
    ): (TreasuryCap<LFS>, EmissionsCap) {
        // Create the LFS coin with 9 decimals (standard for tokens)
        let (treasury_cap, metadata) = coin::create_currency<LFS>(
            LFS {},
            9, // decimals
            b"LFS",
            b"Leafsii",
            b"Leafsii governance token for Leafsii Protocol",
            option::none(),
            ctx
        );

        // Transfer metadata to treasury owner
        sui::transfer::public_transfer(metadata, treasury_owner);

        // Create emissions capability with zero emissions minted initially
        let emissions_cap = EmissionsCap {
            id: new(ctx),
            emissions_minted: 0,
        };

        // Emit initialization event
        event::emit(LFSInitialized {
            total_supply: TOTAL_SUPPLY_CAP,
            treasury_owner,
        });

        (treasury_cap, emissions_cap)
    }

    /// Mint LFS tokens for emissions with supply cap enforcement
    ///
    /// Mints new LFS tokens for emissions distribution while enforcing the 2M
    /// total supply hard cap. Tracks emissions separately from other minting.
    ///
    /// # Parameters
    /// - `cap`: Emissions capability (mutable)
    /// - `amount`: Amount of LFS to mint
    /// - `treasury_cap`: Treasury capability for minting
    ///
    /// # Returns
    /// Newly minted LFS coins
    ///
    /// # Aborts
    /// - `E_INVALID_AMOUNT`: If amount is zero
    /// - `E_TOTAL_SUPPLY_CAP_EXCEEDED`: If minting would exceed 2M cap
    public(package) fun mint_emissions(
        cap: &mut EmissionsCap,
        amount: u64,
        treasury_cap: &mut TreasuryCap<LFS>,
        ctx: &mut TxContext
    ): Coin<LFS> {
        assert!(amount > 0, E_INVALID_AMOUNT);

        // Check that this emission doesn't exceed the total supply cap
        let new_emissions_total = cap.emissions_minted + amount;
        let current_supply = coin::total_supply(treasury_cap);

        assert!(
            current_supply + amount <= TOTAL_SUPPLY_CAP,
            E_TOTAL_SUPPLY_CAP_EXCEEDED
        );

        // Update emissions counter
        cap.emissions_minted = new_emissions_total;

        // Mint the tokens
        let minted_coin = coin::mint(treasury_cap, amount, ctx);

        // Emit event
        event::emit(EmissionsMinted {
            amount,
            total_emissions_to_date: new_emissions_total,
        });

        minted_coin
    }

    /// Get current total supply of LFS tokens
    ///
    /// # Parameters
    /// - `treasury_cap`: Treasury capability for supply queries
    ///
    /// # Returns
    /// Current total supply of LFS tokens
    public fun total_supply(treasury_cap: &TreasuryCap<LFS>): u64 {
        coin::total_supply(treasury_cap)
    }

    /// Get total amount of LFS minted through emissions
    ///
    /// # Parameters
    /// - `cap`: Emissions capability
    ///
    /// # Returns
    /// Total LFS minted through emissions system
    public(package) fun emissions_minted_to_date(cap: &EmissionsCap): u64 {
        cap.emissions_minted
    }

    /// Get the hard cap for total LFS supply
    ///
    /// # Returns
    /// Maximum possible LFS supply (2M with 9 decimals)
    public fun total_supply_cap(): u64 {
        TOTAL_SUPPLY_CAP
    }

    #[test_only]
    public fun init_for_testing(ctx: &mut TxContext): (TreasuryCap<LFS>, EmissionsCap) {
        let treasury_cap = coin::create_treasury_cap_for_testing<LFS>(ctx);

        let emissions_cap = EmissionsCap {
            id: new(ctx),
            emissions_minted: 0,
        };

        (treasury_cap, emissions_cap)
    }
}