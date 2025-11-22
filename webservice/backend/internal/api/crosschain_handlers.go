package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/leafsii/leafsii-backend/internal/crosschain"
	"github.com/shopspring/decimal"
)

func (h *Handler) GetLatestCheckpoint(w http.ResponseWriter, r *http.Request) {
	chainID := r.URL.Query().Get("chainId")
	asset := r.URL.Query().Get("asset")
	if chainID == "" || asset == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "chainId and asset are required")
		return
	}

	cp, err := h.crosschainSvc.GetLatestCheckpoint(r.Context(), crosschain.ChainID(chainID), asset)
	if err != nil {
		if err == crosschain.ErrNotFound {
			h.writeError(w, http.StatusNotFound, "CHECKPOINT_NOT_FOUND", "no checkpoint available")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "CHECKPOINT_ERROR", err.Error())
		return
	}

	dto := WalrusCheckpointDTO{
		UpdateID:     cp.UpdateID,
		ChainID:      string(cp.ChainID),
		Asset:        cp.Asset,
		Vault:        cp.Vault,
		BlockNumber:  cp.BlockNumber,
		BlockHash:    cp.BlockHash,
		TotalShares:  cp.TotalShares.String(),
		Index:        cp.Index.String(),
		BalancesRoot: cp.BalancesRoot,
		ProofType:    cp.ProofType,
		WalrusBlobID: cp.WalrusBlobID,
		Status:       string(cp.Status),
		Timestamp:    cp.Timestamp.Unix(),
	}

	h.writeJSON(w, http.StatusOK, WalrusCheckpointResponse{Checkpoint: &dto})
}

func (h *Handler) SubmitCheckpoint(w http.ResponseWriter, r *http.Request) {
	var req SubmitCheckpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid checkpoint payload")
		return
	}

	totalShares, err := decimal.NewFromString(req.TotalShares)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_TOTAL_SHARES", "totalShares must be a decimal string")
		return
	}
	index, err := decimal.NewFromString(req.Index)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_INDEX", "index must be a decimal string")
		return
	}

	var proofBlob []byte
	if req.ProofBlob != "" {
		proofBlob, err = base64.StdEncoding.DecodeString(req.ProofBlob)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "INVALID_PROOF_BLOB", "proofBlob must be base64 encoded")
			return
		}
	}

	cp := crosschain.WalrusCheckpoint{
		ChainID:      crosschain.ChainID(req.ChainID),
		Asset:        req.Asset,
		Vault:        req.Vault,
		BlockNumber:  req.BlockNumber,
		BlockHash:    req.BlockHash,
		TotalShares:  totalShares,
		Index:        index,
		BalancesRoot: req.BalancesRoot,
		ProofType:    req.ProofType,
		ProofBlob:    proofBlob,
		WalrusBlobID: req.WalrusBlobID,
		Status:       crosschain.CheckpointStatusVerified,
		Timestamp:    time.Now(),
	}

	created, err := h.crosschainSvc.SubmitCheckpoint(r.Context(), cp)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "CHECKPOINT_ERROR", err.Error())
		return
	}

	dto := WalrusCheckpointDTO{
		UpdateID:     created.UpdateID,
		ChainID:      string(created.ChainID),
		Asset:        created.Asset,
		Vault:        created.Vault,
		BlockNumber:  created.BlockNumber,
		BlockHash:    created.BlockHash,
		TotalShares:  created.TotalShares.String(),
		Index:        created.Index.String(),
		BalancesRoot: created.BalancesRoot,
		ProofType:    created.ProofType,
		WalrusBlobID: created.WalrusBlobID,
		Status:       string(created.Status),
		Timestamp:    created.Timestamp.Unix(),
	}

	h.writeJSON(w, http.StatusCreated, WalrusCheckpointResponse{Checkpoint: &dto})
}

