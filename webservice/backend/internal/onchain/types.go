package onchain

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pattonkan/sui-go/sui"
	"github.com/pattonkan/sui-go/sui/movebcs"
	"github.com/shopspring/decimal"
)

type ProtocolState struct {
	CR           decimal.Decimal `json:"cr"`
	CRTarget     decimal.Decimal `json:"cr_target"`
	ReservesR    decimal.Decimal `json:"reserves_r"`
	SupplyF      decimal.Decimal `json:"supply_f"`
	SupplyX      decimal.Decimal `json:"supply_x"`
	Pf           uint64          `json:"pf"`
	Px           uint64          `json:"px"`
	P            uint64          `json:"p"` // price for reserve token
	PegDeviation decimal.Decimal `json:"peg_deviation"`
	Mode         string          `json:"mode"`
	OracleAgeSec int64           `json:"oracle_age_sec"`
	AsOf         time.Time       `json:"as_of"`
}

type MoveObjectProtocol struct {
	Id                      *sui.ObjectId
	AuthorizedPoolId        *sui.ObjectId
	ReserveTokenBalance     *movebcs.MoveBalance
	StableTreasuryCap       *movebcs.MoveTreasuryCap
	LeverageTreasuryCap     *movebcs.MoveTreasuryCap
	LastReservePrice        uint64
	Pf                      uint64
	Px                      uint64
	FeeTreasuryBalanceValue *movebcs.MoveBalance
	FeeConfig               *MoveFeeConfig
	AllowUserActions        bool
	LastOracleTs            time.Time
	StableSupply            *movebcs.MoveSupply
	LeverageSupply          *movebcs.MoveSupply
}

type MoveFeeConfig struct {
	NormalMintFFeeBps     uint64
	NormalMintXFeeBps     uint64
	NormalRedeemFFeeBps   uint64
	NormalRedeemXFeeBps   uint64
	L1RedeemXFeeBps       uint64       // Increased xToken redeem fee in L1
	StabilityBonusRateBps uint64       // Bonus rate for rebalancers
	FeeRecipient          *sui.Address // Where fees are sent
}

type SPIndex struct {
	IndexValue    decimal.Decimal `json:"index_value"`
	TVLF          decimal.Decimal `json:"tvl_f"`
	TotalRewardsR decimal.Decimal `json:"total_rewards_r"`
	AsOf          time.Time       `json:"as_of"`
}

