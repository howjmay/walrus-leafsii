#[test_only]
module leafsii::test_unauthorized_pool {
    use sui::clock::{Self, Clock};
    use sui::coin::{Self};
    use sui::test_scenario::{Self as ts, Scenario};
    use sui::sui::SUI;
    use sui_system::staking_pool::FungibleStakedSui;

    use oracle::oracle::{Self, MockOracle};
    use leafsii::leafsii::{Self, Protocol};
    use leafsii::stability_pool::{Self, StabilityPool};

    // Test asset types
    public struct TEST_FTOKEN has drop {}
    public struct TEST_XTOKEN has drop {}

    const INITIAL_PRICE_E6: u64 = 2_000_000; // $2.00
    const DEPOSIT_AMOUNT: u64 = 1_000_000; // 1 SUI

    fun setup_test(): (Scenario, Clock, MockOracle<SUI>) {
        let mut scenario = ts::begin(@0x1);
        let ctx = ts::ctx(&mut scenario);

        let clock = clock::create_for_testing(ctx);
        let oracle = oracle::create_mock_oracle<SUI>(INITIAL_PRICE_E6, &clock, ctx);
        
        (scenario, clock, oracle)
    }

    #[test]
    #[expected_failure(abort_code = 8, location = leafsii)]
    fun test_unauthorized_pool_mint_f_fails() {
        let (mut scenario, clock, _oracle) = setup_test();
        let ctx = ts::ctx(&mut scenario);
        
        // Create authorized pool first
        let _sp_cap = stability_pool::create_stability_pool<TEST_FTOKEN>(ctx);
        
        ts::next_tx(&mut scenario, @0x1);
        let mut authorized_pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);
        
        // Create protocol with authorized pool
        let stable_treasury_cap = coin::create_treasury_cap_for_testing<TEST_FTOKEN>(ts::ctx(&mut scenario));
        let leverage_treasury_cap = coin::create_treasury_cap_for_testing<TEST_XTOKEN>(ts::ctx(&mut scenario));
        let reserve_coin = coin::mint_for_testing<SUI>(DEPOSIT_AMOUNT, ts::ctx(&mut scenario));
        // Bind pool to protocol
        let protocol_id = sui::object::id_from_address(@0x123);

        let (_coin_f, _coin_x, _admin_cap) = leafsii::init_protocol<TEST_FTOKEN, TEST_XTOKEN>(
            stable_treasury_cap,
            leverage_treasury_cap,
            INITIAL_PRICE_E6,
            reserve_coin,
            &mut authorized_pool,
            _sp_cap,
            &clock,
            ts::ctx(&mut scenario)
        );

        // Return authorized pool
        ts::return_shared(authorized_pool);

        ts::next_tx(&mut scenario, @0x1);

        // Create UNAUTHORIZED pool - different pool instance
        let _sp_cap2 = stability_pool::create_stability_pool<TEST_FTOKEN>(ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let unauthorized_pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);

        // Try to mint with unauthorized pool - should fail with E_UNAUTHORIZED_POOL
        let reserve_in = coin::mint_for_testing<SUI>(1000, ts::ctx(&mut scenario));
        let _f_coin = leafsii::mint_f(
            &mut protocol,
            &unauthorized_pool,
            reserve_in,
            ts::ctx(&mut scenario)
        );
        
        // This line should never be reached due to expected failure
        abort 99
    }
}