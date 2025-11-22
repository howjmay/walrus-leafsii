#[test_only]
module leafsii::test_crosschain_vault {
    use sui::clock::{Self as clock, Clock};
    use sui::coin::{Self as coin};
    use sui::test_scenario::{Self as ts, Scenario};

    use leafsii::collateral_registry::{Self as registry, CollateralRegistry, WalrusBlobRef};
    use leafsii::crosschain_vault::{Self as crosschain_vault};

    public struct TEST_FTOKEN has drop {}
    public struct TEST_XTOKEN has drop {}
    public struct TEST_PHANTOM has drop {}

    const USER: address = @0x1;
    const RELAYER: address = @0x2;
    const BETA_E9: u64 = 600_000_000;
    const LTV_RATIO_E9: u64 = 650_000_000;
    const MAINTENANCE_RATIO_E9: u64 = 720_000_000;
    const LIQ_PENALTY_E9: u64 = 60_000_000;
    const HAIRCUT_E9: u64 = 20_000_000;
    const STALENESS_CAP_MS: u64 = 3_600_000;
    const MINT_LIMIT_VALUE_E9: u128 = 1_000_000_000_000_000_000_000u128;
    const WITHDRAW_LIMIT_VALUE_E9: u128 = 1_000_000_000_000_000_000_000u128;
    const CURRENT_EPOCH: u64 = 42;
    const CHECKPOINT_INDEX_E9: u64 = 1_000_000_000;
    const ORACLE_PRICE_E9: u64 = 2_000_000_000;
    const XTOKEN_PRICE_E9: u64 = 2_500_000_000;
    const INITIAL_TOTAL_SHARES: u128 = 500;
    const EXPECTED_F_MINT: u64 = 490_000_000_000;
    const EXPECTED_X_MINT: u64 = 196_000_000_000;
    const EXPECTED_F_MINT_BRIDGE: u64 = 245_000_000_000;
    const EXPECTED_X_MINT_BRIDGE: u64 = 98_000_000_000;
    const VOUCHER_EXPIRY_DELTA: u64 = 600_000;
    const CHECKPOINT_UPDATE_ID: u64 = 1;

