package api

import (
	"encoding/json"

	"github.com/pattonkan/sui-go/sui"
)

type ProtocolStateDTO struct {
	CR           string `json:"cr"`
	CRTarget     string `json:"cr_target"`
	ReservesR    string `json:"reserves_r"`
	SupplyF      string `json:"supply_f"`
	SupplyX      string `json:"supply_x"`
	Px           uint64 `json:"px"`
	PegDeviation string `json:"peg_deviation"`
	OracleAgeSec int64  `json:"oracle_age_s"`
	Mode         string `json:"mode"`
	AsOf         int64  `json:"asOf"`
}

type ProtocolMetricsDTO struct {
	CurrentCR    string `json:"currentCR"`
	TargetCR     string `json:"targetCR"`
	PegDeviation string `json:"pegDeviation"`
	ReservesR    string `json:"reservesR"`
	SupplyF      string `json:"supplyF"`
	SupplyX      string `json:"supplyX"`
	SPTVL        string `json:"spTVL"`
	RewardAPR    string `json:"rewardAPR"`
	IndexDelta   string `json:"indexDelta"`
	AsOf         int64  `json:"asOf"`
}

type HealthDTO struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons"`
}

type QuoteMintDTO struct {
	FOut   string `json:"fOut"`
	Fee    string `json:"fee"`
	PostCR string `json:"postCR"`
	TTL    int    `json:"ttlSec"`
	ID     string `json:"quoteId"`
	AsOf   int64  `json:"asOf"`
}

type QuoteRedeemDTO struct {
	ROut   string `json:"rOut"`
	Fee    string `json:"fee"`
	PostCR string `json:"postCR"`
	TTL    int    `json:"ttlSec"`
	ID     string `json:"quoteId"`
	AsOf   int64  `json:"asOf"`
}

type QuoteMintXDTO struct {
	XOut   string `json:"xOut"`
	Fee    string `json:"fee"`
	PostCR string `json:"postCR"`
	TTL    int    `json:"ttlSec"`
	ID     string `json:"quoteId"`
	AsOf   int64  `json:"asOf"`
}

type QuoteRedeemXDTO struct {
	ROut   string `json:"rOut"`
	Fee    string `json:"fee"`
	PostCR string `json:"postCR"`
	TTL    int    `json:"ttlSec"`
	ID     string `json:"quoteId"`
	AsOf   int64  `json:"asOf"`
}

type QuoteStakeDTO struct {
	ExpectedIndexDelta string `json:"expectedIndexDelta"`
	EstAPR             string `json:"estAPR"`
	TTL                int    `json:"ttlSec"`
	QuoteID            string `json:"quoteId"`
	AsOf               int64  `json:"asOf"`
}

type SPIndexDTO struct {
	IndexNow    string `json:"indexNow"`
	Index24hAgo string `json:"index24hAgo"`
	APR         string `json:"apr"`
	TVLF        string `json:"tvlF"`
}

type SPUserDTO struct {
	StakeF            string `json:"stakeF"`
	EnteredAt         int64  `json:"enteredAt"`
	IndexAtJoin       string `json:"indexAtJoin"`
	ClaimableR        string `json:"claimableR"`
	PendingIndexDelta string `json:"pendingIndexDelta"`
}

type UserPositionsDTO struct {
	Address   *sui.Address      `json:"address"`
	Balances  map[string]string `json:"balances"`
	SPStake   *SPUserDTO        `json:"spStake,omitempty"`
	UpdatedAt int64             `json:"updatedAt"`
}

type UserBalancesDTO struct {
	Address   *sui.Address      `json:"address"`
	Balances  map[string]string `json:"balances"`
	UpdatedAt int64             `json:"updatedAt"`
}

type TransactionDTO struct {
	ID        int64                  `json:"id"`
	Timestamp int64                  `json:"timestamp"`
	Type      string                 `json:"type"`
	TxDigest  string                 `json:"txDigest"`
	Fields    map[string]interface{} `json:"fields"`
}

