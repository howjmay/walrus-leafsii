#[test_only]
module leafsii::test_lfs_token {
    use sui::test_scenario::{Self as ts};
    use sui::coin::{Self};

    use leafsii::lfs_token::{Self, LFS};

    const ADMIN: address = @0xAD;
    const ALICE: address = @0xA11CE;

    const MINT_AMOUNT: u64 = 1000_000_000; // 1000 LFS
    const TOTAL_SUPPLY_CAP: u64 = 2_000_000_000_000_000; // 2M LFS

    // Test initialization
    #[test]
    fun test_init_lfs() {
        let mut scenario = ts::begin(ADMIN);

        let (treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Verify initial state
        let total_supply = lfs_token::total_supply(&treasury_cap);
        assert!(total_supply == 0, 0);

        let emissions_minted = lfs_token::emissions_minted_to_date(&emissions_cap);
        assert!(emissions_minted == 0, 1);

        // Verify cap constant
        let cap = lfs_token::total_supply_cap();
        assert!(cap == TOTAL_SUPPLY_CAP, 2);

        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test basic emissions minting
    #[test]
    fun test_mint_emissions() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Mint emissions
        let lfs_coin = lfs_token::mint_emissions(
            &mut emissions_cap,
            MINT_AMOUNT,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        assert!(coin::value(&lfs_coin) == MINT_AMOUNT, 0);

        // Check emissions tracking
        let emissions_minted = lfs_token::emissions_minted_to_date(&emissions_cap);
        assert!(emissions_minted == MINT_AMOUNT, 1);

        // Check total supply
        let total_supply = lfs_token::total_supply(&treasury_cap);
        assert!(total_supply == MINT_AMOUNT, 2);

        transfer::public_transfer(lfs_coin, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test multiple emissions minting
    #[test]
    fun test_multiple_emissions() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // First mint
        let lfs_1 = lfs_token::mint_emissions(
            &mut emissions_cap,
            MINT_AMOUNT,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        // Second mint
        let lfs_2 = lfs_token::mint_emissions(
            &mut emissions_cap,
            MINT_AMOUNT * 2,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        // Third mint
        let lfs_3 = lfs_token::mint_emissions(
            &mut emissions_cap,
            MINT_AMOUNT / 2,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        // Check total emissions
        let total_emissions = lfs_token::emissions_minted_to_date(&emissions_cap);
        let expected_total = MINT_AMOUNT + (MINT_AMOUNT * 2) + (MINT_AMOUNT / 2);
        assert!(total_emissions == expected_total, 0);

        // Check total supply
        let total_supply = lfs_token::total_supply(&treasury_cap);
        assert!(total_supply == expected_total, 1);

        transfer::public_transfer(lfs_1, ALICE);
        transfer::public_transfer(lfs_2, ALICE);
        transfer::public_transfer(lfs_3, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test minting zero amount fails
    #[test]
    #[expected_failure(abort_code = 1)] // E_INVALID_AMOUNT
    fun test_mint_zero_amount() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Try to mint zero
        let lfs_coin = lfs_token::mint_emissions(
            &mut emissions_cap,
            0,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        transfer::public_transfer(lfs_coin, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test supply cap enforcement
    #[test]
    #[expected_failure(abort_code = 3)] // E_TOTAL_SUPPLY_CAP_EXCEEDED
    fun test_supply_cap_exceeded() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Try to mint more than the cap
        let lfs_coin = lfs_token::mint_emissions(
            &mut emissions_cap,
            TOTAL_SUPPLY_CAP + 1,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        transfer::public_transfer(lfs_coin, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test approaching supply cap gradually
    #[test]
    fun test_approach_supply_cap() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Mint 90% of cap
        let amount_90_percent = TOTAL_SUPPLY_CAP * 9 / 10;
        let lfs_1 = lfs_token::mint_emissions(
            &mut emissions_cap,
            amount_90_percent,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        // Check we're at 90%
        let supply_after_90 = lfs_token::total_supply(&treasury_cap);
        assert!(supply_after_90 == amount_90_percent, 0);

        // Mint another 9% (should succeed)
        let amount_9_percent = TOTAL_SUPPLY_CAP * 9 / 100;
        let lfs_2 = lfs_token::mint_emissions(
            &mut emissions_cap,
            amount_9_percent,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        // Now at 99%
        let supply_after_99 = lfs_token::total_supply(&treasury_cap);
        assert!(supply_after_99 == amount_90_percent + amount_9_percent, 1);

        // Mint the remaining 1% (should succeed)
        let remaining = TOTAL_SUPPLY_CAP - supply_after_99;
        let lfs_3 = lfs_token::mint_emissions(
            &mut emissions_cap,
            remaining,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        // Should now be at exactly the cap
        let final_supply = lfs_token::total_supply(&treasury_cap);
        assert!(final_supply == TOTAL_SUPPLY_CAP, 2);

        // Emissions tracking should match
        let total_emissions = lfs_token::emissions_minted_to_date(&emissions_cap);
        assert!(total_emissions == TOTAL_SUPPLY_CAP, 3);

        transfer::public_transfer(lfs_1, ALICE);
        transfer::public_transfer(lfs_2, ALICE);
        transfer::public_transfer(lfs_3, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test minting exactly at cap
    #[test]
    fun test_mint_exactly_at_cap() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Mint exactly the cap amount
        let lfs_coin = lfs_token::mint_emissions(
            &mut emissions_cap,
            TOTAL_SUPPLY_CAP,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        assert!(coin::value(&lfs_coin) == TOTAL_SUPPLY_CAP, 0);

        let total_supply = lfs_token::total_supply(&treasury_cap);
        assert!(total_supply == TOTAL_SUPPLY_CAP, 1);

        transfer::public_transfer(lfs_coin, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test that one more token after cap fails
    #[test]
    #[expected_failure(abort_code = 3)] // E_TOTAL_SUPPLY_CAP_EXCEEDED
    fun test_one_token_over_cap() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Mint to cap
        let lfs_1 = lfs_token::mint_emissions(
            &mut emissions_cap,
            TOTAL_SUPPLY_CAP,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        // Try to mint one more token - should fail
        let lfs_2 = lfs_token::mint_emissions(
            &mut emissions_cap,
            1,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        transfer::public_transfer(lfs_1, ALICE);
        transfer::public_transfer(lfs_2, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test emissions tracking accuracy
    #[test]
    fun test_emissions_tracking() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        let mut total_minted = 0u64;
        let mut i = 0;

        // Mint 10 batches
        while (i < 10) {
            let amount = MINT_AMOUNT * (i + 1);
            let lfs = lfs_token::mint_emissions(
                &mut emissions_cap,
                amount,
                &mut treasury_cap,
                ts::ctx(&mut scenario)
            );

            total_minted = total_minted + amount;

            // Verify tracking is accurate after each mint
            let emissions_to_date = lfs_token::emissions_minted_to_date(&emissions_cap);
            assert!(emissions_to_date == total_minted, i);

            transfer::public_transfer(lfs, ALICE);
            i = i + 1;
        };

        // Final verification
        let total_supply = lfs_token::total_supply(&treasury_cap);
        let total_emissions = lfs_token::emissions_minted_to_date(&emissions_cap);

        assert!(total_supply == total_minted, 100);
        assert!(total_emissions == total_minted, 101);
        assert!(total_supply == total_emissions, 102);

        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test regular treasury minting (outside emissions)
    #[test]
    fun test_treasury_mint() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Mint directly from treasury (not through emissions cap)
        let lfs_treasury = coin::mint(&mut treasury_cap, MINT_AMOUNT, ts::ctx(&mut scenario));

        // Check total supply increased
        let total_supply = lfs_token::total_supply(&treasury_cap);
        assert!(total_supply == MINT_AMOUNT, 0);

        // But emissions tracking should still be zero
        let emissions = lfs_token::emissions_minted_to_date(&emissions_cap);
        assert!(emissions == 0, 1);

        transfer::public_transfer(lfs_treasury, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test mixed treasury and emissions minting
    #[test]
    fun test_mixed_minting() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Mint from treasury
        let lfs_treasury = coin::mint(&mut treasury_cap, MINT_AMOUNT, ts::ctx(&mut scenario));

        // Mint from emissions
        let lfs_emissions = lfs_token::mint_emissions(
            &mut emissions_cap,
            MINT_AMOUNT * 2,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        // Total supply should be sum of both
        let total_supply = lfs_token::total_supply(&treasury_cap);
        assert!(total_supply == MINT_AMOUNT * 3, 0);

        // But emissions tracking only counts emissions mint
        let emissions = lfs_token::emissions_minted_to_date(&emissions_cap);
        assert!(emissions == MINT_AMOUNT * 2, 1);

        transfer::public_transfer(lfs_treasury, ALICE);
        transfer::public_transfer(lfs_emissions, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test supply cap with mixed minting
    #[test]
    #[expected_failure(abort_code = 3)] // E_TOTAL_SUPPLY_CAP_EXCEEDED
    fun test_supply_cap_with_mixed_minting() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Mint 1M from treasury
        let lfs_treasury = coin::mint(
            &mut treasury_cap,
            TOTAL_SUPPLY_CAP / 2,
            ts::ctx(&mut scenario)
        );

        // Try to mint more than remaining cap from emissions - should fail
        let lfs_emissions = lfs_token::mint_emissions(
            &mut emissions_cap,
            TOTAL_SUPPLY_CAP / 2 + 1,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        transfer::public_transfer(lfs_treasury, ALICE);
        transfer::public_transfer(lfs_emissions, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test coin operations (join, split)
    #[test]
    fun test_coin_operations() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Mint two coins
        let lfs_1 = lfs_token::mint_emissions(
            &mut emissions_cap,
            MINT_AMOUNT,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        let lfs_2 = lfs_token::mint_emissions(
            &mut emissions_cap,
            MINT_AMOUNT,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        // Join them
        let mut lfs_joined = lfs_1;
        coin::join(&mut lfs_joined, lfs_2);

        assert!(coin::value(&lfs_joined) == MINT_AMOUNT * 2, 0);

        // Split
        let lfs_split = coin::split(&mut lfs_joined, MINT_AMOUNT / 2, ts::ctx(&mut scenario));

        assert!(coin::value(&lfs_split) == MINT_AMOUNT / 2, 1);
        assert!(coin::value(&lfs_joined) == MINT_AMOUNT * 2 - MINT_AMOUNT / 2, 2);

        transfer::public_transfer(lfs_joined, ALICE);
        transfer::public_transfer(lfs_split, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }

    // Test large emissions batches
    #[test]
    fun test_large_batch_emissions() {
        let mut scenario = ts::begin(ADMIN);

        let (mut treasury_cap, mut emissions_cap) = lfs_token::init_for_testing(ts::ctx(&mut scenario));

        // Mint 100K LFS (10% of cap)
        let large_amount = TOTAL_SUPPLY_CAP / 10;
        let lfs = lfs_token::mint_emissions(
            &mut emissions_cap,
            large_amount,
            &mut treasury_cap,
            ts::ctx(&mut scenario)
        );

        assert!(coin::value(&lfs) == large_amount, 0);

        let emissions = lfs_token::emissions_minted_to_date(&emissions_cap);
        assert!(emissions == large_amount, 1);

        transfer::public_transfer(lfs, ALICE);
        transfer::public_transfer(treasury_cap, ADMIN);
        transfer::public_transfer(emissions_cap, ADMIN);
        ts::end(scenario);
    }
}
