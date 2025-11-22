package crosschain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/leafsii/leafsii-backend/internal/prices/binance"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// DepositSubmission represents a user-submitted EVM deposit that should be bridged to Sui.
type DepositSubmission struct {
	TxHash   string
	SuiOwner string
	ChainID  ChainID
	Asset    string
	Amount   decimal.Decimal
}

// BridgeReceipt is returned after a deposit has been processed by the bridge worker.
type BridgeReceipt struct {
	ReceiptID    string    `json:"receiptId"`
	TxHash       string    `json:"txHash"`
	SuiOwner     string    `json:"suiOwner"`
	ChainID      ChainID   `json:"chainId"`
	Asset        string    `json:"asset"`
	Minted       string    `json:"minted"`
	CreatedAt    time.Time `json:"createdAt"`
	SuiTxDigests []string  `json:"suiTxDigests,omitempty"`
}

// RedeemSubmission represents a burn on Sui requesting an EVM payout.
type RedeemSubmission struct {
	SuiTxDigest  string
	SuiOwner     string
	EthRecipient string
	ChainID      ChainID
	Asset        string
	Token        string // "f" or "x"
	Amount       decimal.Decimal
}

// RedeemReceipt is returned after a redeem has been processed by the bridge worker.
type RedeemReceipt struct {
	ReceiptID      string    `json:"receiptId"`
	SuiTxDigest    string    `json:"suiTxDigest"`
	SuiOwner       string    `json:"suiOwner"`
	EthRecipient   string    `json:"ethRecipient"`
	ChainID        ChainID   `json:"chainId"`
	Asset          string    `json:"asset"`
	Token          string    `json:"token"`
	Burned         string    `json:"burned"`
	PayoutEth      string    `json:"payoutEth"`
	WalrusUpdateID uint64    `json:"walrusUpdateId,omitempty"`
	WalrusBlobID   string    `json:"walrusBlobId,omitempty"`
	PayoutTxHash   string    `json:"payoutTxHash,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

type bridgeJob struct {
	submission DepositSubmission
	result     chan result
}

type result struct {
	receipt *BridgeReceipt
	err     error
}

// BridgeMintContext carries Walrus-derived state into a mint handler.
type BridgeMintContext struct {
	Submission DepositSubmission
	Checkpoint *WalrusCheckpoint
	Balance    *CrossChainBalance
	NewShares  decimal.Decimal
	MintF      uint64
	MintX      uint64
	PriceUSD   decimal.Decimal
}

// MintResult captures on-chain artifacts from a mint handler.
type MintResult struct {
	TxDigests []string
}

// MintHandler can perform an on-chain mint/transfer for a deposit submission.
type MintHandler interface {
	Mint(ctx context.Context, mintCtx BridgeMintContext) (*MintResult, error)
}

// PayoutHandler can fulfill a Sui-side burn by paying out on the origin chain.
type PayoutHandler interface {
	Payout(ctx context.Context, payout RedeemPayoutContext) (string, error)
}

// RedeemPayoutContext carries computed payout details for a redemption.
type RedeemPayoutContext struct {
	SuiOwner     string
	EthRecipient string
	ChainID      ChainID
	Asset        string
	Token        string
	BurnAmount   decimal.Decimal
	PayoutEth    decimal.Decimal
	PriceUSD     decimal.Decimal
}

// WalrusPublisher persists checkpoints to Walrus DA and returns the blob ID.
type WalrusPublisher interface {
	Publish(ctx context.Context, cp WalrusCheckpoint) (string, error)
}

// BridgeWorkerOption configures a BridgeWorker.
type BridgeWorkerOption func(*BridgeWorker)

// WithMintHandler configures the worker to invoke a mint handler for each submission.
func WithMintHandler(h MintHandler) BridgeWorkerOption {
	return func(w *BridgeWorker) {
		w.mintHandler = h
	}
}

// WithPayoutHandler configures the worker to execute payouts for bridge redeems.
func WithPayoutHandler(h PayoutHandler) BridgeWorkerOption {
	return func(w *BridgeWorker) {
		w.payoutHandler = h
	}
}

// WithWalrusPublisher configures the worker to publish checkpoints to Walrus.
func WithWalrusPublisher(p WalrusPublisher) BridgeWorkerOption {
	return func(w *BridgeWorker) {
		w.walrusPublisher = p
	}
}

// WithRedeemListener configures the worker to listen for bridge_redeem events.
func WithRedeemListener(l RedeemListener) BridgeWorkerOption {
	return func(w *BridgeWorker) {
		w.redeemListener = l
	}
}

// BridgeWorker consumes deposit submissions and mints balances on Sui (via the crosschain Service).
type BridgeWorker struct {
	svc             *Service
	logger          *zap.SugaredLogger
	jobs            chan bridgeJob
	counter         uint64
	mintHandler     MintHandler
	payoutHandler   PayoutHandler
	redeemListener  RedeemListener
	walrusPublisher WalrusPublisher
}

func NewBridgeWorker(svc *Service, logger *zap.SugaredLogger, opts ...BridgeWorkerOption) *BridgeWorker {
	w := &BridgeWorker{
		svc:    svc,
		logger: logger,
		jobs:   make(chan bridgeJob, 64),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Start spins up the worker loop; call once during application startup.
func (w *BridgeWorker) Start(ctx context.Context) {
	w.logger.Infow("Bridge worker starting")

	if w.redeemListener != nil {
		if err := w.redeemListener.Start(ctx, func(evCtx context.Context, sub RedeemSubmission) {
			go func() {
				if _, err := w.Redeem(evCtx, sub); err != nil {
					w.logger.Warnw("Bridge redeem failed", "error", err, "suiTxDigest", sub.SuiTxDigest)
				}
			}()
		}); err != nil {
			w.logger.Warnw("Bridge redeem listener failed to start", "error", err)
		}
	}

	go func() {
		defer w.logger.Infow("Bridge worker stopped")
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-w.jobs:
				receipt, err := w.handle(ctx, job.submission)
				job.result <- result{receipt: receipt, err: err}
			}
		}
	}()
}

// Submit enqueues a deposit for processing and waits for the bridge receipt.
func (w *BridgeWorker) Submit(ctx context.Context, sub DepositSubmission) (*BridgeReceipt, error) {
	if sub.SuiOwner == "" || sub.Asset == "" || sub.ChainID == "" || !sub.Amount.GreaterThan(decimal.Zero) {
		return nil, ErrInvalidRequest
	}

	w.logger.Infow("Bridge worker received deposit submission",
		"txHash", sub.TxHash,
		"suiOwner", sub.SuiOwner,
		"chainId", sub.ChainID,
		"asset", sub.Asset,
		"amount", sub.Amount.String(),
	)

	job := bridgeJob{
		submission: sub,
		result:     make(chan result, 1),
	}

	select {
	case w.jobs <- job:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case res := <-job.result:
		return res.receipt, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Redeem processes a burn on Sui and initiates an origin-chain payout.
func (w *BridgeWorker) Redeem(ctx context.Context, sub RedeemSubmission) (*RedeemReceipt, error) {
	token := strings.ToLower(strings.TrimSpace(sub.Token))
	if sub.SuiOwner == "" || sub.Asset == "" || sub.ChainID == "" || sub.EthRecipient == "" || !sub.Amount.GreaterThan(decimal.Zero) || (token != "f" && token != "x") {
		return nil, ErrInvalidRequest
	}

	priceUSD, err := w.fetchUSDPrice(ctx, sub.ChainID, sub.Asset)
	if err != nil {
		return nil, fmt.Errorf("fetch price: %w", err)
	}

	var (
		payoutEth  decimal.Decimal
		burnShares = sub.Amount
	)

	switch token {
	case "f":
		payoutEth = sub.Amount.Div(priceUSD)
	case "x":
		payoutEth = sub.Amount
	}

	if !payoutEth.GreaterThan(decimal.Zero) {
		return nil, fmt.Errorf("invalid payout computed from %s %s", sub.Amount.String(), token)
	}

	cp, bal, err := w.updateWalrusCheckpointForRedeem(ctx, sub, burnShares)
	if err != nil {
		return nil, fmt.Errorf("update walrus: %w", err)
	}

	id := atomic.AddUint64(&w.counter, 1)
	receipt := &RedeemReceipt{
		ReceiptID:    fmt.Sprintf("redeem_%d", id),
		SuiTxDigest:  sub.SuiTxDigest,
		SuiOwner:     sub.SuiOwner,
		EthRecipient: sub.EthRecipient,
		ChainID:      sub.ChainID,
		Asset:        sub.Asset,
		Token:        token,
		Burned:       sub.Amount.String(),
		PayoutEth:    payoutEth.String(),
		CreatedAt:    time.Now(),
	}
	if cp != nil {
		receipt.WalrusUpdateID = cp.UpdateID
		receipt.WalrusBlobID = cp.WalrusBlobID
	}

	if w.payoutHandler != nil {
		if txHash, err := w.payoutHandler.Payout(ctx, RedeemPayoutContext{
			SuiOwner:     sub.SuiOwner,
			EthRecipient: sub.EthRecipient,
			ChainID:      sub.ChainID,
			Asset:        sub.Asset,
			Token:        token,
			BurnAmount:   sub.Amount,
			PayoutEth:    payoutEth,
			PriceUSD:     priceUSD,
		}); err != nil {
			return nil, fmt.Errorf("payout handler: %w", err)
		} else {
			receipt.PayoutTxHash = txHash
		}
	}

	w.logger.Infow("Bridge redeem processed",
		"receiptId", receipt.ReceiptID,
		"suiOwner", sub.SuiOwner,
		"ethRecipient", sub.EthRecipient,
		"token", token,
		"burnAmount", sub.Amount.String(),
		"payoutEth", payoutEth.String(),
		"priceUSD", priceUSD.String(),
		"walrusUpdateId", receipt.WalrusUpdateID,
		"walrusBlobId", receipt.WalrusBlobID,
		"newShares", bal.Shares.String(),
		"value", bal.Value.String(),
	)

	return receipt, nil
}

func (w *BridgeWorker) handle(ctx context.Context, sub DepositSubmission) (*BridgeReceipt, error) {
	priceUSD, err := w.fetchUSDPrice(ctx, sub.ChainID, sub.Asset)
	if err != nil {
		return nil, fmt.Errorf("fetch price: %w", err)
	}

	mintF, mintX, mintShares, err := splitMintAmounts(sub.Amount, priceUSD)
	if err != nil {
		return nil, fmt.Errorf("mint split: %w", err)
	}

	subForMint := sub
	subForMint.Amount = mintShares

	cp, bal, err := w.updateWalrusCheckpoint(ctx, subForMint)
	if err != nil {
		return nil, fmt.Errorf("update walrus: %w", err)
	}

	id := atomic.AddUint64(&w.counter, 1)
	receipt := &BridgeReceipt{
		ReceiptID: fmt.Sprintf("bridge_%d", id),
		TxHash:    sub.TxHash,
		SuiOwner:  sub.SuiOwner,
		ChainID:   sub.ChainID,
		Asset:     sub.Asset,
		Minted:    fmt.Sprintf("f=%s,x=%s", mintF.StringFixed(9), mintX.StringFixed(9)),
		CreatedAt: time.Now(),
	}

	w.logger.Infow("Bridge deposit minted",
		"receiptId", receipt.ReceiptID,
		"suiOwner", sub.SuiOwner,
		"asset", sub.Asset,
		"chainId", sub.ChainID,
		"amountEth", sub.Amount.String(),
		"priceUSD", priceUSD.String(),
		"fMinted", mintF.StringFixed(9),
		"xMinted", mintX.StringFixed(9),
		"txHash", sub.TxHash,
		"newShares", bal.Shares.String(),
		"value", bal.Value.String(),
		"walrusUpdateId", cp.UpdateID,
		"walrusBlobId", cp.WalrusBlobID,
		"walrusIndex", cp.Index.String(),
		"walrusBalancesRoot", cp.BalancesRoot,
	)

	if w.mintHandler != nil {
		mintResult, err := w.mintHandler.Mint(ctx, BridgeMintContext{
			Submission: subForMint,
			Checkpoint: cp,
			Balance:    bal,
			NewShares:  mintShares,
			MintF:      toUint(mintF),
			MintX:      toUint(mintX),
			PriceUSD:   priceUSD,
		})
		if err != nil {
			return nil, fmt.Errorf("mint handler: %w", err)
		}
		if mintResult != nil && len(mintResult.TxDigests) > 0 {
			receipt.SuiTxDigests = append([]string{}, mintResult.TxDigests...)
		}
	}

	return receipt, nil
}

// fetchUSDPrice pulls the latest USD price for the given chain/asset from Binance.
func (w *BridgeWorker) fetchUSDPrice(ctx context.Context, chainID ChainID, asset string) (decimal.Decimal, error) {
	asset = strings.ToUpper(strings.TrimSpace(asset))
	symbol := ""

	switch {
	case chainID == ChainIDEthereum && asset == "ETH":
		symbol = "ETHUSDT"
	default:
		return decimal.Zero, fmt.Errorf("unsupported asset for price fetch: %s:%s", chainID, asset)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v3/ticker/price?symbol=%s", binance.BinanceRestAPI, symbol), nil)
	if err != nil {
		return decimal.Zero, fmt.Errorf("build price request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return decimal.Zero, fmt.Errorf("price request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return decimal.Zero, fmt.Errorf("price request returned %d", resp.StatusCode)
	}

	var payload struct {
		Price string `json:"price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return decimal.Zero, fmt.Errorf("decode price response: %w", err)
	}

	price, err := decimal.NewFromString(payload.Price)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse price: %w", err)
	}
	if !price.GreaterThan(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("invalid price %s", payload.Price)
	}

	return price, nil
}

