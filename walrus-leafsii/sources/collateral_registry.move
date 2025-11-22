/// Collateral registry extension that tracks Walrus-backed cross-chain
/// collateral series and their oracle / risk configuration. The registry
/// stores the per-asset parameters (oracle ids, β split, LTV, Walrus proof
/// anchors) so FXN series can remain collateral-agnostic.
#[allow(duplicate_alias)]
module leafsii::collateral_registry {
    use std::option;
    use std::type_name::{Self as type_name, TypeName};
    use std::vector;

    use sui::event;
    use sui::object;
    use sui::object::{ID, UID};
    use sui::table::{Self as table, Table};
    use sui::tx_context;

    const E_COLLATERAL_EXISTS: u64 = 1;
    const E_UNKNOWN_COLLATERAL: u64 = 2;
    const E_WALRUS_REQUIRED: u64 = 3;
    const E_WALRUS_EXPIRED: u64 = 4;
    const E_COLLATERAL_INACTIVE: u64 = 5;
    const E_PARAM_INVALID: u64 = 6;

    const SCALE_FACTOR: u64 = 1_000_000_000;
    const MAX_LTV_BPS: u64 = 10_000;

    const ASSET_TYPE_NATIVE: u8 = 0;
    const ASSET_TYPE_CROSSCHAIN: u8 = 2;

    /// Reference to a Walrus blob that anchors custody proofs.
    #[allow(unused_field)]
    public struct WalrusBlobRef has copy, drop, store {
        blob_object_id: address,
        blob_id: vector<u8>,
        content_hash: vector<u8>,
        writer: address,
        non_deletable: bool,
        expires_at_epoch: u64,
    }

    /// Risk configuration for cross-chain collateral series.
    public struct CrosschainRiskConfig has copy, drop, store {
        ltv_ratio_e9: u64,
        maintenance_ratio_e9: u64,
        liquidation_penalty_e9: u64,
        oracle_haircut_e9: u64,
        staleness_cap_ms: u64,
        mint_limit_value_e9: u128,
        withdraw_limit_value_e9: u128,
    }

    /// Metadata describing a collateral type.
    public struct CollateralMetadata has store {
        type_name: TypeName,
        symbol: vector<u8>,
        asset_type: u8,
        price_feed_id: vector<u8>,
        walrus_required: bool,
        walrus_anchor: option::Option<WalrusBlobRef>,
        beta_e9: u64,
        ltv_bps: u64,
        max_capacity: u64,
        is_active: bool,
        last_anchor_epoch: u64,
        // Optional chain tag for cross-chain collaterals (e.g., b"ethereum")
        chain_tag: option::Option<vector<u8>>,
        crosschain_config: option::Option<CrosschainRiskConfig>,
    }

    /// Root object storing all registered collateral configs.
    public struct CollateralRegistry has key, store {
        id: UID,
        entries: Table<vector<u8>, CollateralMetadata>,
    }

    /// Event emitted whenever a new collateral type is registered.
    public struct CollateralRegistered has copy, drop {
        symbol: vector<u8>,
        asset_type: u8,
        beta_e9: u64,
        ltv_bps: u64,
    }

    /// Event emitted whenever risk params for a cross-chain collateral change.
    public struct CrosschainRiskParamsUpdated has copy, drop {
        symbol: vector<u8>,
        ltv_ratio_e9: u64,
        maintenance_ratio_e9: u64,
        liquidation_penalty_e9: u64,
        oracle_haircut_e9: u64,
        staleness_cap_ms: u64,
        mint_limit_value_e9: u128,
        withdraw_limit_value_e9: u128,
    }

    /// Event for Walrus anchor updates.
    public struct WalrusAnchorUpdated has copy, drop {
        symbol: vector<u8>,
        blob_object_id: address,
        expires_at_epoch: u64,
    }

    /// Event when governance toggles active status.
    public struct CollateralStatusChanged has copy, drop {
        symbol: vector<u8>,
        is_active: bool,
    }

    /// Create a fresh registry object.
    public fun init_registry(ctx: &mut tx_context::TxContext): CollateralRegistry {
        CollateralRegistry {
            id: object::new(ctx),
            entries: table::new<vector<u8>, CollateralMetadata>(ctx),
        }
    }

