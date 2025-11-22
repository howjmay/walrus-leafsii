#[test_only]
module leafsii::test_delegate_redeem {
    use sui::clock::{Self, Clock};
    use sui::coin::{Self, Coin};
    use sui::test_scenario::{Self as ts, Scenario};

    use oracle::oracle::{Self, MockOracle};
    use leafsii::leafsii::{Self, Protocol};
    use leafsii::stability_pool::{Self, StabilityPool};
    use leafsii::staking_helper;
    use sui::sui::SUI;

    // Test asset types
    public struct TEST_FTOKEN has drop {}
    public struct TEST_XTOKEN has drop {}

    const INITIAL_PRICE_E6: u64 = 2_000_000_000; // $2.00 in 1e9 scale
    const DEPOSIT_AMOUNT: u64 = 1_000_000_000; // 1 SUI to initialize protocol
    const REDEMPTION_QUEUE_DELAY: u64 = 3 * 24 * 60 * 60 * 1000; // 3 days in ms
    const DEFAULT_DELEGATE_OPERATION_FEE: u64 = 1_000_000_000; // 1 SUI

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

    #[test]
    fun test_redeem_f_delegate_creates_valid_ticket() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);

        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);

        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);

        // First, deplete the buffer by redeeming init F tokens
        let (sui_from_init, init_ticket) = leafsii::redeem_f(&mut protocol, &pool, init_f, &setup_clock, ts::ctx(&mut scenario));
        transfer::public_transfer(sui_from_init, @0x1);
        if (option::is_some(&init_ticket)) {
            transfer::public_transfer(option::destroy_some(init_ticket), @0x1);
        } else {
            option::destroy_none(init_ticket);
        };

        // Add reserves via X to maintain high CR
        let restore_reserve = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario)); // 10 SUI
        let restore_x = leafsii::mint_x(&mut protocol, &pool, restore_reserve, ts::ctx(&mut scenario));
        transfer::public_transfer(restore_x, @0x1);

        // Mint F tokens for redemption
        let reserve_coin = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario)); // 10 SUI
        let minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));

        // Drain most of the buffer to simulate staked reserves
        // This leaves only a small amount in buffer, forcing ticket creation on redemption
        let drain_amount = 18_000_000_000; // Drain 18 SUI, leaving ~3-4 SUI in buffer
        let drained_sui = leafsii::test_drain_buffer(&mut protocol, drain_amount, ts::ctx(&mut scenario));
        transfer::public_transfer(drained_sui, @0x1);

        // Redeem with delegate enabled - buffer was depleted, so ticket should be created
        let _f_amount = coin::value(&minted_f);
        let (sui_coin, mut ticket_opt) = leafsii::redeem_f_delegate(&mut protocol, &pool, minted_f, &setup_clock, ts::ctx(&mut scenario));

        // Verify ticket was created (buffer is insufficient for full redemption)
        assert!(option::is_some(&ticket_opt), 0);
        let ticket = option::extract(&mut ticket_opt);
        option::destroy_none(ticket_opt);

        // Verify ticket properties
        assert!(staking_helper::is_delegate_enabled(&ticket), 1);
        assert!(staking_helper::get_ticket_operation_fee(&ticket) == DEFAULT_DELEGATE_OPERATION_FEE, 2);
        assert!(staking_helper::get_ticket_user(&ticket) == @0x1, 3);
        assert!(staking_helper::get_ticket_amount(&ticket) > 0, 4);

        // Verify expiration timestamp is set to a future time
        let current_time = clock::timestamp_ms(&setup_clock);
        let ticket_expiration = staking_helper::get_ticket_expiration(&ticket);
        assert!(ticket_expiration > current_time, 5);

        // Cleanup
        transfer::public_transfer(sui_coin, @0x1);
        transfer::public_transfer(ticket, @0x1);
        // init_f was already redeemed
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
    fun test_redeem_x_delegate_creates_valid_ticket() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);

        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);

        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);

        // First, deplete the buffer by redeeming init X tokens
        let (sui_from_init, init_ticket) = leafsii::redeem_x(&mut protocol, &pool, init_x, &setup_clock, ts::ctx(&mut scenario));
        transfer::public_transfer(sui_from_init, @0x1);
        if (option::is_some(&init_ticket)) {
            transfer::public_transfer(option::destroy_some(init_ticket), @0x1);
        } else {
            option::destroy_none(init_ticket);
        };

        // Add reserves via X tokens to maintain high CR
        let restore_reserve = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario));
        let restore_x = leafsii::mint_x(&mut protocol, &pool, restore_reserve, ts::ctx(&mut scenario));
        transfer::public_transfer(restore_x, @0x1);

        // Mint X tokens for redemption
        let reserve_coin = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario));
        let minted_x = leafsii::mint_x(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));

        // Drain most of the buffer to simulate staked reserves
        let drain_amount = 18_000_000_000; // Drain 18 SUI, leaving ~3-4 SUI in buffer
        let drained_sui = leafsii::test_drain_buffer(&mut protocol, drain_amount, ts::ctx(&mut scenario));
        transfer::public_transfer(drained_sui, @0x1);

        // Redeem with delegate enabled - buffer was depleted, so ticket should be created
        let _x_amount = coin::value(&minted_x);
        let (sui_coin, mut ticket_opt) = leafsii::redeem_x_delegate(&mut protocol, &pool, minted_x, &setup_clock, ts::ctx(&mut scenario));

        // Verify ticket was created
        assert!(option::is_some(&ticket_opt), 0);
        let ticket = option::extract(&mut ticket_opt);
        option::destroy_none(ticket_opt);

        // Verify ticket has delegation enabled
        assert!(staking_helper::is_delegate_enabled(&ticket), 1);
        assert!(staking_helper::get_ticket_operation_fee(&ticket) == DEFAULT_DELEGATE_OPERATION_FEE, 2);

        // Cleanup
        transfer::public_transfer(sui_coin, @0x1);
        transfer::public_transfer(ticket, @0x1);
        transfer::public_transfer(init_f, @0x1);
        // init_x was already redeemed
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }

    #[test]
    fun test_regular_redeem_creates_non_delegate_ticket() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);

        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);

        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);

        // First, deplete the buffer by redeeming init F tokens
        let (sui_from_init, init_ticket) = leafsii::redeem_f(&mut protocol, &pool, init_f, &setup_clock, ts::ctx(&mut scenario));
        transfer::public_transfer(sui_from_init, @0x1);
        if (option::is_some(&init_ticket)) {
            transfer::public_transfer(option::destroy_some(init_ticket), @0x1);
        } else {
            option::destroy_none(init_ticket);
        };

        // Add reserves via X to maintain high CR
        let restore_reserve = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario));
        let restore_x = leafsii::mint_x(&mut protocol, &pool, restore_reserve, ts::ctx(&mut scenario));
        transfer::public_transfer(restore_x, @0x1);

        // Mint F tokens for redemption
        let reserve_coin = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario));
        let minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));

        // Drain most of the buffer to simulate staked reserves
        let drain_amount = 18_000_000_000; // Drain 18 SUI, leaving ~3-4 SUI in buffer
        let drained_sui = leafsii::test_drain_buffer(&mut protocol, drain_amount, ts::ctx(&mut scenario));
        transfer::public_transfer(drained_sui, @0x1);

        // Redeem without delegate (regular redeem) - buffer was depleted, so ticket should be created
        let (sui_coin, mut ticket_opt) = leafsii::redeem_f(&mut protocol, &pool, minted_f, &setup_clock, ts::ctx(&mut scenario));

        // Verify ticket was created
        assert!(option::is_some(&ticket_opt), 0);
        let ticket = option::extract(&mut ticket_opt);
        option::destroy_none(ticket_opt);

        // Verify ticket does NOT have delegation enabled
        assert!(!staking_helper::is_delegate_enabled(&ticket), 1);
        assert!(staking_helper::get_ticket_operation_fee(&ticket) == 0, 2); // No operation fee

        // Cleanup
        transfer::public_transfer(sui_coin, @0x1);
        transfer::public_transfer(ticket, @0x1);
        // init_f was already redeemed
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
    fun test_ticket_expiration_detection() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);

        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);

        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);

        // Set ticket expiration to 3 days to match test expectations
        leafsii::set_ticket_expiration(&mut protocol, REDEMPTION_QUEUE_DELAY, &admin_cap);

        // First, deplete the buffer by redeeming init F tokens
        let (sui_from_init, init_ticket) = leafsii::redeem_f(&mut protocol, &pool, init_f, &setup_clock, ts::ctx(&mut scenario));
        transfer::public_transfer(sui_from_init, @0x1);
        if (option::is_some(&init_ticket)) {
            transfer::public_transfer(option::destroy_some(init_ticket), @0x1);
        } else {
            option::destroy_none(init_ticket);
        };

        // Add reserves via X to maintain high CR
        let restore_reserve = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario));
        let restore_x = leafsii::mint_x(&mut protocol, &pool, restore_reserve, ts::ctx(&mut scenario));
        transfer::public_transfer(restore_x, @0x1);

        // Mint F tokens for redemption
        let reserve_coin = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario));
        let minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));

        // Drain most of the buffer to simulate staked reserves
        let drain_amount = 18_000_000_000; // Drain 18 SUI, leaving ~3-4 SUI in buffer
        let drained_sui = leafsii::test_drain_buffer(&mut protocol, drain_amount, ts::ctx(&mut scenario));
        transfer::public_transfer(drained_sui, @0x1);

        let (sui_coin, mut ticket_opt) = leafsii::redeem_f_delegate(&mut protocol, &pool, minted_f, &setup_clock, ts::ctx(&mut scenario));
        transfer::public_transfer(sui_coin, @0x1);

        assert!(option::is_some(&ticket_opt), 0);
        let ticket = option::extract(&mut ticket_opt);
        option::destroy_none(ticket_opt);

        // Verify ticket is not expired at creation time
        let current_time = clock::timestamp_ms(&setup_clock);
        assert!(!staking_helper::is_ticket_expired(&ticket, current_time), 1);

        // Verify ticket would be expired after the delay
        let future_time = current_time + REDEMPTION_QUEUE_DELAY + 1000;
        assert!(staking_helper::is_ticket_expired(&ticket, future_time), 2);

        // Cleanup
        transfer::public_transfer(ticket, @0x1);
        // init_f was already redeemed
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
    fun test_admin_can_update_operation_fee() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);

        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);

        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);

        // Update operation fee to 2 SUI
        let new_fee = 2_000_000_000;
        leafsii::set_delegate_operation_fee(&mut protocol, new_fee, &admin_cap);

        // First, deplete the buffer by redeeming init F tokens
        let (sui_from_init, init_ticket) = leafsii::redeem_f(&mut protocol, &pool, init_f, &setup_clock, ts::ctx(&mut scenario));
        transfer::public_transfer(sui_from_init, @0x1);
        if (option::is_some(&init_ticket)) {
            transfer::public_transfer(option::destroy_some(init_ticket), @0x1);
        } else {
            option::destroy_none(init_ticket);
        };

        // Add reserves via X to maintain high CR
        let restore_reserve = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario));
        let restore_x = leafsii::mint_x(&mut protocol, &pool, restore_reserve, ts::ctx(&mut scenario));
        transfer::public_transfer(restore_x, @0x1);

        // Mint F tokens for redemption
        let reserve_coin = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario));
        let minted_f = leafsii::mint_f(&mut protocol, &pool, reserve_coin, ts::ctx(&mut scenario));

        // Drain most of the buffer to simulate staked reserves
        let drain_amount = 18_000_000_000; // Drain 18 SUI, leaving ~3-4 SUI in buffer
        let drained_sui = leafsii::test_drain_buffer(&mut protocol, drain_amount, ts::ctx(&mut scenario));
        transfer::public_transfer(drained_sui, @0x1);

        let (sui_coin, mut ticket_opt) = leafsii::redeem_f_delegate(&mut protocol, &pool, minted_f, &setup_clock, ts::ctx(&mut scenario));

        assert!(option::is_some(&ticket_opt), 0);
        let ticket = option::extract(&mut ticket_opt);
        option::destroy_none(ticket_opt);

        // Verify ticket uses new operation fee
        assert!(staking_helper::get_ticket_operation_fee(&ticket) == new_fee, 1);

        // Cleanup
        transfer::public_transfer(sui_coin, @0x1);
        transfer::public_transfer(ticket, @0x1);
        // init_f was already redeemed
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
    fun test_both_redeem_types_in_one_session() {
        let (mut scenario, test_clock, oracle) = setup_test();
        let (init_f, init_x, admin_cap, setup_clock) = setup_protocol_with_sp(&mut scenario);

        ts::next_tx(&mut scenario, @0x1);
        let mut protocol = ts::take_shared<Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);
        let pool = ts::take_shared<StabilityPool<TEST_FTOKEN>>(&scenario);

        leafsii::update_from_oracle(&mut protocol, &oracle, &setup_clock, &admin_cap);

        // First, deplete the buffer by redeeming both init F and X tokens
        let (sui_from_init_f, init_ticket_f) = leafsii::redeem_f(&mut protocol, &pool, init_f, &setup_clock, ts::ctx(&mut scenario));
        transfer::public_transfer(sui_from_init_f, @0x1);
        if (option::is_some(&init_ticket_f)) {
            transfer::public_transfer(option::destroy_some(init_ticket_f), @0x1);
        } else {
            option::destroy_none(init_ticket_f);
        };

        let (sui_from_init_x, init_ticket_x) = leafsii::redeem_x(&mut protocol, &pool, init_x, &setup_clock, ts::ctx(&mut scenario));
        transfer::public_transfer(sui_from_init_x, @0x1);
        if (option::is_some(&init_ticket_x)) {
            transfer::public_transfer(option::destroy_some(init_ticket_x), @0x1);
        } else {
            option::destroy_none(init_ticket_x);
        };

        // Add reserves via X to maintain high CR
        let restore_reserve = coin::mint_for_testing<SUI>(10_000_000_000, ts::ctx(&mut scenario));
        let restore_x = leafsii::mint_x(&mut protocol, &pool, restore_reserve, ts::ctx(&mut scenario));
        transfer::public_transfer(restore_x, @0x1);

        // Mint F tokens for two redemptions
        let reserve_coin1 = coin::mint_for_testing<SUI>(5_000_000_000, ts::ctx(&mut scenario)); // 5 SUI
        let minted_f1 = leafsii::mint_f(&mut protocol, &pool, reserve_coin1, ts::ctx(&mut scenario));

        let reserve_coin2 = coin::mint_for_testing<SUI>(5_000_000_000, ts::ctx(&mut scenario)); // 5 SUI
        let minted_f2 = leafsii::mint_f(&mut protocol, &pool, reserve_coin2, ts::ctx(&mut scenario));

        // Drain most of the buffer to simulate staked reserves
        // This leaves only a small amount in buffer, forcing ticket creation on both redemptions
        let drain_amount = 18_000_000_000; // Drain 18 SUI, leaving ~3-4 SUI in buffer
        let drained_sui = leafsii::test_drain_buffer(&mut protocol, drain_amount, ts::ctx(&mut scenario));
        transfer::public_transfer(drained_sui, @0x1);

        // Create one self-redeem ticket
        let (sui_coin1, mut ticket_opt1) = leafsii::redeem_f(&mut protocol, &pool, minted_f1, &setup_clock, ts::ctx(&mut scenario));

        // Create one delegate-redeem ticket
        let (sui_coin2, mut ticket_opt2) = leafsii::redeem_f_delegate(&mut protocol, &pool, minted_f2, &setup_clock, ts::ctx(&mut scenario));

        // Verify both tickets were created
        assert!(option::is_some(&ticket_opt1), 0);
        assert!(option::is_some(&ticket_opt2), 1);

        let ticket1 = option::extract(&mut ticket_opt1);
        let ticket2 = option::extract(&mut ticket_opt2);
        option::destroy_none(ticket_opt1);
        option::destroy_none(ticket_opt2);

        // Verify ticket1 is NOT delegate enabled
        assert!(!staking_helper::is_delegate_enabled(&ticket1), 2);
        assert!(staking_helper::get_ticket_operation_fee(&ticket1) == 0, 3);

        // Verify ticket2 IS delegate enabled
        assert!(staking_helper::is_delegate_enabled(&ticket2), 4);
        assert!(staking_helper::get_ticket_operation_fee(&ticket2) == DEFAULT_DELEGATE_OPERATION_FEE, 5);

        // Cleanup
        transfer::public_transfer(sui_coin1, @0x1);
        transfer::public_transfer(sui_coin2, @0x1);
        transfer::public_transfer(ticket1, @0x1);
        transfer::public_transfer(ticket2, @0x1);
        // init_f and init_x were already redeemed
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, @0x1);
        ts::return_shared(pool);
        transfer::public_transfer(oracle, @0x1);
        clock::destroy_for_testing(test_clock);
        clock::destroy_for_testing(setup_clock);
        ts::end(scenario);
    }
}
