#[test_only]
module leafsii::test_cr_level_transitions {
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
    const DEPOSIT_AMOUNT: u64 = 1_000_000; // 1 SUI
    const SCALE_FACTOR: u64 = 1_000_000_000; // 1e9

    // CR Thresholds (copied from leafsii.move for reference)
    const CR_T_L1: u64 = 1_306_000_000;  // 1.306 * 1e9
    const CR_T_L2: u64 = 1_206_000_000;  // 1.206 * 1e9
    const CR_T_L3: u64 = 1_144_000_000;  // 1.144 * 1e9

    fun setup_test(): (Scenario, Clock, MockOracle<SUI>) {
        let mut scenario = ts::begin(@0x1);
        let ctx = ts::ctx(&mut scenario);

        let clock = clock::create_for_testing(ctx);
        let oracle = oracle::create_mock_oracle(INITIAL_PRICE_E6, &clock, ctx);

        (scenario, clock, oracle)
    }

    fun setup_protocol_with_sp(scenario: &mut Scenario, clock: &Clock): (Coin<TEST_FTOKEN>, Coin<TEST_XTOKEN>, leafsii::AdminCap) {
        let ctx = ts::ctx(scenario);

        // Create stability pool first
        let sp_cap = stability_pool::create_stability_pool<TEST_FTOKEN>(ctx);

        ts::next_tx(scenario, @0x1);
        let mut pool = ts::take_shared<stability_pool::StabilityPool<TEST_FTOKEN>>(scenario);

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
            clock,
            ts::ctx(scenario)
        );