    /// Register a cross-chain collateral (e.g., ETH on Ethereum) validated via Walrus.
    /// The `symbol` should be a stable identifier like b"ETH" and `chain_tag` a tag like b"ethereum".
    /// `price_feed_id` references an oracle feed on Sui for the base asset USD price.
    public fun register_crosschain<PhantomMarker>(
        registry: &mut CollateralRegistry,
        symbol: vector<u8>,
        chain_tag: vector<u8>,
        price_feed_id: vector<u8>,
        beta_e9: u64,
        max_capacity: u64,
        walrus_anchor: WalrusBlobRef,
        risk_config: CrosschainRiskConfig,
        current_epoch: u64,
    ) {
        assert!(beta_e9 <= SCALE_FACTOR, E_PARAM_INVALID);
        assert!(max_capacity > 0, E_PARAM_INVALID);
        assert!(!table::contains(&registry.entries, clone_symbol(&symbol)), E_COLLATERAL_EXISTS);

        validate_walrus_anchor(&walrus_anchor, current_epoch);
        let validated_config = validate_crosschain_risk_config(risk_config);
        let ltv_bps = ratio_to_bps(validated_config.ltv_ratio_e9);

        let metadata = CollateralMetadata {
            // Cross-chain collateral has no on-chain Coin type; use marker type to bind defining ID
            type_name: type_name::with_defining_ids<PhantomMarker>(),
            symbol: clone_symbol(&symbol),
            asset_type: ASSET_TYPE_CROSSCHAIN,
            price_feed_id,
            walrus_required: true,
            walrus_anchor: option::some(walrus_anchor),
            beta_e9,
            ltv_bps,
            max_capacity,
            is_active: true,
            last_anchor_epoch: 0,
            chain_tag: option::some(chain_tag),
            crosschain_config: option::some(validated_config),
        };

        let emitted_symbol = clone_symbol(&metadata.symbol);
        table::add(&mut registry.entries, symbol, metadata);
        event::emit(CollateralRegistered {
            symbol: clone_symbol(&emitted_symbol),
            asset_type: ASSET_TYPE_CROSSCHAIN,
            beta_e9,
            ltv_bps,
        });
        emit_crosschain_config_event(emitted_symbol, &validated_config);
    }

    /// Update the Walrus blob anchor for a collateral type.
    public fun update_walrus_anchor(
        registry: &mut CollateralRegistry,
        symbol: &vector<u8>,
        walrus_anchor: WalrusBlobRef,
        current_epoch: u64,
    ) {
        assert!(table::contains(&registry.entries, clone_symbol(symbol)), E_UNKNOWN_COLLATERAL);
        let metadata = table::borrow_mut(&mut registry.entries, clone_symbol(symbol));
        assert!(metadata.walrus_required, E_WALRUS_REQUIRED);
        validate_walrus_anchor(&walrus_anchor, current_epoch);
        metadata.walrus_anchor = option::some(walrus_anchor);
        metadata.last_anchor_epoch = current_epoch;

        let anchor_ref = option::borrow(&metadata.walrus_anchor);
        event::emit(WalrusAnchorUpdated {
            symbol: clone_symbol(symbol),
            blob_object_id: anchor_ref.blob_object_id,
            expires_at_epoch: anchor_ref.expires_at_epoch,
        });
    }

    /// Toggle whether a collateral type can be used.
    public fun set_collateral_active(
        registry: &mut CollateralRegistry,
        symbol: &vector<u8>,
        is_active: bool,
    ) {
        assert!(table::contains(&registry.entries, clone_symbol(symbol)), E_UNKNOWN_COLLATERAL);
        let metadata = table::borrow_mut(&mut registry.entries, clone_symbol(symbol));
        metadata.is_active = is_active;
        event::emit(CollateralStatusChanged { symbol: clone_symbol(symbol), is_active });
    }

    /// Configure or update the risk params for a cross-chain collateral.
    public fun set_crosschain_risk_config(
        registry: &mut CollateralRegistry,
        symbol: &vector<u8>,
        risk_config: CrosschainRiskConfig,
    ) {
        assert!(table::contains(&registry.entries, clone_symbol(symbol)), E_UNKNOWN_COLLATERAL);
        let metadata = table::borrow_mut(&mut registry.entries, clone_symbol(symbol));
        assert!(metadata.asset_type == ASSET_TYPE_CROSSCHAIN, E_PARAM_INVALID);
        let validated = validate_crosschain_risk_config(risk_config);
        metadata.ltv_bps = ratio_to_bps(validated.ltv_ratio_e9);
        metadata.crosschain_config = option::some(validated);
        let config_ref = option::borrow(&metadata.crosschain_config);
        emit_crosschain_config_event(clone_symbol(symbol), config_ref);
    }