func (h *Handler) SubmitCrossChainDeposit(w http.ResponseWriter, r *http.Request) {
	if h.bridgeWorker == nil {
		h.writeError(w, http.StatusServiceUnavailable, "BRIDGE_UNAVAILABLE", "bridge worker not configured")
		return
	}

	var req BridgeDepositRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid deposit payload")
		return
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || !amount.GreaterThan(decimal.Zero) {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "amount must be a positive decimal string")
		return
	}

	receipt, err := h.bridgeWorker.Submit(r.Context(), crosschain.DepositSubmission{
		TxHash:   req.TxHash,
		SuiOwner: req.SuiOwner,
		ChainID:  crosschain.ChainID(req.ChainID),
		Asset:    req.Asset,
		Amount:   amount,
	})
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "BRIDGE_ERROR", err.Error())
		return
	}

	h.logger.Infow("Bridge deposit processed",
		"txHash", req.TxHash,
		"suiOwner", req.SuiOwner,
		"chainId", req.ChainID,
		"asset", req.Asset,
		"amount", amount.String(),
		"receiptId", receipt.ReceiptID,
	)

	dto := BridgeReceiptDTO{
		ReceiptID:    receipt.ReceiptID,
		TxHash:       receipt.TxHash,
		SuiOwner:     receipt.SuiOwner,
		ChainID:      string(receipt.ChainID),
		Asset:        receipt.Asset,
		Minted:       receipt.Minted,
		CreatedAt:    receipt.CreatedAt.Unix(),
		SuiTxDigests: receipt.SuiTxDigests,
	}

	h.writeJSON(w, http.StatusCreated, BridgeReceiptResponse{Receipt: dto})
}

func (h *Handler) SubmitCrossChainRedeem(w http.ResponseWriter, r *http.Request) {
	if h.bridgeWorker == nil {
		h.writeError(w, http.StatusServiceUnavailable, "BRIDGE_UNAVAILABLE", "bridge worker not configured")
		return
	}

	var req BridgeRedeemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid redeem payload")
		return
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || !amount.GreaterThan(decimal.Zero) {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "amount must be a positive decimal string")
		return
	}

	token := strings.ToLower(strings.TrimSpace(req.Token))
	if token != "f" && token != "x" {
		h.writeError(w, http.StatusBadRequest, "INVALID_TOKEN", "token must be 'f' or 'x'")
		return
	}

	receipt, err := h.bridgeWorker.Redeem(r.Context(), crosschain.RedeemSubmission{
		SuiTxDigest:  req.SuiTxDigest,
		SuiOwner:     req.SuiOwner,
		EthRecipient: req.EthRecipient,
		ChainID:      crosschain.ChainID(req.ChainID),
		Asset:        req.Asset,
		Token:        token,
		Amount:       amount,
	})
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "BRIDGE_ERROR", err.Error())
		return
	}

	h.logger.Infow("Bridge redeem processed",
		"suiTxDigest", req.SuiTxDigest,
		"suiOwner", req.SuiOwner,
		"ethRecipient", req.EthRecipient,
		"chainId", req.ChainID,
		"asset", req.Asset,
		"token", token,
		"amount", amount.String(),
		"receiptId", receipt.ReceiptID,
	)

	dto := RedeemReceiptDTO{
		ReceiptID:      receipt.ReceiptID,
		SuiTxDigest:    receipt.SuiTxDigest,
		SuiOwner:       receipt.SuiOwner,
		EthRecipient:   receipt.EthRecipient,
		ChainID:        string(receipt.ChainID),
		Asset:          receipt.Asset,
		Token:          receipt.Token,
		Burned:         receipt.Burned,
		PayoutEth:      receipt.PayoutEth,
		WalrusUpdateID: receipt.WalrusUpdateID,
		WalrusBlobID:   receipt.WalrusBlobID,
		PayoutTxHash:   receipt.PayoutTxHash,
		CreatedAt:      receipt.CreatedAt.Unix(),
	}

	h.writeJSON(w, http.StatusCreated, RedeemReceiptResponse{Receipt: dto})
}

