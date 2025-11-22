/// Cross-chain collateral support (e.g., ETH on Ethereum) backed by Walrus checkpoints.
/// Instead of holding the underlying asset on Sui, the module tracks off-chain shares
/// and index data published via Walrus and emits vouchers on redeem that can be
/// fulfilled on the origin chain.
#[allow(duplicate_alias)]
module leafsii::crosschain_vault {
    use std::option;
    use std::vector;

    use sui::clock::{Clock, timestamp_ms};
    use sui::coin::{Self as coin, Coin, TreasuryCap};
    use sui::event;
    use sui::object;
    use sui::object::{ID, UID};
    use sui::table::{Self as table, Table};
    use sui::transfer;
    use sui::tx_context;

    use math::math;
    use leafsii::collateral_registry::{Self as registry, CollateralMetadata, CollateralRegistry, WalrusBlobRef};

    const PF_FIXED: u64 = 1_000_000_000; // $1.00 in 1e9 scale
    const SCALE_FACTOR: u64 = 1_000_000_000;
    const E_INVALID_AMOUNT: u64 = 1;
    const E_INVALID_PRICE: u64 = 2;
    const E_REGISTRY_MISMATCH: u64 = 3;
    const E_UNAUTHORIZED: u64 = 4;
    const E_OVERFLOW: u64 = 5;
    const E_STALE_CHECKPOINT: u64 = 6;
    const E_WRONG_COLLATERAL_TYPE: u64 = 7;
    const E_SHARES_OVERUSE: u64 = 8;
    const E_INVALID_PROOF: u64 = 9;
    const E_RATE_LIMIT: u64 = 10;
    const E_VOUCHER_STATE: u64 = 11;
    const E_VOUCHER_EXPIRED: u64 = 12;
    const E_UNAUTHORIZED_VOUCHER: u64 = 13;
    const E_LTV_EXCEEDED: u64 = 14;

    const VOUCHER_STATE_PENDING: u8 = 0;
    const VOUCHER_STATE_SPENT: u8 = 1;
    const VOUCHER_STATE_SETTLED: u8 = 2;

    /// Proof of a Walrus checkpoint verification.
    public struct CheckpointProof has copy, drop, store {
        update_id: u64,
        index_e9: u64,
        block_number: u64,
        block_hash: vector<u8>,
        balances_root: vector<u8>,
        walrus_blob_id: vector<u8>,
        source_timestamp_ms: u64,
        proof_blob: vector<u8>,
    }

    /// Stored checkpoint details for staleness gates and user proofs.
    public struct CheckpointState has copy, drop, store {
        update_id: u64,
        index_e9: u64,
        block_number: u64,
        block_hash: vector<u8>,
        balances_root: vector<u8>,
        walrus_blob_id: vector<u8>,
        source_timestamp_ms: u64,
        verified_ms: u64,
    }

    /// Proof of a user's attested vault shares.
    public struct BalanceProof has copy, drop, store {
        owner: address,
        update_id: u64,
        balances_root: vector<u8>,
        total_shares: u128,
        proof_blob: vector<u8>,
    }

    /// View struct exposing the latest verified checkpoint for off-chain monitors.
    public struct CheckpointView has copy, drop, store {
        update_id: u64,
        index_e9: u64,
        block_number: u64,
        block_hash: vector<u8>,
        balances_root: vector<u8>,
        walrus_blob_id: vector<u8>,
        source_timestamp_ms: u64,
        verified_ms: u64,
    }

    /// View struct exposing current rate limiter consumption.
    public struct RateLimitUsage has copy, drop {
        mint_epoch: u64,
        mint_value_e9: u128,
        redeem_epoch: u64,
        redeem_value_e9: u128,
    }

    /// A cross-chain series representing f/x tokens backed by off-chain vault shares.
    public struct CrossChainSeries<phantom StableToken, phantom LeverageToken> has key, store {
        id: UID,
        registry_id: ID,
        // Human-ish tags to bind origin collateral
        collateral_symbol: vector<u8>,   // e.g., b"ETH"
        chain_tag: vector<u8>,           // e.g., b"ethereum"

        // Minting treasuries
        stable_treasury_cap: TreasuryCap<StableToken>,
        leverage_treasury_cap: TreasuryCap<LeverageToken>,

        // Pricing and split
        pf: u64,
        px: u64,
        beta_e9: u64,

        // Reserve accounting (USD, 1e9)
        reserve_value_e9: u128,
        stable_supply: u64,
        leverage_supply: u64,