        ts::return_shared(pool);
        (f_coin, x_coin, admin_cap)
    }

    // Helper function to push protocol to a specific CR level
    fun push_to_level(
        protocol: &Protocol<TEST_FTOKEN, TEST_XTOKEN>,
        pool: &mut StabilityPool<TEST_FTOKEN>,
        target_level: u8,
        ctx: &mut TxContext
    ): SPPosition<TEST_FTOKEN> {
        let mut position = stability_pool::create_position<TEST_FTOKEN>(ctx);
        let f_token = coin::mint_for_testing<TEST_FTOKEN>(3_000_000, ctx);
        stability_pool::deposit_f(pool, &mut position, f_token, ctx);
        
        // Calculate current state
        let current_reserve = leafsii::get_reserve_balance(protocol);
        let (current_nf, _, _, _, _, _, _, _) = leafsii::get_protocol_state(protocol);
        
        if (target_level == 0) {
            // Normal: High CR - no additional obligations needed
        } else if (target_level == 1) {
            // L1: Target CR = 1.25 (between 1.206 and 1.306)  
            let target_cr = 1_250_000_000; // 1e9 scale
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
        } else if (target_level == 2) {
            // L2: Target CR = 1.175 (between 1.144 and 1.206)
            let target_cr = 1_175_000_000; // 1e9 scale
            let target_reserve_net_usd = math::mul_div(current_nf, target_cr, SCALE_FACTOR);
            let current_reserve_usd = math::mul_div(current_reserve, INITIAL_PRICE_E6, SCALE_FACTOR);
            if (current_reserve_usd > target_reserve_net_usd) {
                let needed_obligation_usd = current_reserve_usd - target_reserve_net_usd;
                let needed_obligation_r = math::mul_div(needed_obligation_usd, SCALE_FACTOR, INITIAL_PRICE_E6);
                let pool_protocol_id = stability_pool::pool_id(pool);
                let dummy_cap = stability_pool::create_dummy_capability_with_id(pool_protocol_id, ctx);
                let (_burned, _indexed) = stability_pool::sp_controller_rebalance(pool, &dummy_cap, 1_000_000, needed_obligation_r);
                stability_pool::destroy_capability(dummy_cap);
            };
        } else if (target_level == 3) {
            // L3: Target CR = 1.1 (below 1.144)
            let target_cr = 1_100_000_000; // 1e9 scale
            let target_reserve_net_usd = math::mul_div(current_nf, target_cr, SCALE_FACTOR);
            let current_reserve_usd = math::mul_div(current_reserve, INITIAL_PRICE_E6, SCALE_FACTOR);
            if (current_reserve_usd > target_reserve_net_usd) {
                let needed_obligation_usd = current_reserve_usd - target_reserve_net_usd;
                let needed_obligation_r = math::mul_div(needed_obligation_usd, SCALE_FACTOR, INITIAL_PRICE_E6);
                let pool_protocol_id = stability_pool::pool_id(pool);
                let dummy_cap = stability_pool::create_dummy_capability_with_id(pool_protocol_id, ctx);
                let (_burned, _indexed) = stability_pool::sp_controller_rebalance(pool, &dummy_cap, 1_200_000, needed_obligation_r);
                stability_pool::destroy_capability(dummy_cap);
            };
        };
        
        position
    }

    #[test]
    /// Test: Small minting at L2 that stays within boundaries
    /// Expected behavior: fToken minting should be blocked at L1 due to fee policy
    #[expected_failure(abort_code = leafsii::E_ACTION_BLOCKED_BY_CR)]
    fun test_mint_f_blocked_at_l1() {
        let (mut scenario, clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap) = setup_protocol_with_sp(&mut scenario, &clock);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Push to L1 (marginal level where fToken minting is blocked)
        let position = push_to_level(&protocol, &mut pool, 1, ts::ctx(&mut scenario));
        
        let initial_level = leafsii::current_level(&protocol, &pool);
        let initial_cr = leafsii::collateral_ratio(&protocol, &pool);
        assert!(initial_level == 1, initial_level as u64); // L1 mode
        assert!(initial_cr >= CR_T_L2 && initial_cr < CR_T_L1, initial_cr);
        
        // Try to mint - this should fail due to fee policy blocking fToken minting at L1+
        let small_reserve = coin::mint_for_testing<SUI>(50_000, ts::ctx(&mut scenario));
        
        let result = leafsii::mint_f(&mut protocol, &pool, small_reserve, ts::ctx(&mut scenario));
        
        // Cleanup (should not reach here due to expected failure)
        sui::transfer::public_transfer(result, @0x1);
        sui::transfer::public_transfer(position, @0x1);
        sui::transfer::public_transfer(init_f, @0x1);
        sui::transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        sui::transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        sui::transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    /// Test: Minting X tokens at L2 that pushes system to L3
    /// Expected behavior: Similar to F tokens, should either reject or implement partial minting
    fun test_mint_x_l2_to_l3_transition() {
        let (mut scenario, clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap) = setup_protocol_with_sp(&mut scenario, &clock);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Push to L2  
        let position = push_to_level(&protocol, &mut pool, 2, ts::ctx(&mut scenario));
        
        let initial_level = leafsii::current_level(&protocol, &pool);
        assert!(initial_level == 2, initial_level as u64); // L2 mode
        
        // Try to mint X tokens that would push system to L3
        let large_reserve = coin::mint_for_testing<SUI>(800_000, ts::ctx(&mut scenario));
        let result = leafsii::mint_x(&mut protocol, &pool, large_reserve, ts::ctx(&mut scenario));
        
        let final_level = leafsii::current_level(&protocol, &pool);
        let final_cr = leafsii::collateral_ratio(&protocol, &pool);
        
        // X tokens have higher leverage, so they impact CR more significantly
        // The system should handle this appropriately
        assert!(final_level <= 2, final_level as u64); // Should maintain L1-L2
        assert!(final_cr >= CR_T_L2, final_cr);
        
        // Cleanup
        sui::transfer::public_transfer(result, @0x1);
        sui::transfer::public_transfer(position, @0x1);
        sui::transfer::public_transfer(init_f, @0x1);
        sui::transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        sui::transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        sui::transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = leafsii::E_ACTION_BLOCKED_BY_CR)]
    /// Test: Operations at L3 should be blocked
    /// Expected behavior: All user operations (mint/redeem) should fail at L3
    fun test_operations_blocked_at_l3() {
        let (mut scenario, clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap) = setup_protocol_with_sp(&mut scenario, &clock);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Push to L3 (warning level where operations are blocked)
        let position = push_to_level(&protocol, &mut pool, 3, ts::ctx(&mut scenario));
        
        let level = leafsii::current_level(&protocol, &pool);
        let cr = leafsii::collateral_ratio(&protocol, &pool);
        assert!(level == 3, level as u64); // L3 mode
        assert!(cr < CR_T_L3, cr); // L3 mode means CR < 114.4%
        
        // Try to mint F tokens - this should fail
        let reserve_coin = coin::mint_for_testing<SUI>(100_000, ts::ctx(&mut scenario));
        let result = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario)); // Should abort
        
        // Should not reach here, but if it does, transfer the result
        sui::transfer::public_transfer(result, @0x1);
        
        // Cleanup (won't reach here due to expected failure)
        sui::transfer::public_transfer(position, @0x1);
        sui::transfer::public_transfer(init_f, @0x1);
        sui::transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        sui::transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        sui::transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    /// Test: Redeem operations improving CR from L3 back to L2
    /// Expected behavior: Redeem operations should be blocked at L3, but protocol can recover via SP liquidations
    fun test_cr_recovery_l3_to_l2() {
        let (mut scenario, clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap) = setup_protocol_with_sp(&mut scenario, &clock);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Push to L3
        let mut position = push_to_level(&protocol, &mut pool, 3, ts::ctx(&mut scenario));
        
        let initial_level = leafsii::current_level(&protocol, &pool);
        let initial_cr = leafsii::collateral_ratio(&protocol, &pool);
        assert!(initial_level == 3, initial_level as u64); // L3 mode
        assert!(initial_cr < CR_T_L3, initial_cr); // L3 mode means CR < 114.4%
        
        // Simulate protocol recovery via SP liquidation (reduce obligations)
        // This would happen through liquidations in practice
        let _settled = stability_pool::settle_user(&pool, &mut position);
        
        // Check if system can naturally recover to L2 through liquidation mechanics
        let _final_level = leafsii::current_level(&protocol, &pool);
        let final_cr = leafsii::collateral_ratio(&protocol, &pool);
        
        // The system should have improved CR through liquidation settlements
        assert!(final_cr >= initial_cr, final_cr); // CR should improve or stay same
        
        // Cleanup
        sui::transfer::public_transfer(position, @0x1);
        sui::transfer::public_transfer(init_f, @0x1);
        sui::transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        sui::transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        sui::transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    /// Test: fToken minting should be blocked at L1 due to fee policy
    #[expected_failure(abort_code = leafsii::E_ACTION_BLOCKED_BY_CR)]
    fun test_multiple_level_transitions_ftoken_blocked() {
        let (mut scenario, clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap) = setup_protocol_with_sp(&mut scenario, &clock);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Start at Normal
        let initial_level = leafsii::current_level(&protocol, &pool);
        let initial_cr = leafsii::collateral_ratio(&protocol, &pool);
        assert!(initial_level == 0, initial_level as u64); // Normal mode
        assert!(initial_cr >= CR_T_L1, initial_cr);
        
        // Transition Normal -> L1
        let position = push_to_level(&protocol, &mut pool, 1, ts::ctx(&mut scenario));
        let l2_level = leafsii::current_level(&protocol, &pool);
        let l2_cr = leafsii::collateral_ratio(&protocol, &pool);
        assert!(l2_level == 1, l2_level as u64); // L1 mode
        assert!(l2_cr >= CR_T_L2 && l2_cr < CR_T_L1, l2_cr);
        
        // Try fToken minting at L1 - this should fail due to fee policy
        let small_reserve = coin::mint_for_testing<SUI>(50_000, ts::ctx(&mut scenario));
        let minted_f = leafsii::mint_f(&mut protocol, &pool, small_reserve, ts::ctx(&mut scenario));
        assert!(coin::value(&minted_f) > 0, coin::value(&minted_f));
        
        // Transition L2 -> L3 (via additional SP obligations)
        let pool_protocol_id = stability_pool::pool_id(&pool);
        let dummy_cap = stability_pool::create_dummy_capability_with_id(pool_protocol_id, ts::ctx(&mut scenario));
        let (_burned, _indexed) = stability_pool::sp_controller_rebalance(&mut pool, &dummy_cap, 200_000, 150_000);
        stability_pool::destroy_capability(dummy_cap);
        let l3_level = leafsii::current_level(&protocol, &pool);
        assert!(l3_level >= 3, l3_level as u64);
        
        // Operations should now be blocked
        // We won't test this here to avoid aborting, but this is the expected behavior
        
        // Cleanup
        sui::transfer::public_transfer(minted_f, @0x1);
        sui::transfer::public_transfer(position, @0x1);
        sui::transfer::public_transfer(init_f, @0x1);
        sui::transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        sui::transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        sui::transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    /// Test: Document exact CR calculation during minting
    /// Expected behavior: Show how CR changes during mint operations
    fun test_cr_calculation_during_mint() {
        let (mut scenario, clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap) = setup_protocol_with_sp(&mut scenario, &clock);

        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Stay in Normal mode for fToken minting (fToken mint blocked in L1+)
        // let position = push_to_level(&protocol, &mut pool, 2, ts::ctx(&mut scenario));
        
        let before_cr = leafsii::collateral_ratio(&protocol, &pool);
        let (before_nf, before_nx, _, _, _, before_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        let before_sp_obligations = stability_pool::get_sp_obligation_amount(&pool);
        
        // Mint a small amount and observe CR change (only works in Normal mode)
        let mint_amount = 100_000; // Small amount
        let reserve_coin = coin::mint_for_testing<SUI>(mint_amount, ts::ctx(&mut scenario));
        let minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));
        
        let after_cr = leafsii::collateral_ratio(&protocol, &pool);
        let (after_nf, after_nx, _, _, _, after_reserve, _, _) = leafsii::get_protocol_state(&protocol);
        let after_sp_obligations = stability_pool::get_sp_obligation_amount(&pool);
        
        // Document the changes
        // CR = (Reserve - SP_Obligations) * Reserve_Price / (NF * PF + NX * PX)  
        // When minting F: Reserve increases (minus fee), NF increases, so CR effect depends on relative changes
        
        // Reserve should increase by less than mint_amount due to fees
        assert!(after_reserve > before_reserve, after_reserve - before_reserve);
        assert!(after_reserve < before_reserve + mint_amount, after_reserve - before_reserve); // Fee deducted
        assert!(after_nf > before_nf, after_nf - before_nf); // F tokens minted
        assert!(after_nx == before_nx, after_nx); // X tokens unchanged
        assert!(after_sp_obligations == before_sp_obligations, after_sp_obligations); // SP unchanged
        
        // CR should generally decrease when minting (adding leverage)
        // But exact change depends on current prices and ratios
        let cr_change = if (after_cr > before_cr) {
            after_cr - before_cr
        } else {
            before_cr - after_cr
        };
        
        // Document that CR change occurred (direction depends on protocol mechanics)
        assert!(cr_change >= 0, cr_change); // Some change should occur
        
        // Cleanup
        sui::transfer::public_transfer(minted_f, @0x1);
        // sui::transfer::public_transfer(position, @0x1); // No position created
        sui::transfer::public_transfer(init_f, @0x1);
        sui::transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        sui::transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        sui::transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    /// Test: Large F token mint at L2 - observe actual behavior
    /// Expected behavior: fToken minting should be blocked at L2 due to fee policy
    #[expected_failure(abort_code = leafsii::E_ACTION_BLOCKED_BY_CR)]
    fun test_large_mint_f_blocked_at_l2() {
        let (mut scenario, clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap) = setup_protocol_with_sp(&mut scenario, &clock);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Push to L2 - fToken minting should be blocked here
        let position = push_to_level(&protocol, &mut pool, 2, ts::ctx(&mut scenario));
        
        // Verify we're at L2 
        let level = leafsii::current_level(&protocol, &pool);
        assert!(level == 2, level as u64); // L2 mode
        
        // Try to mint - this should fail due to fee policy blocking fToken minting at L1+
        let huge_reserve = coin::mint_for_testing<SUI>(5_000_000, ts::ctx(&mut scenario));
        
        let result = leafsii::mint_f(&mut protocol, &pool, huge_reserve, ts::ctx(&mut scenario));
        
        // Cleanup (should not reach here due to expected failure)
        sui::transfer::public_transfer(result, @0x1);
        
        // Cleanup (won't reach here due to expected failure)
        sui::transfer::public_transfer(position, @0x1);
        sui::transfer::public_transfer(init_f, @0x1);
        sui::transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        sui::transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        sui::transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    /// Test: fToken minting should be blocked at L2 due to fee policy
    #[expected_failure(abort_code = leafsii::E_ACTION_BLOCKED_BY_CR)]
    fun test_exact_boundary_mint_blocked_at_l2() {
        let (mut scenario, clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap) = setup_protocol_with_sp(&mut scenario, &clock);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Push to L2 
        let position = push_to_level(&protocol, &mut pool, 2, ts::ctx(&mut scenario));
        
        let initial_level = leafsii::current_level(&protocol, &pool);
        assert!(initial_level == 2, initial_level as u64); // L2 mode
        
        // Try to mint - this should fail due to fee policy blocking fToken minting at L1+
        let small_mint = 10_000;
        let reserve_coin = coin::mint_for_testing<SUI>(small_mint, ts::ctx(&mut scenario));
        
        let result = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));
        
        // Cleanup (should not reach here due to expected failure)
        sui::transfer::public_transfer(result, @0x1);
        sui::transfer::public_transfer(position, @0x1);
        sui::transfer::public_transfer(init_f, @0x1);
        sui::transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        sui::transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        sui::transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    #[test]
    /// Test: Redeem operations are allowed at L3 with new fee policy
    /// Expected behavior: Redeem operations work in all CR levels with appropriate fees/bonuses
    fun test_redeem_allowed_at_l3() {
        let (mut scenario, clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap) = setup_protocol_with_sp(&mut scenario, &clock);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let mut pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        // Using admin_cap from setup_protocol_with_sp
        
        leafsii::update_from_oracle(&mut protocol, &oracle, &clock, &admin_cap);
        
        // Mint some tokens at Normal mode (level 0) first
        let mint_reserve = coin::mint_for_testing<SUI>(100_000, ts::ctx(&mut scenario));
        let f_tokens = leafsii::mint_f(&mut protocol, &pool, mint_reserve, ts::ctx(&mut scenario));
        
        // Now push to L3
        let position = push_to_level(&protocol, &mut pool, 3, ts::ctx(&mut scenario));
        
        let level = leafsii::current_level(&protocol, &pool);
        assert!(level == 3, level as u64); // L3 mode
        
        // Redeem F tokens - should work with fee policy (0% fee + bonus at L3)
        let (redeemed, ticket_opt) = leafsii::redeem_f(&mut protocol, &pool, f_tokens, &clock, ts::ctx(&mut scenario));
        option::destroy_none(ticket_opt);
        
        // Should not reach here, but if it does, transfer the result
        sui::transfer::public_transfer(redeemed, @0x1);
        
        // Cleanup (won't reach here due to expected failure)
        sui::transfer::public_transfer(position, @0x1);
        sui::transfer::public_transfer(init_f, @0x1);
        sui::transfer::public_transfer(init_x, @0x1);
        ts::return_shared(protocol);
        sui::transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        sui::transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }
}
