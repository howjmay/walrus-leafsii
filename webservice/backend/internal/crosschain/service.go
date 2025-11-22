package crosschain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidRequest = errors.New("invalid request")
)

// Service manages cross-chain checkpoints, balances, and vouchers in-memory.
type Service struct {
	mu sync.RWMutex

	checkpoints map[string][]*WalrusCheckpoint
	balances    map[string]*CrossChainBalance
	vouchers    map[string]*WithdrawalVoucher
	params      map[string]CollateralParams
	vaults      map[string]VaultInfo

	updateCounter uint64
	nonceCounter  uint64

	logger *zap.SugaredLogger
}

func NewService(logger *zap.SugaredLogger) *Service {
	s := &Service{
		checkpoints: make(map[string][]*WalrusCheckpoint),
		balances:    make(map[string]*CrossChainBalance),
		vouchers:    make(map[string]*WithdrawalVoucher),
		params:      make(map[string]CollateralParams),
		vaults:      make(map[string]VaultInfo),
		logger:      logger,
	}
	s.seedDefaults()
	return s
}

func envOrDefault(def string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return def
}

// seedDefaults initializes the MVP with a single Ethereum vault and checkpoint.
func (s *Service) seedDefaults() {
	key := s.mapKey(ChainIDEthereum, "ETH")
	now := time.Now().Add(-2 * time.Minute)

	vaultAddr := envOrDefault("0xEThVau1t00000000000000000000000000000000",
		"LFS_CROSSCHAIN_VAULT_ADDRESS",
		"LFS_SEPOLIA_VAULT_ADDRESS",
		"LFS_LOCAL_ETH_VAULT_ADDRESS",
	)

	memoFormat := envOrDefault("", "LFS_CROSSCHAIN_MEMO_FORMAT")
	if memoFormat == "" {
		if sample := envOrDefault("", "LFS_SEPOLIA_SUI_OWNER_FOR_DEPOSIT", "LFS_SUI_OWNER"); sample != "" {
			memoFormat = fmt.Sprintf("Use your Sui address (e.g. %s) in the deposit memo", sample)
		} else {
			memoFormat = "Include your Sui address in memo"
		}
	}

	feedURL := envOrDefault("https://walrus.xyz/api/feeds/eth-vault", "LFS_CROSSCHAIN_FEED_URL")
	proofCID := envOrDefault("bafyEthereumVaultProof", "LFS_CROSSCHAIN_PROOF_CID")
	snapshotURL := envOrDefault("https://walrus.storage/eth/latest.json", "LFS_CROSSCHAIN_SNAPSHOT_URL")

	s.params[key] = CollateralParams{
		ChainID:              ChainIDEthereum,
		Asset:                "ETH",
		LTV:                  decimal.RequireFromString("0.65"),
		MaintenanceThreshold: decimal.RequireFromString("0.72"),
		LiquidationPenalty:   decimal.RequireFromString("0.06"),
		OracleHaircut:        decimal.RequireFromString("0.02"),
		StalenessHardCap:     60 * time.Minute,
		MintRateLimit:        decimal.RequireFromString("1000"),
		WithdrawRateLimit:    decimal.RequireFromString("1000"),
		Active:               true,
	}

	s.vaults[key] = VaultInfo{
		ChainID:           ChainIDEthereum,
		Asset:             "ETH",
		VaultAddress:      vaultAddr,
		DepositMemoFormat: memoFormat,
		FeedURL:           feedURL,
		ProofCID:          proofCID,
		SnapshotURL:       snapshotURL,
	}

	checkpoint := &WalrusCheckpoint{
		UpdateID:     1,
		ChainID:      ChainIDEthereum,
		Asset:        "ETH",
		Vault:        s.vaults[key].VaultAddress,
		BlockNumber:  100,
		BlockHash:    "0xmockblock",
		TotalShares:  decimal.RequireFromString("0.5"),
		Index:        decimal.RequireFromString("1.0001"),
		BalancesRoot: "0xmockroot",
		ProofType:    "zk",
		WalrusBlobID: "bafyEthereumVaultProof",
		Status:       CheckpointStatusVerified,
		Timestamp:    now,
	}

	s.checkpoints[key] = []*WalrusCheckpoint{checkpoint}
	s.updateCounter = checkpoint.UpdateID

	// Seed a sample balance for demo/testing flows.
	s.balances[s.balanceKey("0x123", ChainIDEthereum, "ETH")] = &CrossChainBalance{
		SuiOwner:         "0x123",
		ChainID:          ChainIDEthereum,
		Asset:            "ETH",
		Shares:           decimal.RequireFromString("0.5"),
		Index:            checkpoint.Index,
		Value:            decimal.RequireFromString("0.50005"),
		CollateralUSD:    decimal.RequireFromString("2850").Mul(decimal.RequireFromString("0.50005")),
		LastCheckpointID: checkpoint.UpdateID,
		UpdatedAt:        now,
	}
}

