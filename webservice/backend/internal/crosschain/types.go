package crosschain

import (
	"time"

	"github.com/shopspring/decimal"
)

// ChainID represents supported external chains (e.g., ethereum).
type ChainID string

const (
	ChainIDEthereum ChainID = "ethereum"
)

// CheckpointStatus reflects verification lifecycle.
type CheckpointStatus string

const (
	CheckpointStatusPending  CheckpointStatus = "pending"
	CheckpointStatusVerified CheckpointStatus = "verified"
	CheckpointStatusRejected CheckpointStatus = "rejected"
)

// VoucherStatus tracks withdrawal voucher usage.
type VoucherStatus string

const (
	VoucherStatusPending VoucherStatus = "pending"
	VoucherStatusSpent   VoucherStatus = "spent"
	VoucherStatusSettled VoucherStatus = "settled"
)

// WalrusCheckpoint captures cross-chain vault state published to Walrus.
type WalrusCheckpoint struct {
	UpdateID     uint64           `json:"updateId"`
	ChainID      ChainID          `json:"chainId"`
	Asset        string           `json:"asset"`
	Vault        string           `json:"vault"`
	BlockNumber  uint64           `json:"blockNumber"`
	BlockHash    string           `json:"blockHash,omitempty"`
	TotalShares  decimal.Decimal  `json:"totalShares"`
	Index        decimal.Decimal  `json:"index"`
	BalancesRoot string           `json:"balancesRoot"`
	ProofType    string           `json:"proofType,omitempty"`
	ProofBlob    []byte           `json:"proofBlob,omitempty"`
	WalrusBlobID string           `json:"walrusBlobId,omitempty"`
	Status       CheckpointStatus `json:"status"`
	Timestamp    time.Time        `json:"timestamp"`
}

// CrossChainBalance represents a user's balance bridged from another chain.
type CrossChainBalance struct {
	SuiOwner         string          `json:"suiOwner"`
	ChainID          ChainID         `json:"chainId"`
	Asset            string          `json:"asset"`
	Shares           decimal.Decimal `json:"shares"`
	Index            decimal.Decimal `json:"index"`
	Value            decimal.Decimal `json:"value"`
	CollateralUSD    decimal.Decimal `json:"collateralUsd"`
	LastCheckpointID uint64          `json:"lastCheckpointId"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

// WithdrawalVoucher is used for self-custody withdrawals on the source chain.
type WithdrawalVoucher struct {
	VoucherID     string          `json:"voucherId"`
	SuiOwner      string          `json:"suiOwner"`
	ChainID       ChainID         `json:"chainId"`
	Asset         string          `json:"asset"`
	Shares        decimal.Decimal `json:"shares"`
	Nonce         uint64          `json:"nonce"`
	Expiry        time.Time       `json:"expiry"`
	UserSignature string          `json:"userSignature"`
	ProofBlob     []byte          `json:"proofBlob,omitempty"`
	Status        VoucherStatus   `json:"status"`
	TxHash        string          `json:"txHash,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
}

// CollateralParams capture collateralization settings for a cross-chain asset.
type CollateralParams struct {
	ChainID              ChainID         `json:"chainId"`
	Asset                string          `json:"asset"`
	LTV                  decimal.Decimal `json:"ltv"`
	MaintenanceThreshold decimal.Decimal `json:"maintenanceThreshold"`
	LiquidationPenalty   decimal.Decimal `json:"liquidationPenalty"`
	OracleHaircut        decimal.Decimal `json:"oracleHaircut"`
	StalenessHardCap     time.Duration   `json:"stalenessHardCap"`
	MintRateLimit        decimal.Decimal `json:"mintRateLimit"`
	WithdrawRateLimit    decimal.Decimal `json:"withdrawRateLimit"`
	Active               bool            `json:"active"`
}

// VaultInfo holds deposit metadata for an external chain vault.
type VaultInfo struct {
	ChainID           ChainID `json:"chainId"`
	Asset             string  `json:"asset"`
	VaultAddress      string  `json:"vaultAddress"`
	DepositMemoFormat string  `json:"depositMemoFormat"`
	FeedURL           string  `json:"feedUrl,omitempty"`
	ProofCID          string  `json:"proofCid,omitempty"`
	SnapshotURL       string  `json:"snapshotUrl,omitempty"`
}