// splitMintAmounts mirrors init_protocol's 50/50 USD split: half to fToken (Pf fixed at 1),
// half to xToken at current price. Returns token amounts in whole-token decimals (not 1e9 units).
func splitMintAmounts(depositAsset decimal.Decimal, priceUSD decimal.Decimal) (decimal.Decimal, decimal.Decimal, decimal.Decimal, error) {
	if depositAsset.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, decimal.Zero, decimal.Zero, fmt.Errorf("deposit must be positive")
	}
	if !priceUSD.GreaterThan(decimal.Zero) {
		return decimal.Zero, decimal.Zero, decimal.Zero, fmt.Errorf("price must be positive")
	}

	usdValue := depositAsset.Mul(priceUSD)
	usdHalf := usdValue.Div(decimal.NewFromInt(2))

	// fToken: 1 USD per token.
	mintF := usdHalf

	// xToken: USD half divided by price.
	mintX := usdHalf.Div(priceUSD)

	mintShares := mintF.Add(mintX)
	return mintF, mintX, mintShares, nil
}

func toUint(v decimal.Decimal) uint64 {
	if v.LessThanOrEqual(decimal.Zero) {
		return 0
	}
	// Tokens use 9 decimals on Sui.
	withScale := v.Mul(decimal.New(1, 9))
	i := withScale.Truncate(0)
	if b := i.BigInt(); b != nil && b.IsUint64() {
		return b.Uint64()
	}
	return 0
}

