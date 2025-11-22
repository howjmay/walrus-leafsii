#[test_only]
module leafsii::simple_test {
    use sui::coin;
    use sui::clock;
    use sui::test_scenario as ts;
    use sui::sui::SUI;
    use leafsii::leafsii;
    use leafsii::stability_pool;
    use sui_system::staking_pool::FungibleStakedSui;

    public struct TEST_FTOKEN has drop {}
    public struct TEST_XTOKEN has drop {}

    const INITIAL_PRICE_E6: u64 = 2_000_000;
    const INIT_RESERVE: u64 = 1_000_000; // 1.0 R

    #[test]
    fun simple_init_test() {
        let mut scenario = ts::begin(@0x1);
        let ctx = ts::ctx(&mut scenario);
        
        let clock = clock::create_for_testing(ctx);
        
        // Create stability pool first
        let sp_cap = stability_pool::create_stability_pool<TEST_FTOKEN>(ctx);

        ts::next_tx(&mut scenario, @0x1);
        let mut pool = ts::take_shared<stability_pool::StabilityPool<TEST_FTOKEN>>(&scenario);

        // Bind pool to protocol
        let protocol_id = sui::object::id_from_address(@0x123);
        
        // Create protocol with pool reference
        let coin_r = coin::mint_for_testing<SUI>(INIT_RESERVE, ts::ctx(&mut scenario));
        let stable_treasury_cap = coin::create_treasury_cap_for_testing<TEST_FTOKEN>(ts::ctx(&mut scenario));
        let leverage_treasury_cap = coin::create_treasury_cap_for_testing<TEST_XTOKEN>(ts::ctx(&mut scenario));
        
        let (_f_coin, _x_coin, admin_cap) = leafsii::init_protocol<TEST_FTOKEN, TEST_XTOKEN>(
            stable_treasury_cap,
            leverage_treasury_cap,
            INITIAL_PRICE_E6,
            coin_r,
            &mut pool,
            sp_cap,
            &clock,
            ts::ctx(&mut scenario)
        );

        ts::return_shared(pool);

        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<leafsii::Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);

        // Basic state check
        let (nf, nx, pf, px, price, reserve, fees, allowed) = leafsii::get_protocol_state<TEST_FTOKEN, TEST_XTOKEN>(&protocol);
        // 50/50 split at init: $1.0 -> 0.001 F (at nano-USD scale), $1.0 -> 0.5 X at $2.0
        assert!(nf == 1_000, 1); // Changed from 1_000_000 due to PF_FIXED = 1e9
        assert!(nx == 500_000, 2);
        assert!(pf == 1_000_000_000, 3); // Changed from 1_000_000, now in nano-USD (1e9)
        assert!(px == INITIAL_PRICE_E6, 4);
        assert!(price == INITIAL_PRICE_E6, 5);
        assert!(reserve == INIT_RESERVE, 6);
        assert!(fees == 0, 7);
        assert!(allowed == true, 8);

        // Test admin functions
        leafsii::set_user_actions_allowed<TEST_FTOKEN, TEST_XTOKEN>(&mut protocol, false, &admin_cap);
        let (_, _, _, _, _, _, _, allowed_after) = leafsii::get_protocol_state<TEST_FTOKEN, TEST_XTOKEN>(&protocol);
        assert!(allowed_after == false, 9);

        // Cleanup - return shared objects and transfer owned objects
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        transfer::public_transfer(_f_coin, @0x1);
        transfer::public_transfer(_x_coin, @0x1);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }
}
