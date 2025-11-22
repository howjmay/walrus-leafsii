
# ============================================================
# Stability Pool (Deferred index-accrual model; pairs with core v1 β_f=0)
# ============================================================
# - Users deposit fToken into SP (shares). We track scaled-shares:
#     actual_f_total = sp_scaled_total * sp_scale
#     user_actual_f  = user.sp_scaled * sp_scale
# - Protocol L3 rebalance uses a *pro‑rata burn* of SP deposits and indexes
#   SUI obligation to depositors: no SUI leaves reserve at L3 time.
# - Users later claim SUI (or rToken) which *then* reduces reserve_sui and
#   sp_obligation_sui.
# ============================================================

from __future__ import annotations

# Pull selected globals from core if available (for type hints only)
try:
    from pseudocode_stable import Pf, p_sui, reserve_sui  # noqa: F401
except Exception:
    pass

# ----------------------------
# Parameters
# ----------------------------
HARVEST_BOUNTY_BPS     = 100      # 1% to harvest caller
SP_MAX_BURN_FRAC_CALL  = 0.50     # cap per controller call (safety / pacing)

BPS = 10_000
EPS = 1e-12

# ----------------------------
# State
# ----------------------------
sp_scale: float = 1.0            # global shrink factor for pro-rata burns
sp_scaled_total: float = 0.0     # sum of user scaled shares
sp_index_sui_scaled: float = 0.0 # cumulative SUI-per-scaled-share
sp_obligation_sui: float = 0.0   # SUI owed to SP depositors (deferred)

# Per-user record (example shape; host app should manage storage)
class SPUser:
    def __init__(self):
        self.ftoken_balance = 0.0  # free fTokens (not in SP)
        self.sp_scaled      = 0.0  # scaled shares
        self.sp_index_snap  = 0.0  # last index snapshot

# ----------------------------
# Views
# ----------------------------
def sp_total_f() -> float:
    return sp_scaled_total * sp_scale

def sp_quote_burn_cap() -> float:
    # per-call cap
    return SP_MAX_BURN_FRAC_CALL * sp_total_f()

# ----------------------------
# Core-facing controller hook (ONLY Core should call)
# ----------------------------
def sp_controller_rebalance(f_burn: float, payout_sui: float) -> tuple[float, float]:
    """
    Core requests to burn f_burn from SP and index payout_sui in SUI.
    We may reduce both by cap; we:
      1) compute pre-burn totals,
      2) compute allowed_burn (cap),
      3) shrink sp_scale by burn fraction,
      4) add allowed_payout / sp_scaled_total to the index,
      5) increase sp_obligation_sui by allowed_payout.
    Returns (burned_f, indexed_sui).
    """
    global sp_scale, sp_index_sui_scaled, sp_obligation_sui

    f_total_pre = sp_total_f()
    if f_total_pre <= EPS or f_burn <= EPS:
        return (0.0, 0.0)

    # Cap the burn
    allowed_burn = min(f_burn, SP_MAX_BURN_FRAC_CALL * f_total_pre)

    if allowed_burn <= EPS:
        return (0.0, 0.0)

    # Scale the payout proportionally if we cut the requested burn
    sui_per_f = payout_sui / f_burn
    allowed_payout = sui_per_f * allowed_burn

    # 1) Index: use *scaled* denominator snapshot
    if sp_scaled_total <= EPS:
        return (0.0, 0.0)
    delta = allowed_payout / sp_scaled_total
    sp_index_sui_scaled += delta
    sp_obligation_sui   += allowed_payout

    # 2) Pro-rata burn via scale shrink
    frac = allowed_burn / f_total_pre               # fraction of total burnt
    sp_scale *= max(0.0, 1.0 - frac)                # shrink scale

    # NOTE: we do NOT touch reserve_sui here (deferred model).
    return (allowed_burn, allowed_payout)

# ----------------------------
# User flows
# ----------------------------
def _settle_user(user: SPUser) -> float:
    """Accrue pending SUI to be paid to user (but do not pay yet)."""
    owed = user.sp_scaled * (sp_index_sui_scaled - user.sp_index_snap)
    user.sp_index_snap = sp_index_sui_scaled
    return max(0.0, owed)

def sp_deposit(user: SPUser, f_amount: float):
    """User deposits fToken into SP (settles rewards first)."""
    global sp_scaled_total
    if f_amount <= 0.0 or user.ftoken_balance < f_amount:
        raise Exception("bad amount")
    newly = _settle_user(user)   # so index snapshots are fair
    # (protocol may auto-claim here, but we keep obligation until claim)
    scaled = f_amount / sp_scale
    user.ftoken_balance -= f_amount
    user.sp_scaled += scaled
    sp_scaled_total += scaled

def sp_withdraw(user: SPUser, f_amount: float):
    """User withdraws fToken from SP (settles rewards first)."""
    global sp_scaled_total
    if f_amount <= 0.0:
        raise Exception("bad amount")
    newly = _settle_user(user)
    # available actual f
    available = user.sp_scaled * sp_scale
    if f_amount > available + EPS:
        raise Exception("insufficient SP balance")
    scaled = f_amount / sp_scale
    user.sp_scaled -= scaled
    sp_scaled_total -= scaled
    user.ftoken_balance += f_amount

def sp_claim(user: SPUser, core_pay_sui_cb):
    """
    User pulls their accrued SUI. This is the *only* place where reserve_sui is reduced.
    core_pay_sui_cb(amount_sui) must:
      - reduce core.reserve_sui by amount_sui
      - transfer amount_sui to user
    (SP reduces sp_obligation_sui itself)
    """
    global sp_obligation_sui
    owed = _settle_user(user)
    if owed <= EPS:
        return 0.0
    # callback performs the state transitions on the core side
    core_pay_sui_cb(owed)
    sp_obligation_sui -= owed
    # guard against drift
    if sp_obligation_sui < 0: sp_obligation_sui = 0.0
    return owed

# ----------------------------
# Yield harvest indexing (optional)
# ----------------------------
def sp_index_harvest(yield_sui: float, caller_addr=None) -> float:
    """
    Index natural staking yield into SP (less bounty).
    Returns bounty paid to caller (pull-based elsewhere).
    """
    global sp_index_sui_scaled, sp_obligation_sui
    if yield_sui <= EPS or sp_scaled_total <= EPS:
        return 0.0
    bounty = (yield_sui * HARVEST_BOUNTY_BPS) / BPS
    to_pool = yield_sui - bounty
    delta = to_pool / sp_scaled_total
    sp_index_sui_scaled += delta
    sp_obligation_sui   += to_pool
    # Bounty is owed as well (can be a separate small pot); we skip for brevity.
    return bounty