// updateWalrusCheckpointForRedeem publishes a synthetic checkpoint for a burn
// and debits the user's balance before triggering a payout.
func (w *BridgeWorker) updateWalrusCheckpointForRedeem(ctx context.Context, sub RedeemSubmission, burnShares decimal.Decimal) (*WalrusCheckpoint, *CrossChainBalance, error) {
	if burnShares.LessThanOrEqual(decimal.Zero) {
		return nil, nil, ErrInvalidRequest
	}

	now := time.Now()
	var (
		totalShares decimal.Decimal
		index              = decimal.NewFromInt(1)
		blockNumber uint64 = 1
		blockHash          = sub.SuiTxDigest
	)

	last, err := w.svc.GetLatestCheckpoint(ctx, sub.ChainID, sub.Asset)
	if err != nil && err != ErrNotFound {
		return nil, nil, fmt.Errorf("latest checkpoint: %w", err)
	}
	if last != nil {
		blockNumber = last.BlockNumber + 1
		if !last.Index.IsZero() {
			index = last.Index
		}
		if blockHash == "" {
			blockHash = last.BlockHash
		}
		totalShares = last.TotalShares.Sub(burnShares)
		if totalShares.LessThan(decimal.Zero) {
			return nil, nil, fmt.Errorf("burn exceeds tracked shares")
		}
	} else {
		totalShares = decimal.Zero
	}

	vaultAddr := ""
	if vault, err := w.svc.GetVault(ctx, sub.ChainID, sub.Asset); err == nil {
		vaultAddr = vault.VaultAddress
	}

	cp := WalrusCheckpoint{
		ChainID:      sub.ChainID,
		Asset:        sub.Asset,
		Vault:        vaultAddr,
		BlockNumber:  blockNumber,
		BlockHash:    blockHash,
		TotalShares:  totalShares,
		Index:        index,
		BalancesRoot: balancesRootForOwner(sub.SuiOwner, sub.ChainID, sub.Asset, totalShares, blockNumber, blockHash),
		ProofType:    "walrus",
		Status:       CheckpointStatusVerified,
		Timestamp:    now,
	}

	if w.walrusPublisher != nil {
		if blobID, err := w.walrusPublisher.Publish(ctx, cp); err == nil && blobID != "" {
			cp.WalrusBlobID = blobID
		} else if err != nil {
			w.logger.Warnw("Walrus publish failed; falling back to synthetic blob id", "error", err)
		}
	}
	if cp.WalrusBlobID == "" {
		cp.WalrusBlobID = fmt.Sprintf("walrus-%s-%s-%d", sub.ChainID, sub.Asset, now.UnixNano())
	}

	created, err := w.svc.SubmitCheckpoint(ctx, cp)
	if err != nil {
		return nil, nil, fmt.Errorf("submit checkpoint: %w", err)
	}

	bal, err := w.svc.DebitWithdrawal(ctx, sub.SuiOwner, sub.ChainID, sub.Asset, burnShares)
	if err != nil {
		return nil, nil, fmt.Errorf("debit withdrawal: %w", err)
	}

	return created, bal, nil
}

