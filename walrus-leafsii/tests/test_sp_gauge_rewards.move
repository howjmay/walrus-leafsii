#[test_only]
module leafsii::test_sp_gauge_rewards {
    use sui::clock::{Self, Clock};
    use sui::test_scenario::{Self as ts, Scenario};
    use sui::sui::SUI;
    use sui_system::staking_pool::FungibleStakedSui;
    use sui::coin::{Self, Coin};
    use std::debug;

    use oracle::oracle::{Self};
    use leafsii::leafsii::{Self};
    use leafsii::stability_pool::{Self};
    use leafsii::lfs_token::{Self};
    use leafsii::ve_lfs::{Self, Lock};
    use leafsii::sp_gauge::{Self};
    use leafsii::vesting_escrow::{Self};

    // Test asset types
    public struct TEST_FTOKEN has drop {}
    public struct TEST_XTOKEN has drop {}

    const ALICE: address = @0xA11CE;
    const BOB: address = @0xB0B;

    const INITIAL_PRICE_E6: u64 = 2_000_000; // $2.00
    const DEPOSIT_AMOUNT: u64 = 1000_000_000; // 1000 units
    const LFS_REWARD_AMOUNT: u64 = 1000_000_000; // 1000 LFS rewards
    const FOUR_YEARS_MS: u64 = 4 * 365 * 24 * 3600 * 1000;

    fun setup_protocol_and_sp(scenario: &mut Scenario, clock: &Clock): (leafsii::AdminCap, Coin<TEST_FTOKEN>, Coin<TEST_XTOKEN>) {
        let ctx = ts::ctx(scenario);

        // Create stability pool first
        let protocol_id = sui::object::id_from_address(@0x123);
        let sp_cap = stability_pool::create_stability_pool<TEST_FTOKEN>(ctx);

        ts::next_tx(scenario, ALICE);
        let mut pool = ts::take_shared<stability_pool::StabilityPool<TEST_FTOKEN>>(scenario);

        // Create protocol with pool reference
        let stable_treasury_cap = coin::create_treasury_cap_for_testing<TEST_FTOKEN>(ts::ctx(scenario));
        let leverage_treasury_cap = coin::create_treasury_cap_for_testing<TEST_XTOKEN>(ts::ctx(scenario));
        let reserve_coin = coin::mint_for_testing<SUI>(DEPOSIT_AMOUNT, ts::ctx(scenario));

        // Bind pool to protocol

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
        (admin_cap, f_coin, x_coin)
    }

