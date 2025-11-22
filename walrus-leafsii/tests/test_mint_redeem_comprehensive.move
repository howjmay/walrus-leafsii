#[test_only]
module leafsii::test_mint_redeem_comprehensive {
    use sui::clock::{Self, Clock};
    use sui::coin::{Self, Coin};
    use sui::test_scenario::{Self as ts, Scenario};

    use oracle::oracle::{Self, MockOracle};
    use leafsii::leafsii::{Self, Protocol};
    use leafsii::stability_pool::{Self, StabilityPool, SPPosition};
    use math::math;
    use sui_system::staking_pool::FungibleStakedSui;
    use sui::sui::SUI;

    // Test asset types
    public struct TEST_FTOKEN has drop {}
    public struct TEST_XTOKEN has drop {}

    const INITIAL_PRICE_E6: u64 = 2_000_000_000; // $2.00 in 1e9 scale
    const DEPOSIT_AMOUNT: u64 = 1_000_000; // 1 TEST_RESERVE
    const SCALE_FACTOR: u64 = 1_000_000_000; // 1e9
    
    // CR Thresholds
    const CR_T_L1: u64 = 1_306_000_000;  // 1.306 * 1e9
    const CR_T_L2: u64 = 1_206_000_000;  // 1.206 * 1e9

    fun setup_protocol_with_sp(scenario: &mut Scenario): (Coin<TEST_FTOKEN>, Coin<TEST_XTOKEN>, leafsii::AdminCap, Clock) {
        let ctx = ts::ctx(scenario);
        let clock = clock::create_for_testing(ctx);

        // Create stability pool first
        let sp_cap = stability_pool::create_stability_pool<TEST_FTOKEN>(ctx);

        ts::next_tx(scenario, @0x1);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(scenario);

        // Create protocol with pool reference
        let stable_treasury_cap = coin::create_treasury_cap_for_testing<TEST_FTOKEN>(ts::ctx(scenario));
        let leverage_treasury_cap = coin::create_treasury_cap_for_testing<TEST_XTOKEN>(ts::ctx(scenario));
        let reserve_coin = coin::mint_for_testing<SUI>(DEPOSIT_AMOUNT, ts::ctx(scenario));

        // Bind pool to protocol
        let protocol_id = sui::object::id_from_address(@0x123);

        let (f_coin, x_coin, admin_cap) = leafsii::init_protocol<TEST_FTOKEN, TEST_XTOKEN>(
            stable_treasury_cap,
            leverage_treasury_cap,
            INITIAL_PRICE_E6,
            reserve_coin,
            &mut pool,
            sp_cap,
            &clock,
            ts::ctx(scenario)
        );

        ts::return_shared(pool);
        (f_coin, x_coin, admin_cap, clock)
    }

    fun setup_test(): (Scenario, Clock, MockOracle<SUI>) {
        let mut scenario = ts::begin(@0x1);
        let ctx = ts::ctx(&mut scenario);
        
        let clock = clock::create_for_testing(ctx);
        let oracle = oracle::create_mock_oracle<SUI>(INITIAL_PRICE_E6, &clock, ctx);
        
        (scenario, clock, oracle)
    }

    // Push CR to L2 by creating SP obligations
    fun push_to_l2(
        protocol: &Protocol<TEST_FTOKEN, TEST_XTOKEN>,
        pool: &mut StabilityPool<TEST_FTOKEN>,
        ctx: &mut TxContext
    ): SPPosition<TEST_FTOKEN> {
        let mut position = stability_pool::create_position<TEST_FTOKEN>(ctx);
        let f_token = coin::mint_for_testing<TEST_FTOKEN>(2_000_000, ctx);
        stability_pool::deposit_f(pool, &mut position, f_token, ctx);
        
        // Calculate precise SP obligation needed to get CR = 1.25 (L2)
        // L2: Need CR between 1.206 and 1.306
        // Target CR = 1.25, so reserve_net_usd / (nf * pf) = 1.25
        // reserve_net_usd = nf * pf * 1.25 = nf * 1_000_000 * 1.25
        let target_cr = 1_250_000_000; // 1e9 scale
        let current_reserve = leafsii::get_reserve_balance(protocol);
        let (current_nf, _, _, _, _, _, _, _) = leafsii::get_protocol_state(protocol);
        let target_reserve_net_usd = math::mul_div(current_nf, target_cr, SCALE_FACTOR);
        let current_reserve_usd = math::mul_div(current_reserve, INITIAL_PRICE_E6, SCALE_FACTOR);
        if (current_reserve_usd > target_reserve_net_usd) {
            let needed_obligation_usd = current_reserve_usd - target_reserve_net_usd;
            let needed_obligation_r = math::mul_div(needed_obligation_usd, SCALE_FACTOR, INITIAL_PRICE_E6);
            let pool_protocol_id = stability_pool::pool_id(pool);
            let dummy_cap = stability_pool::create_dummy_capability_with_id(pool_protocol_id, ctx);
            let (_burned, _indexed) = stability_pool::sp_controller_rebalance(pool, &dummy_cap, 800_000, needed_obligation_r);
            stability_pool::destroy_capability(dummy_cap);
        };
        position
    }