// updateWalrusCheckpoint publishes a synthetic Walrus checkpoint for the vault and
// revalues the user's balance against that checkpoint before minting on Sui.
func (w *BridgeWorker) updateWalrusCheckpoint(ctx context.Context, sub DepositSubmission) (*WalrusCheckpoint, *CrossChainBalance, error) {
	now := time.Now()
	var (
		totalShares        = sub.Amount
		index              = decimal.NewFromInt(1)
		blockNumber uint64 = 1
		blockHash          = sub.TxHash
	)

	last, err := w.svc.GetLatestCheckpoint(ctx, sub.ChainID, sub.Asset)
	if err != nil && err != ErrNotFound {
		return nil, nil, fmt.Errorf("latest checkpoint: %w", err)
	}
	if last != nil {
		totalShares = last.TotalShares.Add(sub.Amount)
		blockNumber = last.BlockNumber + 1
		if !last.Index.IsZero() {
			index = last.Index
		}
		if blockHash == "" {
			blockHash = last.BlockHash
		}
	}

	vaultAddr := ""
	if vault, err := w.svc.GetVault(ctx, sub.ChainID, sub.Asset); err == nil {
		vaultAddr = vault.VaultAddress
	}

	cp := WalrusCheckpoint{
		ChainID:      sub.ChainID,
		Asset:        sub.Asset,
		Vault:        vaultAddr,
		BlockNumber:  blockNumber,
		BlockHash:    blockHash,
		TotalShares:  totalShares,
		Index:        index,
		BalancesRoot: balancesRootForOwner(sub.SuiOwner, sub.ChainID, sub.Asset, totalShares, blockNumber, blockHash),
		ProofType:    "walrus",
		Status:       CheckpointStatusVerified,
		Timestamp:    now,
	}

	if w.walrusPublisher != nil {
		if blobID, err := w.walrusPublisher.Publish(ctx, cp); err == nil && blobID != "" {
			cp.WalrusBlobID = blobID
		} else if err != nil {
			w.logger.Warnw("Walrus publish failed; falling back to synthetic blob id", "error", err)
		}
	}
	if cp.WalrusBlobID == "" {
		cp.WalrusBlobID = fmt.Sprintf("walrus-%s-%s-%d", sub.ChainID, sub.Asset, now.UnixNano())
	}

	created, err := w.svc.SubmitCheckpoint(ctx, cp)
	if err != nil {
		return nil, nil, fmt.Errorf("submit checkpoint: %w", err)
	}

	// Update user balance against the fresh Walrus index.
	bal, err := w.svc.CreditDeposit(ctx, sub.SuiOwner, sub.ChainID, sub.Asset, sub.Amount)
	if err != nil {
		return nil, nil, fmt.Errorf("credit deposit: %w", err)
	}

	return created, bal, nil
}