func (h *Handler) GetCrossChainBalance(w http.ResponseWriter, r *http.Request) {
	suiOwner := r.URL.Query().Get("suiOwner")
	chainID := r.URL.Query().Get("chainId")
	asset := r.URL.Query().Get("asset")
	if suiOwner == "" || chainID == "" || asset == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "suiOwner, chainId, and asset are required")
		return
	}

	balance, err := h.crosschainSvc.GetBalance(r.Context(), suiOwner, crosschain.ChainID(chainID), asset)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "BALANCE_ERROR", err.Error())
		return
	}

	dto := CrossChainBalanceDTO{
		SuiOwner:         balance.SuiOwner,
		ChainID:          string(balance.ChainID),
		Asset:            balance.Asset,
		Shares:           balance.Shares.String(),
		Index:            balance.Index.String(),
		Value:            balance.Value.String(),
		CollateralUSD:    balance.CollateralUSD.String(),
		LastCheckpointID: balance.LastCheckpointID,
		UpdatedAt:        balance.UpdatedAt.Unix(),
	}

	h.writeJSON(w, http.StatusOK, CrossChainBalanceResponse{Balance: dto})
}

func (h *Handler) CreateVoucher(w http.ResponseWriter, r *http.Request) {
	var req CreateVoucherRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid voucher payload")
		return
	}

	shares, err := decimal.NewFromString(req.Shares)
	if err != nil || !shares.GreaterThan(decimal.Zero) {
		h.writeError(w, http.StatusBadRequest, "INVALID_SHARES", "shares must be a positive decimal string")
		return
	}

	expiry := time.Unix(req.Expiry, 0)
	voucher, err := h.crosschainSvc.CreateVoucher(r.Context(), crosschain.WithdrawalVoucher{
		SuiOwner:  req.SuiOwner,
		ChainID:   crosschain.ChainID(req.ChainID),
		Asset:     req.Asset,
		Shares:    shares,
		Expiry:    expiry,
		Status:    crosschain.VoucherStatusPending,
		CreatedAt: time.Now(),
	})
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "VOUCHER_ERROR", err.Error())
		return
	}

	dto := VoucherDTO{
		VoucherID: voucher.VoucherID,
		SuiOwner:  voucher.SuiOwner,
		ChainID:   string(voucher.ChainID),
		Asset:     voucher.Asset,
		Shares:    voucher.Shares.String(),
		Nonce:     voucher.Nonce,
		Expiry:    voucher.Expiry.Unix(),
		Status:    string(voucher.Status),
		TxHash:    voucher.TxHash,
		CreatedAt: voucher.CreatedAt.Unix(),
	}

	h.writeJSON(w, http.StatusCreated, VoucherResponse{Voucher: &dto})
}