        // Latest Walrus checkpoint summary
        checkpoint: option::Option<CheckpointState>,
        walrus_anchor: WalrusBlobRef,

        // Rate-limit accounting (USD, 1e9 scale)
        mint_rate_epoch: u64,
        mint_value_e9: u128,
        redeem_rate_epoch: u64,
        redeem_value_e9: u128,

        // Voucher nonce management
        next_voucher_nonce: u64,

        // Shares usage ledger: address -> used_shares (attested and consumed for minting)
        used_shares: Table<address, u128>,
    }

    /// Voucher emitted on redeem; redeemable on origin chain by owner.
    public struct WithdrawalVoucher has key, store {
        id: UID,
        series_id: ID,
        owner: address,
        collateral_symbol: vector<u8>,
        chain_tag: vector<u8>,
        shares: u128,
        update_id: u64,
        created_ms: u64,
        expiry_ms: u64,
        nonce: u64,
        state: u8,
        user_signature: option::Option<vector<u8>>,
        spent_reference: option::Option<vector<u8>>,
        settled_reference: option::Option<vector<u8>>,
        spent_ms: u64,
        settled_ms: u64,
    }

    public struct CrossChainSeriesCreated has copy, drop {
        series_id: ID,
        symbol: vector<u8>,
        chain_tag: vector<u8>,
        beta_e9: u64,
    }

    public struct CheckpointUpdated has copy, drop {
        series_id: ID,
        update_id: u64,
        index_e9: u64,
        block_number: u64,
        balances_root: vector<u8>,
        walrus_blob_id: vector<u8>,
    }

    public struct CrossChainMint has copy, drop {
        series_id: ID,
        minter: address,
        shares_used: u128,
        price_e9: u64,
        px_e9: u64,
        index_e9: u64,
        update_id: u64,
        f_minted: u64,
        x_minted: u64,
    }

    public struct VoucherCreated has copy, drop {
        series_id: ID,
        owner: address,
        shares: u128,
        update_id: u64,
        nonce: u64,
        expiry_ms: u64,
    }

    public struct VoucherSpent has copy, drop {
        series_id: ID,
        voucher_id: ID,
        owner: address,
        nonce: u64,
    }

    public struct VoucherSettled has copy, drop {
        series_id: ID,
        voucher_id: ID,
        owner: address,
        nonce: u64,
    }

    /// Initialize a cross-chain series (e.g., fETH/xETH) bound to a registry entry.
    public fun init_crosschain_series<StableToken, LeverageToken>(
        registry: &CollateralRegistry,
        symbol: vector<u8>,            // e.g., b"ETH"
        stable_treasury_cap: TreasuryCap<StableToken>,
        leverage_treasury_cap: TreasuryCap<LeverageToken>,
        ctx: &mut tx_context::TxContext
    ): CrossChainSeries<StableToken, LeverageToken> {
        let metadata = registry::get_metadata(registry, &symbol);
        assert!(registry::metadata_asset_type(metadata) == registry::asset_type_crosschain(), E_WRONG_COLLATERAL_TYPE);
        let anchor_ref = registry::metadata_walrus_anchor(metadata);
        let chain_tag_ref = registry::metadata_chain_tag(metadata);

        let series = CrossChainSeries {
            id: object::new(ctx),
            registry_id: registry::registry_id(registry),
            collateral_symbol: clone_vec(&symbol),
            chain_tag: clone_vec(chain_tag_ref),
            stable_treasury_cap,
            leverage_treasury_cap,
            pf: PF_FIXED,
            px: PF_FIXED,
            beta_e9: registry::metadata_beta(metadata),
            reserve_value_e9: 0,
            stable_supply: 0,
            leverage_supply: 0,
            checkpoint: option::none<CheckpointState>(),
            walrus_anchor: *anchor_ref,
            mint_rate_epoch: 0,
            mint_value_e9: 0,
            redeem_rate_epoch: 0,
            redeem_value_e9: 0,
            next_voucher_nonce: 0,
            used_shares: table::new<address, u128>(ctx),
        };

        event::emit(CrossChainSeriesCreated {
            series_id: object::id(&series),
            symbol: clone_vec(&series.collateral_symbol),
            chain_tag: clone_vec(&series.chain_tag),
            beta_e9: series.beta_e9,
        });

        series
    }

