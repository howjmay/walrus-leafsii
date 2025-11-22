
# ============================================================
# f(x) Protocol v1 — Core (β_f = 0; Deferred Stability Pool model)
# ============================================================
# - Reserve asset: SUI-like (price in USD via oracle)
# - fToken: low-volatility liability; Pf fixed at $1 because β_f = 0
# - xToken: equity; Px implied by invariant
# - CR = (reserve_net_usd) / (Nf * Pf), where reserve_net_usd = (reserve_sui - sp_obligation_sui) * p_sui
#
# This file exposes the "core" policy including an L3 protocol rebalance that
# burns fToken using the Stability Pool (SP) *without* paying SUI immediately.
# Instead, SUI is recorded as an obligation and distributed later via SP.claim().
#
# The matching SP implementation is in `rebalance_pool.py`.
# ============================================================

from __future__ import annotations
from dataclasses import dataclass

# ---------------------------------------------
# Parameters (configurable)
# ---------------------------------------------
# VaR-style defaults from paper (rounded up slightly); adjust via governance
CR_T_L1 = 1.306   # Stability mode
CR_T_L2 = 1.206   # User rebalance mode
CR_T_L3 = 1.144   # Protocol rebalance
CR_T_L4 = 1.050   # Emergency recap

# Economics
BETA_F = 0.0      # <-- locked per request (Pf stays $1)
PF_FIXED = 1.0

# Oracle safety
MAX_STALENESS_SEC = 3600
MAX_REL_STEP      = 0.20   # max 20% per update

# L3 pacing (also enforced inside SP)
MAX_F_BURN_FRACTION_PER_CALL = 0.50

EPS = 1e-12

# ---------------------------------------------
# Global state (simplified)
# ---------------------------------------------
Nf = 0.0                 # fToken supply
Nx = 0.0                 # xToken supply
Pf = PF_FIXED            # fToken NAV ($)
Px = 0.0                 # xToken NAV ($), implied
p_sui = 0.0              # SUI price ($)
last_oracle_ts = 0

reserve_sui  = 0.0       # SUI reserve balance
treasury_sui = 0.0       # Treasury SUI for bonuses/fees/etc.

# Imported from SP module at runtime (single source of truth for obligations)
# Here we assign placeholders so this file remains importable on its own;
# when integrated, these will be bound by SP.bind_core(self) or by shared module scope.
try:
    from rebalance_pool import sp_obligation_sui, sp_quote_burn_cap, sp_controller_rebalance
except Exception:
    sp_obligation_sui = 0.0
    def sp_quote_burn_cap() -> float:
        return 0.0
    def sp_controller_rebalance(f_burn: float, payout_sui: float) -> tuple[float, float]:
        return 0.0, 0.0

# ---------------------------------------------
# Helpers
# ---------------------------------------------
def reserve_usd() -> float:
    return reserve_sui * p_sui

def reserve_net_sui() -> float:
    # Net of indexed-but-unpaid SP obligations (deferred model)
    return max(0.0, reserve_sui - sp_obligation_sui)

def reserve_net_usd() -> float:
    return reserve_net_sui() * p_sui

def collateral_ratio() -> float:
    denom = max(EPS, Nf * Pf)
    return reserve_net_usd() / denom

def _update_px():
    global Px
    # Px implied from invariant: reserve_usd = Nf*Pf + Nx*Px  => Px = (reserve_usd - Nf*Pf)/Nx
    if Nx <= EPS:
        # Bootstrap: keep Px as-is (or set policy value)
        return
    Px = max(0.0, (reserve_usd() - (Nf * Pf)) / max(EPS, Nx))

# ---------------------------------------------
# Oracle + NAV updater (atomic)
# ---------------------------------------------
def update_from_oracle(p_new: float, now_ts: int):
    """
    Single source of truth for price and NAV updates.
    With β_f = 0, Pf stays at $1; Px is recomputed from the invariant.
    """
    global p_sui, last_oracle_ts, Pf
    if last_oracle_ts != 0:
        # Staleness
        if now_ts - last_oracle_ts > MAX_STALENESS_SEC:
            raise Exception("oracle too stale")
        # Max step
        rel = abs(p_new / max(EPS, p_sui) - 1.0)
        if rel > MAX_REL_STEP:
            raise Exception("oracle step too large")

    p_sui = p_new
    last_oracle_ts = now_ts

    # β_f = 0 -> Pf fixed to 1.0; keep explicit in case of future governance change
    Pf = PF_FIXED
    _update_px()

# ---------------------------------------------
# Mode helpers
# ---------------------------------------------
def current_level() -> int:
    cr = collateral_ratio()
    if cr >= CR_T_L1: return 1
    if cr >= CR_T_L2: return 2
    if cr >= CR_T_L3: return 3
    if cr >= CR_T_L4: return 4
    return 5  # emergency recap

