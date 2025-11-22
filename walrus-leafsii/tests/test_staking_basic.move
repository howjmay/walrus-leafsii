/// Basic unit tests for staking helper functions.
///
/// This module contains fundamental tests for the staking infrastructure:
///
/// - **test_basic_staking_helper_functions()**: Tests core staking helper utilities
///   - PoolStake creation and destruction
///   - Buffer calculation logic
///   - Total reserve calculation
///   - Stake conversion conditions
///
/// - **test_redemption_ticket()**: Tests redemption ticket management
///   - Ticket creation with user and amount
///   - Getter functions for ticket properties
///   - Proper cleanup and destruction
#[test_only]
module leafsii::test_staking_basic {
    use sui::test_scenario as ts;

    use leafsii::staking_helper;

    #[test]
    fun test_basic_staking_helper_functions() {
        let mut scenario = ts::begin(@0x1);

        ts::next_tx(&mut scenario, @0x1);

        // Test new_pool_stake
        let stake = staking_helper::new_pool_stake(ts::ctx(&mut scenario));

        // Test basic getters
        assert!(staking_helper::get_total_staked_amount(&stake) == 0, 0);
        assert!(staking_helper::get_active_fss_amount(&stake) == 0, 1);
        assert!(staking_helper::get_pending_stakes_count(&stake) == 0, 2);
        assert!(!staking_helper::has_active_fss(&stake), 3);

        // Test buffer calculation
        let stake_amount = staking_helper::calculate_stake_amount(
            1000, // current_buffer
            500,  // incoming_amount
            1000, // target_buffer_bps (10%)
            10000 // total_reserve
        );
        // Target = 10% of 10000 = 1000
        // New buffer = 1000 + 500 = 1500
        // Stake = 1500 - 1000 = 500
        assert!(stake_amount == 500, 4);

        // Test total reserve calculation
        let total = staking_helper::get_total_reserve(2000, &stake);
        assert!(total == 2000, 5);

        // Test can_convert_stake
        assert!(staking_helper::can_convert_stake(10, 15), 6);
        assert!(!staking_helper::can_convert_stake(20, 15), 7);

        // Clean up
        staking_helper::destroy_pool_stake_if_empty(stake);

        ts::end(scenario);
    }

    #[test]
    fun test_redemption_ticket() {
        let mut scenario = ts::begin(@0x1);

        ts::next_tx(&mut scenario, @0x1);

        // Create ticket with 7 day expiration (604800000 ms)
        let expiration_time = 604800000;
        let ticket = staking_helper::new_redemption_ticket(
            @0x1,
            1000,
            expiration_time,
            0, // operation_fee (self-redeem, no fee)
            false, // delegate_enabled (self-redeem)
            ts::ctx(&mut scenario)
        );

        // Test getters
        assert!(staking_helper::get_ticket_user(&ticket) == @0x1, 0);
        assert!(staking_helper::get_ticket_amount(&ticket) == 1000, 1);
        assert!(staking_helper::get_ticket_expiration(&ticket) == expiration_time, 2);

        // Test expiration check (not expired yet at time 0)
        assert!(!staking_helper::is_ticket_expired(&ticket, 0), 3);
        // Test expiration check (expired after expiration time)
        assert!(staking_helper::is_ticket_expired(&ticket, expiration_time + 1), 4);

        // Test destruction
        let (user, amount, exp) = staking_helper::destroy_ticket(ticket);
        assert!(user == @0x1, 5);
        assert!(amount == 1000, 6);
        assert!(exp == expiration_time, 7);

        ts::end(scenario);
    }
}