func (s *Service) mapKey(chainID ChainID, asset string) string {
	return fmt.Sprintf("%s:%s", chainID, asset)
}

func (s *Service) balanceKey(owner string, chainID ChainID, asset string) string {
	return fmt.Sprintf("%s:%s:%s", owner, chainID, asset)
}

// CreditDeposit mints shares for a Sui owner based on an observed external deposit.
func (s *Service) CreditDeposit(_ context.Context, suiOwner string, chainID ChainID, asset string, shares decimal.Decimal) (*CrossChainBalance, error) {
	if suiOwner == "" || shares.LessThanOrEqual(decimal.Zero) {
		return nil, ErrInvalidRequest
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Use latest checkpoint index to value the shares; fall back to 1 if none yet.
	idx := decimal.RequireFromString("1")
	if cp := s.latestCheckpointLocked(chainID, asset); cp != nil && !cp.Index.IsZero() {
		idx = cp.Index
	}

	key := s.balanceKey(suiOwner, chainID, asset)
	bal, ok := s.balances[key]
	if !ok {
		bal = &CrossChainBalance{
			SuiOwner: suiOwner,
			ChainID:  chainID,
			Asset:    asset,
		}
		s.balances[key] = bal
	}

	bal.Shares = bal.Shares.Add(shares)
	bal.Index = idx
	bal.Value = bal.Shares.Mul(idx)
	if cp := s.latestCheckpointLocked(chainID, asset); cp != nil {
		bal.LastCheckpointID = cp.UpdateID
	}
	bal.UpdatedAt = time.Now()

	return bal, nil
}

// DebitWithdrawal burns shares for a Sui owner when a redeem is fulfilled.
func (s *Service) DebitWithdrawal(_ context.Context, suiOwner string, chainID ChainID, asset string, shares decimal.Decimal) (*CrossChainBalance, error) {
	if suiOwner == "" || shares.LessThanOrEqual(decimal.Zero) {
		return nil, ErrInvalidRequest
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := decimal.RequireFromString("1")
	if cp := s.latestCheckpointLocked(chainID, asset); cp != nil && !cp.Index.IsZero() {
		idx = cp.Index
	}

	key := s.balanceKey(suiOwner, chainID, asset)
	bal, ok := s.balances[key]
	if !ok || bal.Shares.LessThan(shares) {
		return nil, ErrInvalidRequest
	}

	bal.Shares = bal.Shares.Sub(shares)
	bal.Index = idx
	bal.Value = bal.Shares.Mul(idx)
	if cp := s.latestCheckpointLocked(chainID, asset); cp != nil {
		bal.LastCheckpointID = cp.UpdateID
	}
	bal.UpdatedAt = time.Now()

	return bal, nil
}

func (s *Service) GetLatestCheckpoint(_ context.Context, chainID ChainID, asset string) (*WalrusCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.mapKey(chainID, asset)
	cps := s.checkpoints[key]
	if len(cps) == 0 {
		return nil, ErrNotFound
	}
	return cps[len(cps)-1], nil
}

func (s *Service) SubmitCheckpoint(_ context.Context, cp WalrusCheckpoint) (*WalrusCheckpoint, error) {
	if cp.ChainID == "" || cp.Asset == "" {
		return nil, ErrInvalidRequest
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.updateCounter++
	cp.UpdateID = s.updateCounter
	if cp.Timestamp.IsZero() {
		cp.Timestamp = time.Now()
	}
	if cp.Status == "" {
		cp.Status = CheckpointStatusVerified
	}

	key := s.mapKey(cp.ChainID, cp.Asset)
	s.checkpoints[key] = append(s.checkpoints[key], &cp)

	// Bump balances to new index for the given asset.
	for _, bal := range s.balances {
		if bal.ChainID == cp.ChainID && bal.Asset == cp.Asset {
			bal.Index = cp.Index
			bal.Value = bal.Shares.Mul(cp.Index)
			bal.LastCheckpointID = cp.UpdateID
			bal.UpdatedAt = cp.Timestamp
		}
	}

	return &cp, nil
}

func (s *Service) GetBalance(_ context.Context, suiOwner string, chainID ChainID, asset string) (*CrossChainBalance, error) {
	if suiOwner == "" {
		return nil, ErrInvalidRequest
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.balanceKey(suiOwner, chainID, asset)
	if bal, ok := s.balances[key]; ok {
		return bal, nil
	}

	// Return zero balance with latest checkpoint metadata if available.
	cp := s.latestCheckpointLocked(chainID, asset)
	var idx decimal.Decimal
	var cpID uint64
	if cp != nil {
		idx = cp.Index
		cpID = cp.UpdateID
	}

	return &CrossChainBalance{
		SuiOwner:         suiOwner,
		ChainID:          chainID,
		Asset:            asset,
		Shares:           decimal.Zero,
		Index:            idx,
		Value:            decimal.Zero,
		CollateralUSD:    decimal.Zero,
		LastCheckpointID: cpID,
		UpdatedAt:        time.Now(),
	}, nil
}

func (s *Service) latestCheckpointLocked(chainID ChainID, asset string) *WalrusCheckpoint {
	key := s.mapKey(chainID, asset)
	cps := s.checkpoints[key]
	if len(cps) == 0 {
		return nil
	}
	return cps[len(cps)-1]
}

func (s *Service) CreateVoucher(_ context.Context, voucher WithdrawalVoucher) (*WithdrawalVoucher, error) {
	if voucher.SuiOwner == "" || voucher.Shares.LessThanOrEqual(decimal.Zero) {
		return nil, ErrInvalidRequest
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nonceCounter++
	voucher.Nonce = s.nonceCounter
	if voucher.VoucherID == "" {
		voucher.VoucherID = fmt.Sprintf("voucher_%d", voucher.Nonce)
	}
	if voucher.Status == "" {
		voucher.Status = VoucherStatusPending
	}
	if voucher.CreatedAt.IsZero() {
		voucher.CreatedAt = time.Now()
	}

	s.vouchers[voucher.VoucherID] = &voucher
	return &voucher, nil
}

func (s *Service) ListVouchers(_ context.Context, suiOwner string) ([]*WithdrawalVoucher, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WithdrawalVoucher
	for _, v := range s.vouchers {
		if v.SuiOwner == suiOwner {
			result = append(result, v)
		}
	}
	return result, nil
}

func (s *Service) GetVoucher(_ context.Context, voucherID string) (*WithdrawalVoucher, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if v, ok := s.vouchers[voucherID]; ok {
		return v, nil
	}
	return nil, ErrNotFound
}

func (s *Service) GetCollateralParams(_ context.Context, chainID ChainID, asset string) (*CollateralParams, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.mapKey(chainID, asset)
	if params, ok := s.params[key]; ok {
		return &params, nil
	}
	return nil, ErrNotFound
}

func (s *Service) GetVault(_ context.Context, chainID ChainID, asset string) (*VaultInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.mapKey(chainID, asset)
	if vault, ok := s.vaults[key]; ok {
		return &vault, nil
	}
	return nil, ErrNotFound
}