func (h *Handler) ListVouchers(w http.ResponseWriter, r *http.Request) {
	suiOwner := r.URL.Query().Get("suiOwner")
	if suiOwner == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "suiOwner is required")
		return
	}

	vouchers, err := h.crosschainSvc.ListVouchers(r.Context(), suiOwner)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "VOUCHER_ERROR", err.Error())
		return
	}

	resp := VoucherListResponse{Vouchers: make([]VoucherDTO, 0, len(vouchers))}
	for _, v := range vouchers {
		resp.Vouchers = append(resp.Vouchers, VoucherDTO{
			VoucherID: v.VoucherID,
			SuiOwner:  v.SuiOwner,
			ChainID:   string(v.ChainID),
			Asset:     v.Asset,
			Shares:    v.Shares.String(),
			Nonce:     v.Nonce,
			Expiry:    v.Expiry.Unix(),
			Status:    string(v.Status),
			TxHash:    v.TxHash,
			CreatedAt: v.CreatedAt.Unix(),
		})
	}

	h.writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetVoucher(w http.ResponseWriter, r *http.Request) {
	voucherID := r.URL.Query().Get("voucherId")
	if voucherID == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "voucherId is required")
		return
	}

	voucher, err := h.crosschainSvc.GetVoucher(r.Context(), voucherID)
	if err != nil {
		if err == crosschain.ErrNotFound {
			h.writeError(w, http.StatusNotFound, "VOUCHER_NOT_FOUND", "voucher not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "VOUCHER_ERROR", err.Error())
		return
	}

	dto := VoucherDTO{
		VoucherID: voucher.VoucherID,
		SuiOwner:  voucher.SuiOwner,
		ChainID:   string(voucher.ChainID),
		Asset:     voucher.Asset,
		Shares:    voucher.Shares.String(),
		Nonce:     voucher.Nonce,
		Expiry:    voucher.Expiry.Unix(),
		Status:    string(voucher.Status),
		TxHash:    voucher.TxHash,
		CreatedAt: voucher.CreatedAt.Unix(),
	}

	h.writeJSON(w, http.StatusOK, VoucherResponse{Voucher: &dto})
}

func (h *Handler) GetCollateralParams(w http.ResponseWriter, r *http.Request) {
	chainID := r.URL.Query().Get("chainId")
	asset := r.URL.Query().Get("asset")
	if chainID == "" || asset == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "chainId and asset are required")
		return
	}

	params, err := h.crosschainSvc.GetCollateralParams(r.Context(), crosschain.ChainID(chainID), asset)
	if err != nil {
		if err == crosschain.ErrNotFound {
			h.writeError(w, http.StatusNotFound, "PARAMS_NOT_FOUND", "collateral params not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "PARAMS_ERROR", err.Error())
		return
	}

	dto := CollateralParamsDTO{
		ChainID:              string(params.ChainID),
		Asset:                params.Asset,
		LTV:                  params.LTV.String(),
		MaintenanceThreshold: params.MaintenanceThreshold.String(),
		LiquidationPenalty:   params.LiquidationPenalty.String(),
		OracleHaircut:        params.OracleHaircut.String(),
		StalenessHardCap:     int64(params.StalenessHardCap),
		MintRateLimit:        params.MintRateLimit.String(),
		WithdrawRateLimit:    params.WithdrawRateLimit.String(),
		Active:               params.Active,
	}

	h.writeJSON(w, http.StatusOK, CollateralParamsResponse{Params: &dto})
}

func (h *Handler) GetVaultInfo(w http.ResponseWriter, r *http.Request) {
	chainID := r.URL.Query().Get("chainId")
	asset := r.URL.Query().Get("asset")
	if chainID == "" || asset == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "chainId and asset are required")
		return
	}

	vault, err := h.crosschainSvc.GetVault(r.Context(), crosschain.ChainID(chainID), asset)
	if err != nil {
		if err == crosschain.ErrNotFound {
			h.writeError(w, http.StatusNotFound, "VAULT_NOT_FOUND", "vault not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "VAULT_ERROR", err.Error())
		return
	}

	dto := VaultInfoDTO{
		ChainID:           string(vault.ChainID),
		Asset:             vault.Asset,
		VaultAddress:      vault.VaultAddress,
		DepositMemoFormat: vault.DepositMemoFormat,
		FeedURL:           vault.FeedURL,
		ProofCID:          vault.ProofCID,
		SnapshotURL:       vault.SnapshotURL,
	}

	h.writeJSON(w, http.StatusOK, VaultInfoResponse{Vault: &dto})
}

func (h *Handler) ListMarkets(w http.ResponseWriter, _ *http.Request) {
	if h.marketsSvc == nil {
		h.writeError(w, http.StatusInternalServerError, "MARKETS_ERROR", "markets service unavailable")
		return
	}
	h.writeJSON(w, http.StatusOK, h.marketsSvc.List())
}