func balancesRootForOwner(owner string, chainID ChainID, asset string, totalShares decimal.Decimal, blockNumber uint64, blockHash string) string {
	payload := fmt.Sprintf("%s:%s:%s:%s:%d:%s", owner, chainID, asset, totalShares.String(), blockNumber, blockHash)
	h := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("0x%x", h[:])
}

// HTTPWalrusPublisher posts checkpoints to a Walrus gateway and expects a JSON id response.
type HTTPWalrusPublisher struct {
	Endpoint     string
	Client       *http.Client
	Epochs       int
	SendObjectTo string
}

func (p *HTTPWalrusPublisher) Publish(ctx context.Context, cp WalrusCheckpoint) (string, error) {
	if p == nil || p.Endpoint == "" {
		return "", fmt.Errorf("walrus endpoint not configured")
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}

	body, err := json.Marshal(cp)
	if err != nil {
		return "", fmt.Errorf("marshal checkpoint: %w", err)
	}

	epochs := p.Epochs
	if epochs <= 0 {
		epochs = 1
	}
	u, err := url.Parse(p.Endpoint)
	if err != nil {
		return "", fmt.Errorf("parse walrus endpoint: %w", err)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/blobs"
	}
	q := u.Query()
	q.Set("epochs", fmt.Sprintf("%d", epochs))
	if p.SendObjectTo != "" {
		q.Set("send_object_to", p.SendObjectTo)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build walrus request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("walrus post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("walrus post status %d", resp.StatusCode)
	}

	var parsed struct {
		ID     string `json:"id"`
		BlobID string `json:"blobId"`
		Cid    string `json:"cid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err == nil {
		if parsed.ID != "" {
			return parsed.ID, nil
		}
		if parsed.BlobID != "" {
			return parsed.BlobID, nil
		}
		if parsed.Cid != "" {
			return parsed.Cid, nil
		}
	}

	return "", nil
}