type PaginatedResponse struct {
	Data    interface{} `json:"data"`
	Cursor  string      `json:"cursor,omitempty"`
	HasMore bool        `json:"hasMore"`
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Query parameters for endpoints
type QuoteMintRequest struct {
	AmountR  string `form:"amountR" validate:"required"`
	MinFOut  string `form:"minFOut"`
	Slippage string `form:"slippage"`
}

type QuoteRedeemRequest struct {
	AmountF  string `form:"amountF" validate:"required"`
	MinROut  string `form:"minROut"`
	Slippage string `form:"slippage"`
}

type QuoteStakeRequest struct {
	AmountF string `form:"amountF" validate:"required"`
}

type PaginationRequest struct {
	Limit  int    `form:"limit"`
	Cursor string `form:"cursor"`
}

// Stream/WebSocket message types
type StreamMessage struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

// Candle data for charts
type CandleDTO struct {
	Time  int64   `json:"time"` // unix timestamp in seconds
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

type CandlesRequest struct {
	Pair     string `form:"pair"`
	Interval string `form:"interval"`
	Limit    int    `form:"limit"`
}

// Transaction building types
type UnsignedTransactionRequest struct {
	Action    string `json:"action" validate:"required,oneof=mint redeem"`
	TokenType string `json:"tokenType" validate:"required,oneof=xtoken ftoken"`
	Amount    string `json:"amount" validate:"required"`
	MarketID  string `json:"marketId,omitempty"`
}

type UnsignedTransactionResponse struct {
	TransactionBlockBytes []byte            `json:"transactionBlockBytes"`
	GasEstimate           string            `json:"gasEstimate"`
	QuoteID               string            `json:"quoteId,omitempty"`
	Metadata              map[string]string `json:"metadata"`
}

type SignedTransactionRequest struct {
	TxBytes   string `json:"tx_bytes" validate:"required"`
	Signature string `json:"signature" validate:"required"`
	QuoteID   string `json:"quoteId,omitempty"`
}

type SignedTransactionResponse struct {
	TransactionDigest string `json:"transactionDigest"`
	Status            string `json:"status"`
}

// User transactions types
type TransactionItem struct {
	Hash      string `json:"hash"`
	Type      string `json:"type"`
	Amount    string `json:"amount"`
	Token     string `json:"token"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

type UserTransactionsDTO struct {
	Address    *sui.Address      `json:"address"`
	Items      []TransactionItem `json:"items"`
	NextCursor string            `json:"nextCursor"`
	UpdatedAt  int64             `json:"updatedAt"`
}

type UserTransactionsRequest struct {
	Address *sui.Address `json:"address"`
	Limit   int          `form:"limit"`
	Cursor  string       `form:"cursor"`
}

// Oracle Update API types
type UpdateOracleBuildRequest struct {
	Mode  string `json:"mode" validate:"required,oneof=execution devinspect"`
	Price uint64 `json:"price" validate:"required"`
}

type UpdateOracleBuildResponse struct {
	TransactionBlockBytes []byte            `json:"transactionBlockBytes"`
	GasEstimate           string            `json:"gasEstimate"`
	Metadata              map[string]string `json:"metadata"`
}

type UpdateOracleSubmitRequest struct {
	TxBytes   string `json:"tx_bytes" validate:"required"`
	Signature string `json:"signature" validate:"required"`
}

type UpdateOracleSubmitResponse struct {
	TransactionDigest string `json:"transactionDigest"`
	Status            string `json:"status"`
}

// Transaction building info endpoint types
type TransactionBuildInfoResponse struct {
	PackageId       string `json:"packageId"`
	ProtocolId      string `json:"protocolId"`
	PoolId          string `json:"poolId"`
	FtokenPackageId string `json:"ftokenPackageId"`
	XtokenPackageId string `json:"xtokenPackageId"`
	AdminCapId      string `json:"adminCapId"`
	FtokenTreasuryCapId string `json:"ftokenTreasuryCapId,omitempty"`
	XtokenTreasuryCapId string `json:"xtokenTreasuryCapId,omitempty"`
	FtokenAuthorityId   string `json:"ftokenAuthorityId,omitempty"`
	XtokenAuthorityId   string `json:"xtokenAuthorityId,omitempty"`
	Network         string `json:"network"`
	RpcUrl          string `json:"rpcUrl"`
	WsUrl           string `json:"wsUrl"`
	EvmRpcUrl       string `json:"evmRpcUrl,omitempty"`
	EvmChainId      string `json:"evmChainId,omitempty"`
}