type UserPositions struct {
	Address     *sui.Address    `json:"address"`
	BalanceF    decimal.Decimal `json:"balance_f"`
	BalanceX    decimal.Decimal `json:"balance_x"`
	BalanceR    decimal.Decimal `json:"balance_r"`
	StakeF      decimal.Decimal `json:"stake_f"`
	IndexAtJoin decimal.Decimal `json:"index_at_join"`
	ClaimableR  decimal.Decimal `json:"claimable_r"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type Balances struct {
	F decimal.Decimal `json:"f"`
	X decimal.Decimal `json:"x"`
	R decimal.Decimal `json:"r"`
}

type PreviewMint struct {
	FOut   decimal.Decimal `json:"f_out"`
	Fee    decimal.Decimal `json:"fee"`
	PostCR decimal.Decimal `json:"post_cr"`
}

type PreviewRedeem struct {
	ROut   decimal.Decimal `json:"r_out"`
	Fee    decimal.Decimal `json:"fee"`
	PostCR decimal.Decimal `json:"post_cr"`
}

type Event struct {
	ID             int64                  `json:"id"`
	Checkpoint     uint64                 `json:"checkpoint"`
	SequenceNumber uint64                 `json:"sequence_number"`
	Timestamp      time.Time              `json:"timestamp"`
	Type           string                 `json:"type"`
	TxDigest       string                 `json:"tx_digest"`
	Sender         string                 `json:"sender"`
	Fields         map[string]interface{} `json:"fields"`
}

const (
	EventTypeMint      = "MINT"
	EventTypeRedeem    = "REDEEM"
	EventTypeStake     = "STAKE"
	EventTypeUnstake   = "UNSTAKE"
	EventTypeClaim     = "CLAIM"
	EventTypeRebalance = "REBALANCE"
)

// String implements fmt.Stringer for ProtocolState
func (ps ProtocolState) String() string {
	return fmt.Sprintf("ProtocolState{CR=%s, CRTarget=%s, ReservesR=%s, SupplyF=%s, SupplyX=%s, PegDeviation=%s, Mode=%s, OracleAgeSec=%d, AsOf=%s}",
		ps.CR.String(),
		ps.CRTarget.String(),
		ps.ReservesR.String(),
		ps.SupplyF.String(),
		ps.SupplyX.String(),
		ps.PegDeviation.String(),
		ps.Mode,
		ps.OracleAgeSec,
		ps.AsOf.UTC().Format(time.RFC3339),
	)
}

// String implements fmt.Stringer for SPIndex
func (sp SPIndex) String() string {
	return fmt.Sprintf("SPIndex{IndexValue=%s, TVLF=%s, TotalRewardsR=%s, AsOf=%s}",
		sp.IndexValue.String(),
		sp.TVLF.String(),
		sp.TotalRewardsR.String(),
		sp.AsOf.UTC().Format(time.RFC3339),
	)
}

// String implements fmt.Stringer for UserPositions
func (up UserPositions) String() string {
	addressStr := ""
	if up.Address != nil {
		addressStr = up.Address.String()
	}

	return fmt.Sprintf("UserPositions{Address=%s, BalanceF=%s, BalanceX=%s, BalanceR=%s, StakeF=%s, IndexAtJoin=%s, ClaimableR=%s, UpdatedAt=%s}",
		addressStr,
		up.BalanceF.String(),
		up.BalanceX.String(),
		up.BalanceR.String(),
		up.StakeF.String(),
		up.IndexAtJoin.String(),
		up.ClaimableR.String(),
		up.UpdatedAt.UTC().Format(time.RFC3339),
	)
}

// String implements fmt.Stringer for Balances
func (b Balances) String() string {
	return fmt.Sprintf("Balances{F=%s, X=%s, R=%s}",
		b.F.String(),
		b.X.String(),
		b.R.String(),
	)
}

// String implements fmt.Stringer for PreviewMint
func (pm PreviewMint) String() string {
	return fmt.Sprintf("PreviewMint{FOut=%s, Fee=%s, PostCR=%s}",
		pm.FOut.String(),
		pm.Fee.String(),
		pm.PostCR.String(),
	)
}

// String implements fmt.Stringer for PreviewRedeem
func (pr PreviewRedeem) String() string {
	return fmt.Sprintf("PreviewRedeem{ROut=%s, Fee=%s, PostCR=%s}",
		pr.ROut.String(),
		pr.Fee.String(),
		pr.PostCR.String(),
	)
}

// String implements fmt.Stringer for Event
func (e Event) String() string {
	// Sort map keys for stable output
	fieldsKeys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		fieldsKeys = append(fieldsKeys, k)
	}
	sort.Strings(fieldsKeys)
	fieldsStr := "{" + strings.Join(fieldsKeys, ",") + "}"

	return fmt.Sprintf("Event{ID=%d, Checkpoint=%d, SequenceNumber=%d, Timestamp=%s, Type=%s, TxDigest=%s, Sender=%s, Fields=%s}",
		e.ID,
		e.Checkpoint,
		e.SequenceNumber,
		e.Timestamp.UTC().Format(time.RFC3339),
		e.Type,
		e.TxDigest,
		e.Sender,
		fieldsStr,
	)
}

// String implements fmt.Stringer for MoveObjectProtocol
func (mop MoveObjectProtocol) String() string {
	return fmt.Sprintf("MoveObjectProtocol{Id=%s, ReserveTokenBalance=%d, StableTreasuryCap=%v, LeverageTreasuryCap=%v, LastReservePrice=%d, Pf=%d, Px=%d, FeeTreasuryBalanceValue=%d, AllowUserActions=%t, LastOracleTs=%s, StableSupply=%d, LeverageSupply=%d}",
		mop.Id.String(),
		mop.ReserveTokenBalance.Value,
		mop.StableTreasuryCap,
		mop.LeverageTreasuryCap,
		mop.LastReservePrice,
		mop.Pf,
		mop.Px,
		mop.FeeTreasuryBalanceValue.Value,
		mop.AllowUserActions,
		mop.LastOracleTs.UTC().Format(time.RFC3339),
		mop.StableSupply.Value,
		mop.LeverageSupply.Value,
	)
}