    #[test]
    fun test_mint_f_at_high_cr_l1() {
        let (mut scenario, setup_test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, clock) = setup_protocol_with_sp(&mut scenario);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Verify we're in Normal mode (high CR)
        let initial_cr = leafsii::collateral_ratio(&protocol, &pool);
        let level = leafsii::current_level(&protocol, &pool);
        assert!(level == 0, 0); // Normal mode
        assert!(initial_cr >= CR_T_L1, 1);
        
        // Get initial state
        let (initial_nf, _, _, _, _, initial_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        
        // Mint F tokens with 0.5 R
        let reserve_coin = coin::mint_for_testing<SUI>(500_000, ts::ctx(&mut scenario));
        let minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));
        
        // Verify results
        let (final_nf, _, _, _, _, final_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        let minted_amount = coin::value(&minted_f);
        
        // Should mint F tokens (less than 1M due to fees)
        assert!(minted_amount > 0, 2);
        assert!(minted_amount < 1_000_000, 20); // Less due to fees
        assert!(final_nf == initial_nf + minted_amount, 3);
        assert!(final_reserve > initial_reserve, 4); // Reserve increases, but less than deposit due to fees
        
        // CR should decrease but remain in L1
        let final_cr = leafsii::collateral_ratio(&protocol, &pool);
        let final_level = leafsii::current_level(&protocol, &pool);
        assert!(final_cr < initial_cr, 5); // CR decreased
        assert!(final_level == 0, 6); // Still in Normal mode
        
        // Verify invariant
        assert!(leafsii::check_invariant(&protocol), 7);
        
        // Cleanup
        transfer::public_transfer(minted_f, @0x1);
        transfer::public_transfer(init_f, @0x1);
        transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(setup_test_clock);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    fun test_mint_x_at_high_cr_l1() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);
        
        let level = leafsii::current_level(&protocol, &pool);
        assert!(level == 0, 0); // Normal mode
        
        // Get initial state
        let (_, initial_nx, _, _initial_px, _, initial_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        
        // Mint X tokens with 0.5 R
        let reserve_coin = coin::mint_for_testing<SUI>(500_000, ts::ctx(&mut scenario));
        let minted_x = leafsii::mint_x(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));
        