# ---------------------------------------------
# Mint / Redeem (sketches; fee policy omitted for brevity)
# ---------------------------------------------
def mint_ftoken(deposit_sui: float) -> float:
    """
    User deposits SUI; protocol issues fToken at Pf (=$1).
    """
    global reserve_sui, Nf
    if deposit_sui <= 0: raise Exception("bad amount")
    reserve_sui += deposit_sui
    issued_f = deposit_sui * p_sui / Pf
    Nf += issued_f
    _update_px()
    return issued_f

def redeem_ftoken(burn_f: float) -> float:
    """
    User returns fToken; receives SUI at Pf (less fees by level; omitted).
    """
    global reserve_sui, Nf
    if burn_f <= 0 or burn_f > Nf: raise Exception("bad amount")
    sui_out = burn_f * Pf / p_sui
    # In deferred model this *direct* redemption is paid now.
    if sui_out > reserve_net_sui(): raise Exception("insufficient reserve net of SP obligations")
    reserve_sui -= sui_out
    Nf -= burn_f
    _update_px()
    return sui_out

# (xToken mint/redeem omitted; governed by fee table & invariant preservation)

# ---------------------------------------------
# L3 Protocol Rebalance (Deferred payment via SP index)
# ---------------------------------------------
def _compute_f_burn_needed_for_target(target_cr: float) -> float:
    """
    Solve for Nf' s.t. CR' == target_cr, with reserve_net held constant (no immediate SUI movement).
    CR' = (reserve_net_sui * p_sui) / (Nf' * Pf)
    => Nf' = (reserve_net_sui * p_sui) / (target_cr * Pf)
    f_burn_needed = max(0, Nf - Nf')
    """
    if target_cr <= 0: raise Exception("bad target")
    nf_target = (reserve_net_sui() * p_sui) / (target_cr * Pf)
    return max(0.0, Nf - nf_target)

def protocol_rebalance_L3_to_target(target_cr: float = CR_T_L1) -> tuple[float, float]:
    """
    Burns fToken using SP deposits to push CR up to target_cr.
    - Defers SUI payment: records obligation via SP index (no reserve_sui change here).
    Returns (burned_f, indexed_sui).
    """
    global Nf

    if p_sui <= EPS: raise Exception("oracle not set")
    if Nf <= EPS:    return (0.0, 0.0)

    # Need and cap
    need = _compute_f_burn_needed_for_target(target_cr)
    if need <= EPS: return (0.0, 0.0)

    cap_sp = sp_quote_burn_cap()  # SP-side fraction cap
    f_burn = max(0.0, min(need, cap_sp, Nf))
    if f_burn <= EPS: return (0.0, 0.0)

    payout_sui = f_burn * Pf / p_sui

    # Ask SP to (a) pro‑rata shrink deposits and (b) index SUI obligation
    burned, indexed_sui = sp_controller_rebalance(f_burn=f_burn, payout_sui=payout_sui)
    if burned <= EPS:
        return (0.0, 0.0)

    # Burn the liability now (single source of truth)
    Nf -= burned
    _update_px()
    # IMPORTANT: Do NOT touch reserve_sui here (deferred).
    return (burned, indexed_sui)

# ---------------------------------------------
# Emergency recap (L4)
# ---------------------------------------------
def emergency_recapitalization(target_sui: float) -> float:
    """
    Governance-controlled recap to raise reserve_sui immediately (outside SP).
    """
    global reserve_sui
    if target_sui <= 0: return 0.0
    raised = min(target_sui, _raise_collateral_via_governance(target_sui))
    reserve_sui += raised
    _update_px()
    return raised

def _raise_collateral_via_governance(target_sui: float) -> float:
    # Placeholder: sell governance tokens / backstop fund etc.
    return 0.0


# ---------------------------------------------
# Callback example for SP.claim()
# ---------------------------------------------
def sp_claim_pay_sui(amount_sui: float):
    """
    SP calls this to settle a user's indexed SUI.
    This is the *only* place (besides direct fToken redemption) that reduces reserve_sui
    in the deferred model.
    """
    global reserve_sui
    if amount_sui <= 0: return
    if amount_sui > reserve_sui + EPS:
        raise Exception("insufficient reserve to honor SP claim")
    reserve_sui -= amount_sui

# ---------------------------------------------
# Invariants & sanity checks
# ---------------------------------------------
def check_invariant(tol: float = 1e-6) -> bool:
    # n_eth·p_eth ≈ n_f·p_f + n_x·p_x   (SP obligations do not affect invariant)
    lhs = reserve_usd()
    rhs = Nf*Pf + Nx*Px
    return abs(lhs - rhs) <= tol

def check_solvency() -> bool:
    # Reserve must cover SP obligations
    return reserve_sui + EPS >= sp_obligation_sui