    /// Borrow immutable metadata for inspection.
    public fun get_metadata(
        registry: &CollateralRegistry,
        symbol: &vector<u8>,
    ): &CollateralMetadata {
        assert!(table::contains(&registry.entries, clone_symbol(symbol)), E_UNKNOWN_COLLATERAL);
        table::borrow(&registry.entries, clone_symbol(symbol))
    }

    /// Borrow mutable metadata (admin use).
    public fun get_metadata_mut(
        registry: &mut CollateralRegistry,
        symbol: &vector<u8>,
    ): &mut CollateralMetadata {
        assert!(table::contains(&registry.entries, clone_symbol(symbol)), E_UNKNOWN_COLLATERAL);
        table::borrow_mut(&mut registry.entries, clone_symbol(symbol))
    }

    /// View helper to fetch cross-chain risk parameters for API surfaces.
    public fun view_crosschain_params(
        registry: &CollateralRegistry,
        symbol: &vector<u8>,
    ): CrosschainRiskConfig {
        let metadata_ref = get_metadata(registry, symbol);
        assert!(metadata_ref.asset_type == ASSET_TYPE_CROSSCHAIN, E_PARAM_INVALID);
        *option::borrow(&metadata_ref.crosschain_config)
    }

    /// View helper to obtain the Walrus anchor associated with a collateral symbol.
    public fun view_walrus_anchor(
        registry: &CollateralRegistry,
        symbol: &vector<u8>,
    ): WalrusBlobRef {
        let metadata_ref = get_metadata(registry, symbol);
        assert!(option::is_some(&metadata_ref.walrus_anchor), E_WALRUS_REQUIRED);
        *option::borrow(&metadata_ref.walrus_anchor)
    }

    /// Helper to ensure cross-chain collateral is active with a valid Walrus proof.
    public fun assert_crosschain_ready(
        registry: &CollateralRegistry,
        symbol: &vector<u8>,
        current_epoch: u64,
    ): &CollateralMetadata {
        assert!(table::contains(&registry.entries, clone_symbol(symbol)), E_UNKNOWN_COLLATERAL);
        let metadata = table::borrow(&registry.entries, clone_symbol(symbol));
        assert!(metadata.is_active, E_COLLATERAL_INACTIVE);

        if (metadata.walrus_required) {
            assert!(option::is_some(&metadata.walrus_anchor), E_WALRUS_REQUIRED);
            let anchor_ref = option::borrow(&metadata.walrus_anchor);
            assert!(current_epoch <= anchor_ref.expires_at_epoch, E_WALRUS_EXPIRED);
        };

        assert!(metadata.asset_type == ASSET_TYPE_CROSSCHAIN, E_PARAM_INVALID);
        assert!(option::is_some(&metadata.crosschain_config), E_PARAM_INVALID);
        metadata
    }

    /// Retrieve the registry object ID (useful for binding other structs).
    public fun registry_id(registry: &CollateralRegistry): ID {
        object::id(registry)
    }

    /// Expose the asset type identifiers without leaking the private constants.
    public fun asset_type_native(): u8 { ASSET_TYPE_NATIVE }
    public fun asset_type_crosschain(): u8 { ASSET_TYPE_CROSSCHAIN }

    /// Accessors for other modules that need metadata fields.
    public fun metadata_beta(metadata: &CollateralMetadata): u64 { metadata.beta_e9 }

    public fun metadata_asset_type(metadata: &CollateralMetadata): u8 { metadata.asset_type }

    public fun metadata_walrus_anchor(metadata: &CollateralMetadata): &WalrusBlobRef {
        assert!(option::is_some(&metadata.walrus_anchor), E_WALRUS_REQUIRED);
        option::borrow(&metadata.walrus_anchor)
    }

    public fun metadata_chain_tag(metadata: &CollateralMetadata): &vector<u8> {
        // For cross-chain entries, chain_tag must be present
        option::borrow(&metadata.chain_tag)
    }

    public fun metadata_crosschain_config(metadata: &CollateralMetadata): &CrosschainRiskConfig {
        option::borrow(&metadata.crosschain_config)
    }

    public fun risk_ltv_ratio(config: &CrosschainRiskConfig): u64 { config.ltv_ratio_e9 }