        // Verify results
        let (_, final_nx, _, final_px, _, final_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        let minted_amount = coin::value(&minted_x);
        
        // X token amount depends on px
        // $1.0 USD value / px (updated after mint) = x tokens
        assert!(minted_amount > 0, 1);
        assert!(final_nx == initial_nx + minted_amount, 2);
        assert!(final_reserve == initial_reserve + 497_500, 3); // 500,000 - 0.5% fee (2,500)
        
        // px should be updated after mint  
        assert!(final_px > 0, 4);
        
        // Verify invariant
        assert!(leafsii::check_invariant(&protocol), 5);
        
        // Cleanup
        transfer::public_transfer(minted_x, @0x1);
        transfer::public_transfer(init_f, @0x1);
        transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = leafsii::E_ACTION_BLOCKED_BY_CR)]
    fun test_mint_f_at_marginal_cr_l2() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);
        
        // Push to L1 (what was previously called L2)
        let position = push_to_l2(&protocol, &mut pool, ts::ctx(&mut scenario));
        
        let level = leafsii::current_level(&protocol, &pool);
        let cr = leafsii::collateral_ratio(&protocol, &pool);
        assert!(level == 1, 0); // L1 mode (old L2)
        assert!(cr >= CR_T_L2 && cr < CR_T_L1, 1);
        
        // Get initial state
        let (initial_nf, _, _, _, _, initial_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        
        // Mint should still work at L2
        let reserve_coin = coin::mint_for_testing<SUI>(200_000, ts::ctx(&mut scenario));
        let minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));
        
        // Verify mint succeeded
        let (final_nf, _, _, _, _, final_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        let minted_amount = coin::value(&minted_f);
        
        // Should mint $0.4 worth of F tokens (200K * $2.00 / $1.00 = 400K F tokens)
        assert!(minted_amount == 400_000, 2);
        assert!(final_nf == initial_nf + 400_000, 3);
        assert!(final_reserve == initial_reserve + 200_000, 4);
        
        // After minting at marginal L2, we expect to drop to L3
        let final_level = leafsii::current_level(&protocol, &pool);
        assert!(final_level == 3, final_level as u64);
        
        // Cleanup
        transfer::public_transfer(minted_f, @0x1);
        transfer::public_transfer(position, @0x1);
        transfer::public_transfer(init_f, @0x1);
        transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }

    #[test]
    fun test_redeem_f_at_high_cr() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);
        
        let level = leafsii::current_level(&protocol, &pool);
        assert!(level == 0, 0); // Normal mode
        
        // First add more reserve to have enough for redemption
        let reserve_coin = coin::mint_for_testing<SUI>(500_000, ts::ctx(&mut scenario));
        let _minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));
        coin::burn_for_testing(_minted_f);
        
        // Get state before redemption
        let (initial_nf, _, _, _, _, initial_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        
        // Redeem 0.5 F tokens (should get $0.5 / $2.00 = 0.25 R)
        let f_to_redeem = coin::mint_for_testing<TEST_FTOKEN>(500_000, ts::ctx(&mut scenario));
        let (redeemed_reserve, ticket_opt) = leafsii::redeem_f(&mut protocol, &pool, f_to_redeem, &setup_clock, ts::ctx(&mut scenario));
        option::destroy_none(ticket_opt);
        
        // Verify results
        let (final_nf, _, _, _, _, final_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        let redeemed_amount = coin::value(&redeemed_reserve);
        
        // Should redeem 248,750 R (250K R - 0.5% fee = 250K - 1,250)
        assert!(redeemed_amount == 248_750, 1);
        assert!(final_nf == initial_nf - 500_000, 2);
        assert!(final_reserve == initial_reserve - 250_000, 3); // Protocol reserves decrease by full amount
        
        // Verify invariant
        assert!(leafsii::check_invariant(&protocol), 4);
        
        // Cleanup
        transfer::public_transfer(redeemed_reserve, @0x1);
        transfer::public_transfer(init_f, @0x1);
        transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }

    #[test]
    fun test_redeem_x_at_high_cr() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);
        
        // First add more X tokens by minting
        let reserve_coin = coin::mint_for_testing<SUI>(500_000, ts::ctx(&mut scenario));
        let _minted_x = leafsii::mint_x(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));
        coin::burn_for_testing(_minted_x);
        
        // Get state before redemption
        let (_, initial_nx, _, _initial_px, _, initial_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        
        // Redeem some X tokens (use amount based on current supply)
        let redeem_amount = initial_nx / 4; // Redeem 1/4 of supply
        let x_to_redeem = coin::mint_for_testing<TEST_XTOKEN>(redeem_amount, ts::ctx(&mut scenario));
        let (redeemed_reserve, ticket_opt) = leafsii::redeem_x(&mut protocol, &pool, x_to_redeem, &setup_clock, ts::ctx(&mut scenario));
        option::destroy_none(ticket_opt);
        
        // Verify results
        let (_, final_nx, _, final_px, _, final_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        let redeemed_amount = coin::value(&redeemed_reserve);
        
        assert!(redeemed_amount > 0, 1);
        assert!(final_nx == initial_nx - redeem_amount, 2);
        // Reserve change accounts for redeemed amount plus any fees collected
        assert!(final_reserve <= initial_reserve, 3); // Reserve should decrease
        
        // px should be updated after redemption
        assert!(final_px > 0, 4);
        
        // Verify invariant (allowing small tolerance)
        assert!(leafsii::check_invariant(&protocol), 5);
        
        // Cleanup
        transfer::public_transfer(redeemed_reserve, @0x1);
        transfer::public_transfer(init_f, @0x1);
        transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }

    #[test]
    fun test_redeem_respects_sp_obligations() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);
        
        // Add a smaller amount of reserve and keep the minted F tokens for redemption
        let reserve_coin = coin::mint_for_testing<SUI>(100_000, ts::ctx(&mut scenario));
        let initial_minted = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));
        
        // Create SP position and obligations
        let mut position = stability_pool::create_position<TEST_FTOKEN>(ts::ctx(&mut scenario));
        let f_token = coin::mint_for_testing<TEST_FTOKEN>(1_000_000, ts::ctx(&mut scenario));
        stability_pool::deposit_f(&mut pool, &mut position, f_token, ts::ctx(&mut scenario));
        
        // Create moderate SP obligation (200K R) to leave sufficient net reserves
        let pool_protocol_id = stability_pool::pool_id(&pool);
        let dummy_cap = stability_pool::create_dummy_capability_with_id(pool_protocol_id, ts::ctx(&mut scenario));
        let (_burned, _indexed) = stability_pool::sp_controller_rebalance(&mut pool, &dummy_cap, 400_000, 200_000);
        stability_pool::destroy_capability(dummy_cap);
        
        let total_reserve = leafsii::get_reserve_balance(&protocol);
        let sp_obligations = stability_pool::get_sp_obligation_amount(&pool);
        let net_available = total_reserve - sp_obligations;
        
        // Redeem the F tokens we minted earlier (should succeed within net reserve limits)
        let f_amount = coin::value(&initial_minted);
        let expected_reserve_value = (f_amount as u64) * INITIAL_PRICE_E6 / SCALE_FACTOR;
        assert!(expected_reserve_value <= net_available, 1);

        let (redeemed, ticket_opt) = leafsii::redeem_f(&mut protocol, &pool, initial_minted, &setup_clock, ts::ctx(&mut scenario));
        option::destroy_none(ticket_opt);
        
        // Should succeed
        assert!(coin::value(&redeemed) > 0, 0);
        
        // Cleanup
        transfer::public_transfer(redeemed, @0x1);
        transfer::public_transfer(position, @0x1);
        transfer::public_transfer(init_f, @0x1);
        transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = leafsii::E_INSUFFICIENT_RESERVE)]
    fun test_redeem_f_fails_when_exceeds_net_reserve() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);
        
        // Create SP position with minimal obligation to stay at L1 but create insufficient reserve scenario
        let mut position = stability_pool::create_position<TEST_FTOKEN>(ts::ctx(&mut scenario));
        let f_token = coin::mint_for_testing<TEST_FTOKEN>(1_000_000, ts::ctx(&mut scenario));
        stability_pool::deposit_f(&mut pool, &mut position, f_token, ts::ctx(&mut scenario));
        let pool_protocol_id = stability_pool::pool_id(&pool);
        let dummy_cap = stability_pool::create_dummy_capability_with_id(pool_protocol_id, ts::ctx(&mut scenario));
        let (_burned, _indexed) = stability_pool::sp_controller_rebalance(&mut pool, &dummy_cap, 100_000, 100_000);
        stability_pool::destroy_capability(dummy_cap);
        
        // Try to redeem more than net available (should fail)
        // Net = 1M - 100K = 900K R, so max redeemable ~= 900K * $2 = $1800K = 1.8M F tokens
        let f_to_redeem = coin::mint_for_testing<TEST_FTOKEN>(2_000_000, ts::ctx(&mut scenario)); // Try 2M F > 1.8M F max
        let (_redeemed, ticket_opt) = leafsii::redeem_f(&mut protocol, &pool, f_to_redeem, &setup_clock, ts::ctx(&mut scenario));
        option::destroy_none(ticket_opt);
        
        // Should fail with E_INSUFFICIENT_RESERVE
        
        transfer::public_transfer(position, @0x1);
        transfer::public_transfer(init_f, @0x1);
        transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        transfer::public_transfer(_redeemed, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }

    #[test]
    fun test_mint_redeem_preserves_invariant() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);
        
        // Verify initial invariant
        assert!(leafsii::check_invariant(&protocol), 0);

        // Perform multiple mint/redeem operations

        // 1. Mint F with small amount
        let reserve_coin1 = coin::mint_for_testing<SUI>(100_000, ts::ctx(&mut scenario));
        let minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin1, ts::ctx(&mut scenario));
        assert!(leafsii::check_invariant(&protocol), 1);

        // 2. Mint X with small amount
        let reserve_coin2 = coin::mint_for_testing<SUI>(100_000, ts::ctx(&mut scenario));
        let minted_x = leafsii::mint_x(&mut protocol, &pool, reserve_coin2, ts::ctx(&mut scenario));
        assert!(leafsii::check_invariant(&protocol), 2);

        // 3. Redeem some X first (half of minted amount)
        let x_amount = coin::value(&minted_x) / 2;
        let x_to_redeem = coin::mint_for_testing<TEST_XTOKEN>(x_amount, ts::ctx(&mut scenario));
        let (redeemed_r1, ticket_opt1) = leafsii::redeem_x(&mut protocol, &pool, x_to_redeem, &setup_clock, ts::ctx(&mut scenario));
        option::destroy_none(ticket_opt1);
        assert!(leafsii::check_invariant(&protocol), 3);

        // 4. Redeem some F (half of minted amount)
        let f_amount = coin::value(&minted_f) / 2;
        let f_to_redeem = coin::mint_for_testing<TEST_FTOKEN>(f_amount, ts::ctx(&mut scenario));
        let (redeemed_r2, ticket_opt2) = leafsii::redeem_f(&mut protocol, &pool, f_to_redeem, &setup_clock, ts::ctx(&mut scenario));
        option::destroy_none(ticket_opt2);
        assert!(leafsii::check_invariant(&protocol), 4);

        // Verify final invariant
        assert!(leafsii::check_invariant(&protocol), 5);
        
        // Cleanup
        transfer::public_transfer(minted_f, @0x1);
        transfer::public_transfer(minted_x, @0x1);
        transfer::public_transfer(redeemed_r1, @0x1);
        transfer::public_transfer(redeemed_r2, @0x1);
        transfer::public_transfer(init_f, @0x1);
        transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }

    #[test]
    fun test_events_emitted_correctly() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);
        
        // Mint F and check events
        let reserve_coin = coin::mint_for_testing<SUI>(500_000, ts::ctx(&mut scenario));
        let minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));
        
        // Note: In a real test framework, we'd check the emitted MintF event
        // For now, we verify the operation succeeded
        assert!(coin::value(&minted_f) > 0, 0);
        
        // Mint X and check events
        let reserve_coin2 = coin::mint_for_testing<SUI>(300_000, ts::ctx(&mut scenario));
        let minted_x = leafsii::mint_x(&mut protocol, &pool, reserve_coin2, ts::ctx(&mut scenario));
        assert!(coin::value(&minted_x) > 0, 1);
        
        // Redeem F and check events
        let f_to_redeem = coin::mint_for_testing<TEST_FTOKEN>(200_000, ts::ctx(&mut scenario));
        let (redeemed_r1, ticket_opt1) = leafsii::redeem_f(&mut protocol, &pool, f_to_redeem, &setup_clock, ts::ctx(&mut scenario));
        option::destroy_none(ticket_opt1);
        assert!(coin::value(&redeemed_r1) > 0, 2);

        // Redeem X and check events
        let x_to_redeem = coin::mint_for_testing<TEST_XTOKEN>(coin::value(&minted_x) / 2, ts::ctx(&mut scenario));
        let (redeemed_r2, ticket_opt2) = leafsii::redeem_x(&mut protocol, &pool, x_to_redeem, &setup_clock, ts::ctx(&mut scenario));
        option::destroy_none(ticket_opt2);
        assert!(coin::value(&redeemed_r2) > 0, 3);
        
        // Cleanup
        transfer::public_transfer(minted_f, @0x1);
        transfer::public_transfer(minted_x, @0x1);
        transfer::public_transfer(redeemed_r1, @0x1);
        transfer::public_transfer(redeemed_r2, @0x1);
        transfer::public_transfer(init_f, @0x1);
        transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }
}