    #[test]
    fun test_crosschain_mint_and_voucher_flow() {
        let (mut scenario, clock) = setup_series();
        ts::next_tx(&mut scenario, USER);
        let mut series = ts::take_shared<crosschain_vault::CrossChainSeries<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let registry = ts::take_shared<CollateralRegistry>(&scenario);
        let ctx = ts::ctx(&mut scenario);

        crosschain_vault::update_checkpoint(
            &mut series,
            &registry,
            checkpoint_proof(CHECKPOINT_UPDATE_ID),
            CURRENT_EPOCH,
            &clock,
        );

        let (f_coin, x_coin) = crosschain_vault::mint_from_attested_shares(
            &mut series,
            &registry,
            balance_proof(INITIAL_TOTAL_SHARES),
            ORACLE_PRICE_E9,
            XTOKEN_PRICE_E9,
            CURRENT_EPOCH,
            &clock,
            ctx,
        );
        assert!(coin::value(&f_coin) == EXPECTED_F_MINT, 0);
        assert!(coin::value(&x_coin) == EXPECTED_X_MINT, 1);
        assert!(crosschain_vault::used_shares_for(&series, USER) == INITIAL_TOTAL_SHARES, 16);
        assert!(crosschain_vault::xtoken_price(&series) == XTOKEN_PRICE_E9, 17);

        let expiry = clock::timestamp_ms(&clock) + VOUCHER_EXPIRY_DELTA;
        let mut voucher = crosschain_vault::redeem_f(
            &mut series,
            &registry,
            f_coin,
            expiry,
            option::none<vector<u8>>(),
            CURRENT_EPOCH,
            &clock,
            ctx,
        );
        assert!(crosschain_vault::voucher_state(&voucher) == crosschain_vault::voucher_state_pending(), 2);
        assert!(crosschain_vault::voucher_nonce(&voucher) == 0, 3);

        crosschain_vault::mark_voucher_spent(&mut voucher, option::some(spent_reference()), &clock, ctx);
        assert!(crosschain_vault::voucher_state(&voucher) == crosschain_vault::voucher_state_spent(), 4);
        crosschain_vault::mark_voucher_settled(&mut voucher, option::some(settled_reference()), &clock, ctx);
        assert!(crosschain_vault::voucher_state(&voucher) == crosschain_vault::voucher_state_settled(), 5);

        let mut checkpoint_opt = crosschain_vault::latest_checkpoint(&series);
        assert!(option::is_some(&checkpoint_opt), 6);
        let checkpoint_view = option::extract(&mut checkpoint_opt);
        assert!(crosschain_vault::checkpoint_view_update_id(&checkpoint_view) == CHECKPOINT_UPDATE_ID, 7);

        let usage = crosschain_vault::current_rate_usage(&series);
        assert!(crosschain_vault::rate_usage_mint_value(&usage) > 0, 8);
        let (symbol, chain_tag) = crosschain_vault::series_tags(&series);
        assert!(symbol == asset_symbol(), 9);
        assert!(chain_tag == chain_tag(), 10);
        let anchor = crosschain_vault::walrus_anchor(&series);
        assert!(registry::walrus_blob_id(&anchor) == &walrus_blob_id(), 11);

        sui::transfer::public_transfer(voucher, USER);
        sui::transfer::public_transfer(x_coin, USER);

        ts::return_shared(series);
        ts::return_shared(registry);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    fun test_bridge_mint_sends_tokens_to_owner() {
        let (mut scenario, clock) = setup_series();
        ts::next_tx(&mut scenario, RELAYER);
        let mut series = ts::take_shared<crosschain_vault::CrossChainSeries<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let registry = ts::take_shared<CollateralRegistry>(&scenario);
        let ctx = ts::ctx(&mut scenario);

        crosschain_vault::update_checkpoint(
            &mut series,
            &registry,
            checkpoint_proof(CHECKPOINT_UPDATE_ID),
            CURRENT_EPOCH,
            &clock,
        );

        crosschain_vault::entry_bridge_mint(
            &mut series,
            &registry,
            USER,
            CHECKPOINT_UPDATE_ID,
            balances_root(),
            INITIAL_TOTAL_SHARES,
            proof_blob(),
            XTOKEN_PRICE_E9,
            CURRENT_EPOCH,
            &clock,
            ctx,
        );

        ts::return_shared(series);
        ts::return_shared(registry);

        ts::next_tx(&mut scenario, USER);
        let minted_f = ts::take_from_address<coin::Coin<TEST_FTOKEN>>(&scenario, USER);
        let minted_x = ts::take_from_address<coin::Coin<TEST_XTOKEN>>(&scenario, USER);
        assert!(coin::value(&minted_f) == EXPECTED_F_MINT_BRIDGE, 20);
        assert!(coin::value(&minted_x) == EXPECTED_X_MINT_BRIDGE, 21);
        sui::transfer::public_transfer(minted_f, USER);
        sui::transfer::public_transfer(minted_x, USER);

        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    fun setup_series(): (Scenario, Clock) {
        setup_series_with_limits(MINT_LIMIT_VALUE_E9, WITHDRAW_LIMIT_VALUE_E9)
    }

    fun setup_series_with_limits(mint_limit: u128, withdraw_limit: u128): (Scenario, Clock) {
        let mut scenario = ts::begin(@0x1);
        let ctx = ts::ctx(&mut scenario);
        let clock = clock::create_for_testing(ctx);

        let mut reg = registry::init_registry(ctx);
        let risk = registry::new_crosschain_risk_config(
            LTV_RATIO_E9,
            MAINTENANCE_RATIO_E9,
            LIQ_PENALTY_E9,
            HAIRCUT_E9,
            STALENESS_CAP_MS,
            mint_limit,
            withdraw_limit,
        );
        registry::register_crosschain<TEST_PHANTOM>(
            &mut reg,
            asset_symbol(),
            chain_tag(),
            price_feed_id(),
            BETA_E9,
            max_capacity(),
            walrus_anchor(),
            risk,
            CURRENT_EPOCH,
        );

        let stable_cap = coin::create_treasury_cap_for_testing<TEST_FTOKEN>(ctx);
        let leverage_cap = coin::create_treasury_cap_for_testing<TEST_XTOKEN>(ctx);
        let series = crosschain_vault::init_crosschain_series<TEST_FTOKEN, TEST_XTOKEN>(
            &reg,
            asset_symbol(),
            stable_cap,
            leverage_cap,
            ctx,
        );

        sui::transfer::public_share_object(reg);
        sui::transfer::public_share_object(series);

        (scenario, clock)
    }

    fun asset_symbol(): vector<u8> {
        b"ETH"
    }

    fun chain_tag(): vector<u8> {
        b"ethereum"
    }

    fun price_feed_id(): vector<u8> {
        b"ETH_USD"
    }

    fun max_capacity(): u64 {
        10_000_000_000
    }

    fun walrus_anchor(): WalrusBlobRef {
        registry::test_walrus_anchor_with_blob(@0x99, @0x55, walrus_blob_id(), b"hash", CURRENT_EPOCH + 10)
    }

    fun checkpoint_proof(update_id: u64): crosschain_vault::CheckpointProof {
        crosschain_vault::test_checkpoint_proof_for_tests(
            update_id,
            CHECKPOINT_INDEX_E9,
            1_234_567,
            b"block-hash",
            balances_root(),
            walrus_blob_id(),
            1_000,
            proof_blob(),
        )
    }

    fun balance_proof(total_shares: u128): crosschain_vault::BalanceProof {
        crosschain_vault::test_balance_proof_for_tests(
            USER,
            CHECKPOINT_UPDATE_ID,
            balances_root(),
            total_shares,
            proof_blob(),
        )
    }

    fun walrus_blob_id(): vector<u8> {
        b"walrus-blob"
    }

    fun balances_root(): vector<u8> {
        b"balances-root"
    }

    fun proof_blob(): vector<u8> {
        b"zk-proof"
    }

    fun spent_reference(): vector<u8> {
        b"eth-spent"
    }

    fun settled_reference(): vector<u8> {
        b"eth-settled"
    }
}