    public fun risk_oracle_haircut(config: &CrosschainRiskConfig): u64 { config.oracle_haircut_e9 }

    public fun risk_mint_limit(config: &CrosschainRiskConfig): u128 { config.mint_limit_value_e9 }

    public fun risk_withdraw_limit(config: &CrosschainRiskConfig): u128 { config.withdraw_limit_value_e9 }

    public fun risk_staleness_cap(config: &CrosschainRiskConfig): u64 { config.staleness_cap_ms }

    public fun walrus_blob_id(anchor: &WalrusBlobRef): &vector<u8> { &anchor.blob_id }

    /// Internal helper that validates Walrus metadata.
    fun validate_walrus_anchor(anchor: &WalrusBlobRef, current_epoch: u64) {
        assert!(anchor.non_deletable, E_WALRUS_REQUIRED);
        assert!(current_epoch <= anchor.expires_at_epoch, E_WALRUS_EXPIRED);
    }

    #[test_only]
    public fun test_walrus_anchor(
        blob_object_id: address,
        writer: address,
        expires_at_epoch: u64,
    ): WalrusBlobRef {
        WalrusBlobRef {
            blob_object_id,
            blob_id: vector::empty<u8>(),
            content_hash: vector::empty<u8>(),
            writer,
            non_deletable: true,
            expires_at_epoch,
        }
    }

    #[test_only]
    public fun test_walrus_anchor_with_blob(
        blob_object_id: address,
        writer: address,
        blob_id: vector<u8>,
        content_hash: vector<u8>,
        expires_at_epoch: u64,
    ): WalrusBlobRef {
        WalrusBlobRef {
            blob_object_id,
            blob_id,
            content_hash,
            writer,
            non_deletable: true,
            expires_at_epoch,
        }
    }

    public fun new_crosschain_risk_config(
        ltv_ratio_e9: u64,
        maintenance_ratio_e9: u64,
        liquidation_penalty_e9: u64,
        oracle_haircut_e9: u64,
        staleness_cap_ms: u64,
        mint_limit_value_e9: u128,
        withdraw_limit_value_e9: u128,
    ): CrosschainRiskConfig {
        let config = CrosschainRiskConfig {
            ltv_ratio_e9,
            maintenance_ratio_e9,
            liquidation_penalty_e9,
            oracle_haircut_e9,
            staleness_cap_ms,
            mint_limit_value_e9,
            withdraw_limit_value_e9,
        };
        validate_crosschain_risk_config(config)
    }

    fun validate_crosschain_risk_config(config: CrosschainRiskConfig): CrosschainRiskConfig {
        assert!(config.ltv_ratio_e9 > 0 && config.ltv_ratio_e9 <= SCALE_FACTOR, E_PARAM_INVALID);
        assert!(config.maintenance_ratio_e9 >= config.ltv_ratio_e9, E_PARAM_INVALID);
        assert!(config.maintenance_ratio_e9 <= SCALE_FACTOR, E_PARAM_INVALID);
        assert!(config.liquidation_penalty_e9 <= SCALE_FACTOR, E_PARAM_INVALID);
        assert!(config.oracle_haircut_e9 < SCALE_FACTOR, E_PARAM_INVALID);
        assert!(config.staleness_cap_ms > 0, E_PARAM_INVALID);
        config
    }

    fun emit_crosschain_config_event(symbol: vector<u8>, config: &CrosschainRiskConfig) {
        event::emit(CrosschainRiskParamsUpdated {
            symbol,
            ltv_ratio_e9: config.ltv_ratio_e9,
            maintenance_ratio_e9: config.maintenance_ratio_e9,
            liquidation_penalty_e9: config.liquidation_penalty_e9,
            oracle_haircut_e9: config.oracle_haircut_e9,
            staleness_cap_ms: config.staleness_cap_ms,
            mint_limit_value_e9: config.mint_limit_value_e9,
            withdraw_limit_value_e9: config.withdraw_limit_value_e9,
        });
    }

    fun ratio_to_bps(ratio_e9: u64): u64 {
        (ratio_e9 * MAX_LTV_BPS) / SCALE_FACTOR
    }

    fun clone_symbol(symbol: &vector<u8>): vector<u8> {
        let mut out = vector::empty<u8>();
        let len = vector::length(symbol);
        let mut i = 0;
        while (i < len) {
            let byte_ref = vector::borrow(symbol, i);
            vector::push_back(&mut out, *byte_ref);
            i = i + 1;
        };
        out
    }
}