    /// Update the latest verified Walrus checkpoint metadata.
    public fun update_checkpoint<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        proof: CheckpointProof,
        current_epoch: u64,
        clock: &Clock,
    ) {
        ensure_registry_match(series, registry);
        let symbol_key = clone_vec(&series.collateral_symbol);
        let metadata = registry::assert_crosschain_ready(registry, &symbol_key, current_epoch);
        sync_params(series, metadata);
        assert!(proof.index_e9 > 0, E_INVALID_AMOUNT);
        verify_checkpoint_proof(&proof, &series.walrus_anchor);

        if (option::is_some(&series.checkpoint)) {
            let existing = option::borrow(&series.checkpoint);
            assert!(proof.update_id > existing.update_id, E_STALE_CHECKPOINT);
        };

        let new_state = CheckpointState {
            update_id: proof.update_id,
            index_e9: proof.index_e9,
            block_number: proof.block_number,
            block_hash: proof.block_hash,
            balances_root: proof.balances_root,
            walrus_blob_id: proof.walrus_blob_id,
            source_timestamp_ms: proof.source_timestamp_ms,
            verified_ms: timestamp_ms(clock),
        };
        series.checkpoint = option::some(new_state);

        let checkpoint_ref = option::borrow(&series.checkpoint);
        event::emit(CheckpointUpdated {
            series_id: object::id(series),
            update_id: checkpoint_ref.update_id,
            index_e9: checkpoint_ref.index_e9,
            block_number: checkpoint_ref.block_number,
            balances_root: clone_vec(&checkpoint_ref.balances_root),
            walrus_blob_id: clone_vec(&checkpoint_ref.walrus_blob_id),
        });
    }

    /// Mint f/x tokens by consuming an attested amount of new shares.
    public fun mint_from_attested_shares<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        balance_proof: BalanceProof,
        oracle_price_e9: u64,
        xtoken_price_e9: u64,
        current_epoch: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ): (Coin<StableToken>, Coin<LeverageToken>) {
        let recipient = tx_context::sender(ctx);
        mint_from_attested_shares_core(
            series,
            registry,
            balance_proof,
            oracle_price_e9,
            xtoken_price_e9,
            current_epoch,
            clock,
            ctx,
            recipient,
            true,
        )
    }

    /// Mint f/x tokens to an attested owner while a bridge worker pays gas.
    /// The minted coins are still attributed to the attested owner, but the
    /// caller does not need to match that owner.
    public fun bridge_mint<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        balance_proof: BalanceProof,
        xtoken_price_e9: u64,
        current_epoch: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ): (Coin<StableToken>, Coin<LeverageToken>) {
        let recipient = balance_proof.owner;
        // Until a Walrus-provided price feed is wired in, bridge minting uses a
        // fixed $1.00 price anchor.
        let bridge_price_e9 = PF_FIXED;
        mint_from_attested_shares_core(
            series,
            registry,
            balance_proof,
            bridge_price_e9,
            xtoken_price_e9,
            current_epoch,
            clock,
            ctx,
            recipient,
            false,
        )
    }

    /// Shared minting logic used by both user-triggered and bridge-triggered flows.
    fun mint_from_attested_shares_core<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        balance_proof: BalanceProof,
        oracle_price_e9: u64,
        xtoken_price_e9: u64,
        current_epoch: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext,
        recipient: address,
        enforce_sender_is_owner: bool,
    ): (Coin<StableToken>, Coin<LeverageToken>) {
        assert!(oracle_price_e9 > 0, E_INVALID_PRICE);
        assert!(xtoken_price_e9 > 0, E_INVALID_PRICE);
        ensure_registry_match(series, registry);
        let symbol_key = clone_vec(&series.collateral_symbol);
        let metadata = registry::assert_crosschain_ready(registry, &symbol_key, current_epoch);
        sync_params(series, metadata);
        let config = registry::metadata_crosschain_config(metadata);
        let checkpoint = current_checkpoint(series);
        enforce_checkpoint_staleness(&checkpoint, config, clock);

        let BalanceProof { owner, update_id: proof_update_id, balances_root, total_shares, proof_blob } = balance_proof;
        if (enforce_sender_is_owner) {
            assert!(owner == tx_context::sender(ctx), E_UNAUTHORIZED);
        };
        assert!(owner == recipient, E_UNAUTHORIZED);
        assert!(total_shares > 0, E_INVALID_AMOUNT);
        assert!(proof_update_id == checkpoint.update_id, E_INVALID_PROOF);
        verify_balance_proof(&balances_root, &proof_blob, &checkpoint);

        let used = if (table::contains(&series.used_shares, owner)) {
            *table::borrow(&series.used_shares, owner)
        } else { 0u128 };
        assert!(total_shares >= used, E_SHARES_OVERUSE);
        let new_shares = sub_u128(total_shares, used, E_SHARES_OVERUSE);
        assert!(new_shares > 0, E_INVALID_AMOUNT);

        let safe_price = apply_oracle_haircut(oracle_price_e9, registry::risk_oracle_haircut(config));
        assert!(safe_price > 0, E_INVALID_PRICE);
        let value_e9 = mul3_to_u128(new_shares, (checkpoint.index_e9 as u128), (safe_price as u128));
        enforce_rate_limit(&mut series.mint_rate_epoch, &mut series.mint_value_e9, current_epoch, value_e9, registry::risk_mint_limit(config));

        let f_value = value_e9 / 2;
        let x_value = sub_u128(value_e9, f_value, E_OVERFLOW);
        let ltv_cap = math::mul_div_u128(value_e9, (registry::risk_ltv_ratio(config) as u128), (SCALE_FACTOR as u128));
        assert!(f_value <= ltv_cap, E_LTV_EXCEEDED);

        let f_mint = if (f_value > 0) { math::div_back_u64(f_value, series.pf) } else { 0 };
        let x_mint = if (x_value > 0) { math::div_back_u64(x_value, xtoken_price_e9) } else { 0 };

        series.px = xtoken_price_e9;
        series.reserve_value_e9 = add_u128(series.reserve_value_e9, value_e9);
        series.stable_supply = series.stable_supply + f_mint;
        series.leverage_supply = series.leverage_supply + x_mint;

        if (table::contains(&series.used_shares, owner)) {
            let used_mut = table::borrow_mut(&mut series.used_shares, owner);
            *used_mut = total_shares;
        } else {
            table::add(&mut series.used_shares, owner, total_shares);
        };

        let f_coin = if (f_mint > 0) { coin::mint(&mut series.stable_treasury_cap, f_mint, ctx) } else { coin::zero<StableToken>(ctx) };
        let x_coin = if (x_mint > 0) { coin::mint(&mut series.leverage_treasury_cap, x_mint, ctx) } else { coin::zero<LeverageToken>(ctx) };

        event::emit(CrossChainMint {
            series_id: object::id(series),
            minter: owner,
            shares_used: new_shares,
            price_e9: safe_price,
            px_e9: xtoken_price_e9,
            index_e9: checkpoint.index_e9,
            update_id: checkpoint.update_id,
            f_minted: f_mint,
            x_minted: x_mint,
        });

        (f_coin, x_coin)
    }

    /// Redeem fTokens into a withdrawal voucher representing origin-chain shares.
    public fun redeem_f<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        f_tokens: Coin<StableToken>,
        expiry_ms: u64,
        user_signature: option::Option<vector<u8>>,
        current_epoch: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ): WithdrawalVoucher {
        let burn_amount = coin::value(&f_tokens);
        assert!(burn_amount > 0, E_INVALID_AMOUNT);
        let usd_value = math::mul_to_u128(burn_amount, series.pf);
        assert!(usd_value <= series.reserve_value_e9, E_INVALID_AMOUNT);

        ensure_registry_match(series, registry);
        let symbol_key = clone_vec(&series.collateral_symbol);
        let metadata = registry::assert_crosschain_ready(registry, &symbol_key, current_epoch);
        let config = registry::metadata_crosschain_config(metadata);
        let checkpoint = current_checkpoint(series);
        enforce_checkpoint_staleness(&checkpoint, config, clock);

        // Convert USD value back to shares at current index and implicit price=1 (shares already priced)
        let denom = mul_u128(checkpoint.index_e9 as u128, 1u128);
        let shares_out = div_back_u128(usd_value, denom);

        series.reserve_value_e9 = sub_u128(series.reserve_value_e9, usd_value, E_INVALID_AMOUNT);
        series.stable_supply = series.stable_supply - burn_amount;
        coin::burn(&mut series.stable_treasury_cap, f_tokens);
        enforce_rate_limit(&mut series.redeem_rate_epoch, &mut series.redeem_value_e9, current_epoch, usd_value, registry::risk_withdraw_limit(config));

        create_voucher(series, shares_out, expiry_ms, user_signature, clock, ctx)
    }

    /// Entry wrapper for Walrus checkpoint updates to be invoked directly by monitors.
    public entry fun entry_update_checkpoint<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        update_id: u64,
        index_e9: u64,
        block_number: u64,
        block_hash: vector<u8>,
        balances_root: vector<u8>,
        walrus_blob_id: vector<u8>,
        source_timestamp_ms: u64,
        proof_blob: vector<u8>,
        current_epoch: u64,
        clock: &Clock,
        _ctx: &mut tx_context::TxContext,
    ) {
        let proof = CheckpointProof {
            update_id,
            index_e9,
            block_number,
            block_hash,
            balances_root,
            walrus_blob_id,
            source_timestamp_ms,
            proof_blob,
        };
        update_checkpoint(series, registry, proof, current_epoch, clock);
    }

    /// Entry wrapper that validates an attested balance and returns freshly minted f/x coins.
    public entry fun entry_mint_from_attested_shares<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        owner: address,
        update_id: u64,
        balances_root: vector<u8>,
        total_shares: u128,
        proof_blob: vector<u8>,
        oracle_price_e9: u64,
        xtoken_price_e9: u64,
        current_epoch: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ) {
        let balance_proof = BalanceProof { owner, update_id, balances_root, total_shares, proof_blob };
        let (f_coin, x_coin) = mint_from_attested_shares(series, registry, balance_proof, oracle_price_e9, xtoken_price_e9, current_epoch, clock, ctx);
        transfer::public_transfer(f_coin, tx_context::sender(ctx));
        transfer::public_transfer(x_coin, tx_context::sender(ctx));
    }

    /// Entry wrapper that allows a bridge worker to mint on behalf of a user and
    /// send the f/x tokens directly to the attested owner on Sui.
    public entry fun entry_bridge_mint<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        owner: address,
        update_id: u64,
        balances_root: vector<u8>,
        total_shares: u128,
        proof_blob: vector<u8>,
        xtoken_price_e9: u64,
        current_epoch: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ) {
        let balance_proof = BalanceProof { owner, update_id, balances_root, total_shares, proof_blob };
        let (f_coin, x_coin) = bridge_mint(series, registry, balance_proof, xtoken_price_e9, current_epoch, clock, ctx);
        transfer::public_transfer(f_coin, owner);
        transfer::public_transfer(x_coin, owner);
    }

    /// Entry wrapper for fToken redemption into a withdrawal voucher.
    public entry fun entry_redeem_f<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        f_tokens: Coin<StableToken>,
        expiry_ms: u64,
        has_user_signature: bool,
        user_signature: vector<u8>,
        current_epoch: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ) {
        let signature_opt = if (has_user_signature) {
            option::some(user_signature)
        } else {
            option::none<vector<u8>>()
        };
        let voucher = redeem_f(series, registry, f_tokens, expiry_ms, signature_opt, current_epoch, clock, ctx);
        transfer::public_transfer(voucher, tx_context::sender(ctx));
    }

    /// Entry wrapper for xToken redemption into a withdrawal voucher.
    public entry fun entry_redeem_x<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        x_tokens: Coin<LeverageToken>,
        expiry_ms: u64,
        has_user_signature: bool,
        user_signature: vector<u8>,
        current_epoch: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ) {
        let signature_opt = if (has_user_signature) {
            option::some(user_signature)
        } else {
            option::none<vector<u8>>()
        };
        let voucher = redeem_x(series, registry, x_tokens, expiry_ms, signature_opt, current_epoch, clock, ctx);
        transfer::public_transfer(voucher, tx_context::sender(ctx));
    }

    /// Redeem xTokens into a withdrawal voucher representing origin-chain shares.
    public fun redeem_x<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
        x_tokens: Coin<LeverageToken>,
        expiry_ms: u64,
        user_signature: option::Option<vector<u8>>,
        current_epoch: u64,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ): WithdrawalVoucher {
        let burn_amount = coin::value(&x_tokens);
        assert!(burn_amount > 0, E_INVALID_AMOUNT);
        assert!(series.px > 0, E_INVALID_PRICE);
        let usd_value = math::mul_to_u128(burn_amount, series.px);
        assert!(usd_value <= series.reserve_value_e9, E_INVALID_AMOUNT);
        ensure_registry_match(series, registry);
        let symbol_key = clone_vec(&series.collateral_symbol);
        let metadata = registry::assert_crosschain_ready(registry, &symbol_key, current_epoch);
        let config = registry::metadata_crosschain_config(metadata);
        let checkpoint = current_checkpoint(series);
        enforce_checkpoint_staleness(&checkpoint, config, clock);

        let denom = mul_u128(checkpoint.index_e9 as u128, 1u128);
        let shares_out = div_back_u128(usd_value, denom);

        series.reserve_value_e9 = sub_u128(series.reserve_value_e9, usd_value, E_INVALID_AMOUNT);
        series.leverage_supply = series.leverage_supply - burn_amount;
        coin::burn(&mut series.leverage_treasury_cap, x_tokens);
        enforce_rate_limit(&mut series.redeem_rate_epoch, &mut series.redeem_value_e9, current_epoch, usd_value, registry::risk_withdraw_limit(config));

        create_voucher(series, shares_out, expiry_ms, user_signature, clock, ctx)
    }

    /// Internal: create and emit a voucher object.
    fun create_voucher<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        shares: u128,
        expiry_ms: u64,
        user_signature: option::Option<vector<u8>>,
        clock: &Clock,
        ctx: &mut tx_context::TxContext
    ): WithdrawalVoucher {
        let now = timestamp_ms(clock);
        assert!(expiry_ms > now, E_VOUCHER_EXPIRED);
        let checkpoint = current_checkpoint(series);
        let owner = tx_context::sender(ctx);
        let nonce = take_and_increment_nonce(series);
        let voucher = WithdrawalVoucher {
            id: object::new(ctx),
            series_id: object::id(series),
            owner,
            collateral_symbol: clone_vec(&series.collateral_symbol),
            chain_tag: clone_vec(&series.chain_tag),
            shares,
            update_id: checkpoint.update_id,
            created_ms: now,
            expiry_ms,
            nonce,
            state: VOUCHER_STATE_PENDING,
            user_signature,
            spent_reference: option::none<vector<u8>>(),
            settled_reference: option::none<vector<u8>>(),
            spent_ms: 0,
            settled_ms: 0,
        };

        event::emit(VoucherCreated {
            series_id: object::id(series),
            owner,
            shares,
            update_id: checkpoint.update_id,
            nonce,
            expiry_ms,
        });

        voucher
    }

    /// Mark a voucher as spent once the withdrawal transaction has been initiated on the origin chain.
    public fun mark_voucher_spent(
        voucher: &mut WithdrawalVoucher,
        fulfillment_reference: option::Option<vector<u8>>,
        clock: &Clock,
        ctx: &mut tx_context::TxContext,
    ) {
        assert!(tx_context::sender(ctx) == voucher.owner, E_UNAUTHORIZED_VOUCHER);
        assert!(voucher.state == VOUCHER_STATE_PENDING, E_VOUCHER_STATE);
        voucher.state = VOUCHER_STATE_SPENT;
        voucher.spent_reference = fulfillment_reference;
        voucher.spent_ms = timestamp_ms(clock);
        event::emit(VoucherSpent {
            series_id: voucher.series_id,
            voucher_id: object::id(voucher),
            owner: voucher.owner,
            nonce: voucher.nonce,
        });
    }

    /// Mark a voucher as fully settled once the user confirms receipt on the origin chain.
    public fun mark_voucher_settled(
        voucher: &mut WithdrawalVoucher,
        settlement_reference: option::Option<vector<u8>>,
        clock: &Clock,
        ctx: &mut tx_context::TxContext,
    ) {
        assert!(tx_context::sender(ctx) == voucher.owner, E_UNAUTHORIZED_VOUCHER);
        assert!(voucher.state == VOUCHER_STATE_SPENT, E_VOUCHER_STATE);
        voucher.state = VOUCHER_STATE_SETTLED;
        voucher.settled_reference = settlement_reference;
        voucher.settled_ms = timestamp_ms(clock);
        event::emit(VoucherSettled {
            series_id: voucher.series_id,
            voucher_id: object::id(voucher),
            owner: voucher.owner,
            nonce: voucher.nonce,
        });
    }

    /// Entry wrapper to mark a voucher as spent with an optional fulfillment reference (e.g., tx hash).
    public entry fun entry_mark_voucher_spent(
        voucher: &mut WithdrawalVoucher,
        has_reference: bool,
        fulfillment_reference: vector<u8>,
        clock: &Clock,
        ctx: &mut tx_context::TxContext,
    ) {
        let ref_opt = if (has_reference) { option::some(fulfillment_reference) } else { option::none<vector<u8>>() };
        mark_voucher_spent(voucher, ref_opt, clock, ctx);
    }

    /// Entry wrapper to mark a voucher as fully settled once the origin-chain withdrawal clears.
    public entry fun entry_mark_voucher_settled(
        voucher: &mut WithdrawalVoucher,
        has_reference: bool,
        settlement_reference: vector<u8>,
        clock: &Clock,
        ctx: &mut tx_context::TxContext,
    ) {
        let ref_opt = if (has_reference) { option::some(settlement_reference) } else { option::none<vector<u8>>() };
        mark_voucher_settled(voucher, ref_opt, clock, ctx);
    }

    /// Keep β and Walrus anchor in sync with registry updates.
    fun sync_params<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>,
        metadata: &CollateralMetadata,
    ) {
        series.beta_e9 = registry::metadata_beta(metadata);
        let anchor_ref = registry::metadata_walrus_anchor(metadata);
        series.walrus_anchor = *anchor_ref;
    }

    fun current_checkpoint<StableToken, LeverageToken>(
        series: &CrossChainSeries<StableToken, LeverageToken>
    ): CheckpointState {
        assert!(option::is_some(&series.checkpoint), E_STALE_CHECKPOINT);
        *option::borrow(&series.checkpoint)
    }

    fun verify_checkpoint_proof(proof: &CheckpointProof, anchor: &WalrusBlobRef) {
        assert!(vector::length(&proof.balances_root) > 0, E_INVALID_PROOF);
        assert!(vector::length(&proof.block_hash) > 0, E_INVALID_PROOF);
        assert!(vector::length(&proof.walrus_blob_id) > 0, E_INVALID_PROOF);
        assert!(vector::length(&proof.proof_blob) > 0, E_INVALID_PROOF);
        assert!(compare_vecs(&proof.walrus_blob_id, registry::walrus_blob_id(anchor)), E_INVALID_PROOF);
    }

    fun verify_balance_proof(
        balances_root: &vector<u8>,
        proof_blob: &vector<u8>,
        checkpoint: &CheckpointState,
    ) {
        assert!(vector::length(balances_root) > 0, E_INVALID_PROOF);
        assert!(vector::length(proof_blob) > 0, E_INVALID_PROOF);
        assert!(compare_vecs(balances_root, &checkpoint.balances_root), E_INVALID_PROOF);
    }

    fun enforce_checkpoint_staleness(
        checkpoint: &CheckpointState,
        config: &registry::CrosschainRiskConfig,
        clock: &Clock,
    ) {
        let now = timestamp_ms(clock);
        assert!(now >= checkpoint.verified_ms, E_STALE_CHECKPOINT);
        let age = now - checkpoint.verified_ms;
        assert!(age <= registry::risk_staleness_cap(config), E_STALE_CHECKPOINT);
    }

    fun apply_oracle_haircut(price_e9: u64, haircut_e9: u64): u64 {
        if (haircut_e9 == 0) {
            return price_e9
        };
        let adjusted = math::mul_div_u128(
            (price_e9 as u128),
            ((SCALE_FACTOR - haircut_e9) as u128),
            (SCALE_FACTOR as u128)
        );
        adjusted as u64
    }

    fun enforce_rate_limit(
        epoch_ref: &mut u64,
        value_ref: &mut u128,
        current_epoch: u64,
        amount_e9: u128,
        limit_e9: u128,
    ) {
        if (*epoch_ref != current_epoch) {
            *epoch_ref = current_epoch;
            *value_ref = 0;
        };

        let new_total = add_u128(*value_ref, amount_e9);
        if (limit_e9 > 0) {
            assert!(new_total <= limit_e9, E_RATE_LIMIT);
        };
        *value_ref = new_total;
    }

    fun take_and_increment_nonce<StableToken, LeverageToken>(
        series: &mut CrossChainSeries<StableToken, LeverageToken>
    ): u64 {
        let nonce = series.next_voucher_nonce;
        series.next_voucher_nonce = series.next_voucher_nonce + 1;
        nonce
    }

    fun ensure_registry_match<StableToken, LeverageToken>(
        series: &CrossChainSeries<StableToken, LeverageToken>,
        registry: &CollateralRegistry,
    ) {
        assert!(series.registry_id == registry::registry_id(registry), E_REGISTRY_MISMATCH);
    }

    /// Return the most recent checkpoint metadata, if one has been verified.
    public fun latest_checkpoint<StableToken, LeverageToken>(
        series: &CrossChainSeries<StableToken, LeverageToken>
    ): option::Option<CheckpointView> {
        if (!option::is_some(&series.checkpoint)) {
            option::none<CheckpointView>()
        } else {
            let checkpoint = option::borrow(&series.checkpoint);
            option::some(CheckpointView {
                update_id: checkpoint.update_id,
                index_e9: checkpoint.index_e9,
                block_number: checkpoint.block_number,
                block_hash: clone_vec(&checkpoint.block_hash),
                balances_root: clone_vec(&checkpoint.balances_root),
                walrus_blob_id: clone_vec(&checkpoint.walrus_blob_id),
                source_timestamp_ms: checkpoint.source_timestamp_ms,
                verified_ms: checkpoint.verified_ms,
            })
        }
    }

    /// Return the current rate limit consumption for monitoring.
    public fun current_rate_usage<StableToken, LeverageToken>(
        series: &CrossChainSeries<StableToken, LeverageToken>
    ): RateLimitUsage {
        RateLimitUsage {
            mint_epoch: series.mint_rate_epoch,
            mint_value_e9: series.mint_value_e9,
            redeem_epoch: series.redeem_rate_epoch,
            redeem_value_e9: series.redeem_value_e9,
        }
    }

    public fun rate_usage_mint_value(usage: &RateLimitUsage): u128 { usage.mint_value_e9 }

    public fun voucher_state(voucher: &WithdrawalVoucher): u8 { voucher.state }

    public fun voucher_nonce(voucher: &WithdrawalVoucher): u64 { voucher.nonce }

    public fun voucher_state_pending(): u8 { VOUCHER_STATE_PENDING }

    public fun voucher_state_spent(): u8 { VOUCHER_STATE_SPENT }

    public fun voucher_state_settled(): u8 { VOUCHER_STATE_SETTLED }

    public fun checkpoint_view_update_id(view: &CheckpointView): u64 { view.update_id }

    public fun used_shares_for<StableToken, LeverageToken>(
        series: &CrossChainSeries<StableToken, LeverageToken>,
        owner: address
    ): u128 {
        if (table::contains(&series.used_shares, owner)) {
            *table::borrow(&series.used_shares, owner)
        } else { 0u128 }
    }

    public fun walrus_anchor<StableToken, LeverageToken>(
        series: &CrossChainSeries<StableToken, LeverageToken>
    ): WalrusBlobRef {
        series.walrus_anchor
    }

    /// Current xToken price snapshot stored on Sui for this cross-chain series.
    public fun xtoken_price<StableToken, LeverageToken>(
        series: &CrossChainSeries<StableToken, LeverageToken>
    ): u64 {
        series.px
    }

    public fun series_tags<StableToken, LeverageToken>(
        series: &CrossChainSeries<StableToken, LeverageToken>
    ): (vector<u8>, vector<u8>) {
        (clone_vec(&series.collateral_symbol), clone_vec(&series.chain_tag))
    }

    fun clone_vec(bytes: &vector<u8>): vector<u8> {
        let mut out = vector::empty<u8>();
        let len = vector::length(bytes);
        let mut i = 0;
        while (i < len) {
            let byte_ref = vector::borrow(bytes, i);
            vector::push_back(&mut out, *byte_ref);
            i = i + 1;
        };
        out
    }

    fun compare_vecs(a: &vector<u8>, b: &vector<u8>): bool {
        let len = vector::length(a);
        if (len != vector::length(b)) {
            return false
        };
        let mut i = 0;
        while (i < len) {
            if (*vector::borrow(a, i) != *vector::borrow(b, i)) {
                return false
            };
            i = i + 1;
        };
        true
    }

    #[test_only]
    public fun test_checkpoint_proof_for_tests(
        update_id: u64,
        index_e9: u64,
        block_number: u64,
        block_hash: vector<u8>,
        balances_root: vector<u8>,
        walrus_blob_id: vector<u8>,
        source_timestamp_ms: u64,
        proof_blob: vector<u8>,
    ): CheckpointProof {
        CheckpointProof {
            update_id,
            index_e9,
            block_number,
            block_hash,
            balances_root,
            walrus_blob_id,
            source_timestamp_ms,
            proof_blob,
        }
    }

    #[test_only]
    public fun test_balance_proof_for_tests(
        owner: address,
        update_id: u64,
        balances_root: vector<u8>,
        total_shares: u128,
        proof_blob: vector<u8>,
    ): BalanceProof {
        BalanceProof { owner, update_id, balances_root, total_shares, proof_blob }
    }

    fun add_u128(a: u128, b: u128): u128 { let c = a + b; assert!(c >= a, E_OVERFLOW); c }
    fun sub_u128(a: u128, b: u128, err: u64): u128 { assert!(a >= b, err); a - b }
    fun mul_u128(a: u128, b: u128): u128 { let c = a * b; assert!(b == 0 || c / b == a, E_OVERFLOW); c }
    fun mul3_to_u128(a: u128, b: u128, c: u128): u128 { mul_u128(mul_u128(a, b), c) }
    fun div_back_u128(n: u128, d: u128): u128 { assert!(d > 0, E_INVALID_AMOUNT); let q = n / d; if (n % d == 0) { q } else { q + 1 } }
}
