module xtoken::xtoken;

use std::option;
use sui::coin::{Self, TreasuryCap};
use sui::object;
use sui::transfer;
use sui::tx_context;
use sui::event;

const E_NOT_ADMIN: u64 = 1;
const E_NOT_AUTHORIZED: u64 = 2;
const E_INVALID_AMOUNT: u64 = 3;

public struct XTOKEN has drop {}

/// Shared authority object tying the treasury cap to an admin/bridge worker.
public struct MintAuthority has key, store {
    id: object::UID,
    admin: address,
    bridge_worker: address,
}

/// Event emitted whenever xETH is burned for an external (EVM) redemption.
public struct BridgeRedeemEvent has copy, drop {
    redeemer: address,
    eth_recipient: vector<u8>,
    amount: u64,
}

fun init(witness: XTOKEN, ctx: &mut tx_context::TxContext) {
    let (treasury, metadata) = coin::create_currency(
        witness,
        9,
        b"XTOKEN",
        b"",
        b"",
        option::none(),
        ctx,
    );

    transfer::public_freeze_object(metadata);

    let sender = tx_context::sender(ctx);
    let authority = MintAuthority {
        id: object::new(ctx),
        admin: sender,
        bridge_worker: sender,
    };

    // Transfer treasury cap to sender; share authority for bridge operations
    transfer::public_transfer(treasury, sender);
    transfer::public_share_object(authority);
}

/// Admin-only hook to rotate the authorized bridge worker.
public entry fun update_bridge_worker(
    authority: &mut MintAuthority,
    new_worker: address,
    ctx: &mut tx_context::TxContext,
) {
    assert!(tx_context::sender(ctx) == authority.admin, E_NOT_ADMIN);
    authority.bridge_worker = new_worker;
}

/// Bridge mint interface restricted to the admin or delegated worker.
public entry fun bridge_mint(
    treasury_cap: &mut TreasuryCap<XTOKEN>,
    authority: &MintAuthority,
    amount: u64,
    recipient: address,
    ctx: &mut tx_context::TxContext,
) {
    assert!(amount > 0, E_INVALID_AMOUNT);
    let sender = tx_context::sender(ctx);
    assert!(
        sender == authority.admin || sender == authority.bridge_worker,
        E_NOT_AUTHORIZED
    );

    let coin = coin::mint(treasury_cap, amount, ctx);
    transfer::public_transfer(coin, recipient);
}

/// Burn bridged xETH in exchange for an off-chain ETH payout.
/// Emits an event that the bridge worker can listen to.
public entry fun bridge_redeem(
    treasury_cap: &mut TreasuryCap<XTOKEN>,
    authority: &MintAuthority,
    tokens: coin::Coin<XTOKEN>,
    eth_recipient: vector<u8>,
    ctx: &mut tx_context::TxContext,
) {
    let amount = coin::value(&tokens);
    assert!(amount > 0, E_INVALID_AMOUNT);
    let sender = tx_context::sender(ctx);
    assert!(
        sender == authority.admin || sender == authority.bridge_worker,
        E_NOT_AUTHORIZED
    );

    coin::burn(treasury_cap, tokens);
    event::emit(BridgeRedeemEvent {
        redeemer: sender,
        eth_recipient,
        amount,
    });
}
