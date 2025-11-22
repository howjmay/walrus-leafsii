package api

// Cross-chain DTOs separate API wire format from internal types.
type WalrusCheckpointDTO struct {
	UpdateID     uint64 `json:"updateId"`
	ChainID      string `json:"chainId"`
	Asset        string `json:"asset"`
	Vault        string `json:"vault"`
	BlockNumber  uint64 `json:"blockNumber"`
	BlockHash    string `json:"blockHash,omitempty"`
	TotalShares  string `json:"totalShares"`
	Index        string `json:"index"`
	BalancesRoot string `json:"balancesRoot"`
	ProofType    string `json:"proofType,omitempty"`
	WalrusBlobID string `json:"walrusBlobId,omitempty"`
	Status       string `json:"status"`
	Timestamp    int64  `json:"timestamp"`
}

type WalrusCheckpointResponse struct {
	Checkpoint *WalrusCheckpointDTO `json:"checkpoint,omitempty"`
}

type SubmitCheckpointRequest struct {
	ChainID      string `json:"chainId"`
	Asset        string `json:"asset"`
	Vault        string `json:"vault"`
	BlockNumber  uint64 `json:"blockNumber"`
	BlockHash    string `json:"blockHash"`
	TotalShares  string `json:"totalShares"`
	Index        string `json:"index"`
	BalancesRoot string `json:"balancesRoot"`
	ProofType    string `json:"proofType"`
	ProofBlob    string `json:"proofBlob"`
	WalrusBlobID string `json:"walrusBlobId"`
}

type CrossChainBalanceDTO struct {
	SuiOwner         string `json:"suiOwner"`
	ChainID          string `json:"chainId"`
	Asset            string `json:"asset"`
	Shares           string `json:"shares"`
	Index            string `json:"index"`
	Value            string `json:"value"`
	CollateralUSD    string `json:"collateralUsd"`
	LastCheckpointID uint64 `json:"lastCheckpointId"`
	UpdatedAt        int64  `json:"updatedAt"`
}

type CrossChainBalanceResponse struct {
	Balance CrossChainBalanceDTO `json:"balance"`
}

type CreateVoucherRequest struct {
	SuiOwner string `json:"suiOwner"`
	ChainID  string `json:"chainId"`
	Asset    string `json:"asset"`
	Shares   string `json:"shares"`
	Expiry   int64  `json:"expiry"`
}

type BridgeDepositRequest struct {
	TxHash   string `json:"txHash"`
	SuiOwner string `json:"suiOwner"`
	ChainID  string `json:"chainId"`
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
}

type BridgeReceiptDTO struct {
	ReceiptID    string   `json:"receiptId"`
	TxHash       string   `json:"txHash,omitempty"`
	SuiOwner     string   `json:"suiOwner"`
	ChainID      string   `json:"chainId"`
	Asset        string   `json:"asset"`
	Minted       string   `json:"minted"`
	CreatedAt    int64    `json:"createdAt"`
	SuiTxDigests []string `json:"suiTxDigests,omitempty"`
}

type BridgeReceiptResponse struct {
	Receipt BridgeReceiptDTO `json:"receipt"`
}

type BridgeRedeemRequest struct {
	SuiTxDigest  string `json:"suiTxDigest"`
	SuiOwner     string `json:"suiOwner"`
	EthRecipient string `json:"ethRecipient"`
	ChainID      string `json:"chainId"`
	Asset        string `json:"asset"`
	Token        string `json:"token"`
	Amount       string `json:"amount"`
}

type RedeemReceiptDTO struct {
	ReceiptID      string `json:"receiptId"`
	SuiTxDigest    string `json:"suiTxDigest"`
	SuiOwner       string `json:"suiOwner"`
	EthRecipient   string `json:"ethRecipient"`
	ChainID        string `json:"chainId"`
	Asset          string `json:"asset"`
	Token          string `json:"token"`
	Burned         string `json:"burned"`
	PayoutEth      string `json:"payoutEth"`
	WalrusUpdateID uint64 `json:"walrusUpdateId,omitempty"`
	WalrusBlobID   string `json:"walrusBlobId,omitempty"`
	PayoutTxHash   string `json:"payoutTxHash,omitempty"`
	CreatedAt      int64  `json:"createdAt"`
}

type RedeemReceiptResponse struct {
	Receipt RedeemReceiptDTO `json:"receipt"`
}

type VoucherDTO struct {
	VoucherID string `json:"voucherId"`
	SuiOwner  string `json:"suiOwner"`
	ChainID   string `json:"chainId"`
	Asset     string `json:"asset"`
	Shares    string `json:"shares"`
	Nonce     uint64 `json:"nonce"`
	Expiry    int64  `json:"expiry"`
	Status    string `json:"status"`
	TxHash    string `json:"txHash,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

type VoucherResponse struct {
	Voucher *VoucherDTO `json:"voucher,omitempty"`
}

type VoucherListResponse struct {
	Vouchers []VoucherDTO `json:"vouchers"`
}

type CollateralParamsDTO struct {
	ChainID              string `json:"chainId"`
	Asset                string `json:"asset"`
	LTV                  string `json:"ltv"`
	MaintenanceThreshold string `json:"maintenanceThreshold"`
	LiquidationPenalty   string `json:"liquidationPenalty"`
	OracleHaircut        string `json:"oracleHaircut"`
	StalenessHardCap     int64  `json:"stalenessHardCap"`
	MintRateLimit        string `json:"mintRateLimit"`
	WithdrawRateLimit    string `json:"withdrawRateLimit"`
	Active               bool   `json:"active"`
}

type CollateralParamsResponse struct {
	Params *CollateralParamsDTO `json:"params,omitempty"`
}

type VaultInfoDTO struct {
	ChainID           string `json:"chainId"`
	Asset             string `json:"asset"`
	VaultAddress      string `json:"vaultAddress"`
	DepositMemoFormat string `json:"depositMemoFormat"`
	FeedURL           string `json:"feedUrl,omitempty"`
	ProofCID          string `json:"proofCid,omitempty"`
	SnapshotURL       string `json:"snapshotUrl,omitempty"`
}

type VaultInfoResponse struct {
	Vault *VaultInfoDTO `json:"vault,omitempty"`
}