    // Test basic SP gauge creation and user checkpointing
    #[test]
    fun test_sp_gauge_basic() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));
        let oracle = oracle::create_mock_oracle<SUI>(INITIAL_PRICE_E6, &clock, ts::ctx(&mut scenario));

        // Initialize LFS and ve-LFS
        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));

        // Setup protocol and stability pool
        let (admin_cap, mut f_tokens, x_tokens) = setup_protocol_and_sp(&mut scenario, &clock);

        ts::next_tx(&mut scenario, ALICE);
        let mut sp = ts::take_shared<stability_pool::StabilityPool<TEST_FTOKEN>>(&scenario);
        let mut protocol = ts::take_shared<leafsii::Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);

        // Create SP gauge
        let mut sp_gauge = sp_gauge::create_sp_gauge(&sp, ts::ctx(&mut scenario));

        // Mint more f_tokens if needed for testing
        let reserve_coin = coin::mint_for_testing<SUI>(DEPOSIT_AMOUNT, ts::ctx(&mut scenario));
        let additional_f = leafsii::mint_f(&mut protocol, &sp, reserve_coin, ts::ctx(&mut scenario));
        coin::join(&mut f_tokens, additional_f);

        let mut sp_position = stability_pool::create_position<TEST_FTOKEN>(ts::ctx(&mut scenario));
        stability_pool::deposit_f(&mut sp, &mut sp_position, f_tokens, ts::ctx(&mut scenario));

        // Checkpoint user in gauge (no ve-lock initially)
        let no_lock_1: option::Option<Lock> = option::none();
        sp_gauge::checkpoint_user(
            &mut sp_gauge,
            &sp,
            &sp_position,
            &no_lock_1,
            &clock,
            ts::ctx(&mut scenario)
        );
        option::destroy_none(no_lock_1);

        // Check user info
        let (stake, working, claimable) = sp_gauge::get_user_info(&sp_gauge, ALICE);
        assert!(stake > 0, 0); // Should have some stake from fToken deposit
        assert!(working > 0, 1); // Should have working balance
        assert!(claimable == 0, 2); // No rewards yet

        // Working balance should be 40% of stake (no ve boost)
        let expected_working = stake * 4000 / 10000; // 40%
        assert!(working == expected_working, 3);

        debug::print(&stake);
        debug::print(&working);

        // Cleanup
        transfer::public_transfer(sp_position, ALICE);
        transfer::public_transfer(x_tokens, ALICE);
        transfer::public_transfer(sp_gauge, ALICE);
        ts::return_shared(sp);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        transfer::public_transfer(oracle, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test rewards distribution without ve-boost
    #[test]
    fun test_rewards_no_boost() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));
        let oracle = oracle::create_mock_oracle<SUI>(INITIAL_PRICE_E6, &clock, ts::ctx(&mut scenario));

        // Initialize LFS and ve-LFS
        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));

        // Setup protocol and stability pool
        let (admin_cap, mut f_tokens_alice, x_tokens_alice) = setup_protocol_and_sp(&mut scenario, &clock);

        ts::next_tx(&mut scenario, ALICE);
        let mut sp = ts::take_shared<stability_pool::StabilityPool<TEST_FTOKEN>>(&scenario);
        let mut protocol = ts::take_shared<leafsii::Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);

        let mut sp_gauge = sp_gauge::create_sp_gauge(&sp, ts::ctx(&mut scenario));

        // Alice deposits
        let reserve_coin_alice = coin::mint_for_testing<SUI>(DEPOSIT_AMOUNT, ts::ctx(&mut scenario));
        let additional_f_alice = leafsii::mint_f(&mut protocol, &sp, reserve_coin_alice, ts::ctx(&mut scenario));
        coin::join(&mut f_tokens_alice, additional_f_alice);
        let mut sp_position_alice = stability_pool::create_position<TEST_FTOKEN>(ts::ctx(&mut scenario));
        stability_pool::deposit_f(&mut sp, &mut sp_position_alice, f_tokens_alice, ts::ctx(&mut scenario));

        let no_lock: option::Option<Lock> = option::none();
        sp_gauge::checkpoint_user(&mut sp_gauge, &sp, &sp_position_alice, &no_lock, &clock, ts::ctx(&mut scenario));
        option::destroy_none(no_lock);

        ts::next_tx(&mut scenario, BOB);

        // Bob deposits same amount
        let reserve_coin_bob = coin::mint_for_testing<SUI>(DEPOSIT_AMOUNT, ts::ctx(&mut scenario));
        let f_tokens_bob = leafsii::mint_f(&mut protocol, &sp, reserve_coin_bob, ts::ctx(&mut scenario));
        let mut sp_position_bob = stability_pool::create_position<TEST_FTOKEN>(ts::ctx(&mut scenario));
        stability_pool::deposit_f(&mut sp, &mut sp_position_bob, f_tokens_bob, ts::ctx(&mut scenario));

        let no_lock_bob2: option::Option<Lock> = option::none();
        sp_gauge::checkpoint_user(&mut sp_gauge, &sp, &sp_position_bob, &no_lock_bob2, &clock, ts::ctx(&mut scenario));
        option::destroy_none(no_lock_bob2);

        // Notify rewards
        let reward_coin = lfs_token::mint_emissions(&mut emissions_cap, LFS_REWARD_AMOUNT, &mut treasury_cap, ts::ctx(&mut scenario));
        sp_gauge::notify_reward(&mut sp_gauge, reward_coin, 1, ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ALICE);

        // Alice claims rewards
        let mut vesting_opt = sp_gauge::claim(&mut sp_gauge, &clock, ts::ctx(&mut scenario));
        assert!(option::is_some(&vesting_opt), 0);

        let vesting = option::extract(&mut vesting_opt);
        option::destroy_none(vesting_opt);
        let (_, _, _, total_amount, claimed_amount, _) = vesting_escrow::get_vesting_info(&vesting);

        // Alice should get approximately half the rewards (50% due to equal stakes)
        let expected_alice_reward = LFS_REWARD_AMOUNT / 2;
        assert!(total_amount >= expected_alice_reward * 9 / 10, 1); // Allow 10% variance
        assert!(claimed_amount == 0, 2); // Nothing claimed yet

        debug::print(&total_amount);
        debug::print(&expected_alice_reward);

        transfer::public_transfer(vesting, ALICE);
        transfer::public_transfer(sp_position_alice, ALICE);
        transfer::public_transfer(sp_position_bob, BOB);
        transfer::public_transfer(x_tokens_alice, ALICE);
        transfer::public_transfer(sp_gauge, ALICE);
        ts::return_shared(sp);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        transfer::public_transfer(oracle, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test rewards with ve-boost
    #[test]
    fun test_rewards_with_boost() {
        let mut scenario = ts::begin(ALICE);
        let clock = clock::create_for_testing(ts::ctx(&mut scenario));
        let oracle = oracle::create_mock_oracle<SUI>(INITIAL_PRICE_E6, &clock, ts::ctx(&mut scenario));

        // Initialize LFS and ve-LFS
        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));
        ve_lfs::init_for_testing(ts::ctx(&mut scenario));

        // Setup protocol and stability pool
        let (admin_cap, f_tokens_init, x_tokens_alice) = setup_protocol_and_sp(&mut scenario, &clock);

        ts::next_tx(&mut scenario, ALICE);
        let mut sp = ts::take_shared<stability_pool::StabilityPool<TEST_FTOKEN>>(&scenario);
        let mut protocol = ts::take_shared<leafsii::Protocol<TEST_FTOKEN, TEST_XTOKEN>>(&scenario);

        let mut sp_gauge = sp_gauge::create_sp_gauge(&sp, ts::ctx(&mut scenario));

        // Alice creates a max ve-lock
        let lfs_for_lock = coin::mint(&mut treasury_cap, DEPOSIT_AMOUNT, ts::ctx(&mut scenario));
        let alice_lock = ve_lfs::create_lock(lfs_for_lock, FOUR_YEARS_MS, &clock, ts::ctx(&mut scenario));

        // Alice deposits to SP
        let reserve_coin_alice = coin::mint_for_testing<SUI>(DEPOSIT_AMOUNT, ts::ctx(&mut scenario));
        let f_tokens_alice = leafsii::mint_f(&mut protocol, &sp, reserve_coin_alice, ts::ctx(&mut scenario));
        let mut sp_position_alice = stability_pool::create_position<TEST_FTOKEN>(ts::ctx(&mut scenario));
        stability_pool::deposit_f(&mut sp, &mut sp_position_alice, f_tokens_alice, ts::ctx(&mut scenario));

        // Checkpoint Alice with ve-boost
        let mut alice_lock_opt = option::some(alice_lock);
        sp_gauge::checkpoint_user(&mut sp_gauge, &sp, &sp_position_alice, &alice_lock_opt, &clock, ts::ctx(&mut scenario));
        let alice_lock = option::extract(&mut alice_lock_opt);
        option::destroy_none(alice_lock_opt);

        ts::next_tx(&mut scenario, BOB);

        // Bob deposits same amount but no ve-lock
        let reserve_coin_bob = coin::mint_for_testing<SUI>(DEPOSIT_AMOUNT, ts::ctx(&mut scenario));
        let f_tokens_bob = leafsii::mint_f(&mut protocol, &sp, reserve_coin_bob, ts::ctx(&mut scenario));
        let mut sp_position_bob = stability_pool::create_position<TEST_FTOKEN>(ts::ctx(&mut scenario));
        stability_pool::deposit_f(&mut sp, &mut sp_position_bob, f_tokens_bob, ts::ctx(&mut scenario));

        let no_lock_bob3: option::Option<Lock> = option::none();
        sp_gauge::checkpoint_user(&mut sp_gauge, &sp, &sp_position_bob, &no_lock_bob3, &clock, ts::ctx(&mut scenario));
        option::destroy_none(no_lock_bob3);

        // Check working balances
        let (alice_stake, alice_working, _) = sp_gauge::get_user_info(&sp_gauge, ALICE);
        let (bob_stake, bob_working, _) = sp_gauge::get_user_info(&sp_gauge, BOB);

        // Debug output
        std::debug::print(&b"Alice stake:");
        std::debug::print(&alice_stake);
        std::debug::print(&b"Alice working:");
        std::debug::print(&alice_working);
        std::debug::print(&b"Bob stake:");
        std::debug::print(&bob_stake);
        std::debug::print(&b"Bob working:");
        std::debug::print(&bob_working);

        // Alice should have higher working balance due to boost
        assert!(alice_working > bob_working, 0);
        assert!(alice_stake == bob_stake, 1); // Same stakes

        // Notify rewards
        let reward_coin = lfs_token::mint_emissions(&mut emissions_cap, LFS_REWARD_AMOUNT, &mut treasury_cap, ts::ctx(&mut scenario));
        sp_gauge::notify_reward(&mut sp_gauge, reward_coin, 1, ts::ctx(&mut scenario));

        ts::next_tx(&mut scenario, ALICE);

        // Alice claims - should get more than 50% due to boost
        let mut vesting_alice_opt = sp_gauge::claim(&mut sp_gauge, &clock, ts::ctx(&mut scenario));
        assert!(option::is_some(&vesting_alice_opt), 2);

        let vesting_alice = option::extract(&mut vesting_alice_opt);
        option::destroy_none(vesting_alice_opt);
        let (_, _, _, alice_reward, _, _) = vesting_escrow::get_vesting_info(&vesting_alice);

        ts::next_tx(&mut scenario, BOB);

        // Bob claims - should get less than 50%
        let mut vesting_bob_opt = sp_gauge::claim(&mut sp_gauge, &clock, ts::ctx(&mut scenario));
        assert!(option::is_some(&vesting_bob_opt), 3);

        let vesting_bob = option::extract(&mut vesting_bob_opt);
        option::destroy_none(vesting_bob_opt);
        let (_, _, _, bob_reward, _, _) = vesting_escrow::get_vesting_info(&vesting_bob);

        // Alice should get more rewards due to boost
        assert!(alice_reward > bob_reward, 4);
        assert!(alice_reward > LFS_REWARD_AMOUNT / 2, 5); // More than 50%

        debug::print(&alice_stake);
        debug::print(&alice_working);
        debug::print(&bob_stake);
        debug::print(&bob_working);
        debug::print(&alice_reward);
        debug::print(&bob_reward);

        transfer::public_transfer(alice_lock, ALICE);
        transfer::public_transfer(vesting_alice, ALICE);
        transfer::public_transfer(vesting_bob, BOB);
        transfer::public_transfer(sp_position_alice, ALICE);
        transfer::public_transfer(sp_position_bob, BOB);
        transfer::public_transfer(x_tokens_alice, ALICE);
        transfer::public_transfer(f_tokens_init, ALICE);
        transfer::public_transfer(sp_gauge, ALICE);
        ts::return_shared(sp);
        ts::return_shared(protocol);
        transfer::public_transfer(admin_cap, ALICE);
        transfer::public_transfer(treasury_cap, ALICE);
        transfer::public_transfer(emissions_cap, ALICE);
        transfer::public_transfer(oracle, ALICE);
        clock::destroy_for_testing(clock);
        ts::end(scenario);
    }

    // Test working balance calculation
    #[test]
    fun test_working_balance_calculation() {
        // Test the boost math directly
        let stake = 1000;
        let total_stake = 2000;
        let user_ve = 500;
        let total_ve = 1000;

        let working = sp_gauge::test_working_balance_calculation(stake, total_stake, user_ve, total_ve);

        // Base: stake * 40% = 1000 * 0.4 = 400
        // Boost: total_stake * 60% * user_ve / total_ve = 2000 * 0.6 * 500 / 1000 = 600
        // Total: 400 + 600 = 1000
        // But capped at stake, so should be 1000
        assert!(working == 1000, 0);

        // Test with no ve
        let working_no_ve = sp_gauge::test_working_balance_calculation(stake, total_stake, 0, total_ve);
        assert!(working_no_ve == 400, 1); // 40% of stake

        debug::print(&working);
        debug::print(&working_no_ve);
    }
}