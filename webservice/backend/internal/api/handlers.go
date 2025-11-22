package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/leafsii/leafsii-backend/internal/calc"
	"github.com/leafsii/leafsii-backend/internal/config"
	"github.com/leafsii/leafsii-backend/internal/crosschain"
	"github.com/leafsii/leafsii-backend/internal/markets"
	"github.com/leafsii/leafsii-backend/internal/onchain"
	"github.com/leafsii/leafsii-backend/internal/store"
	"github.com/leafsii/leafsii-backend/internal/ws"
	"github.com/pattonkan/sui-go/sui"
	"github.com/pattonkan/sui-go/utils/unit"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// MetricsInterface defines the interface for metrics recording
type MetricsInterface interface {
	RecordHTTPRequest(ctx context.Context, method, path string, status int, duration time.Duration)
}

type Handler struct {
	protocolSvc   *onchain.ProtocolService
	quoteSvc      *onchain.QuoteService
	userSvc       *onchain.UserService
	spSvc         *onchain.StabilityPoolService
	crosschainSvc *crosschain.Service
	bridgeWorker  *crosschain.BridgeWorker
	marketsSvc    *markets.Service
	wsHub         *ws.Hub
	sseHandler    *ws.SSEHandler
	cache         *store.Cache
	config        *config.Config
	logger        *zap.SugaredLogger
	metrics       MetricsInterface
	txBuilder     onchain.TransactionBuilderInterface
	txSubmitter   onchain.TransactionSubmitterInterface
}

func NewHandler(
	protocolSvc *onchain.ProtocolService,
	quoteSvc *onchain.QuoteService,
	userSvc *onchain.UserService,
	spSvc *onchain.StabilityPoolService,
	crosschainSvc *crosschain.Service,
	bridgeWorker *crosschain.BridgeWorker,
	marketsSvc *markets.Service,
	wsHub *ws.Hub,
	sseHandler *ws.SSEHandler,
	cache *store.Cache,
	config *config.Config,
	logger *zap.SugaredLogger,
	metrics MetricsInterface,
	txBuilder onchain.TransactionBuilderInterface,
	txSubmitter onchain.TransactionSubmitterInterface,
) *Handler {
	return &Handler{
		protocolSvc:   protocolSvc,
		quoteSvc:      quoteSvc,
		userSvc:       userSvc,
		spSvc:         spSvc,
		crosschainSvc: crosschainSvc,
		bridgeWorker:  bridgeWorker,
		marketsSvc:    marketsSvc,
		wsHub:         wsHub,
		sseHandler:    sseHandler,
		cache:         cache,
		config:        config,
		logger:        logger,
		metrics:       metrics,
		txBuilder:     txBuilder,
		txSubmitter:   txSubmitter,
	}
}

// Protocol endpoints
func (h *Handler) GetProtocolState(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	state, err := h.protocolSvc.GetState(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "PROTOCOL_STATE_ERROR", err.Error())
		return
	}

	dto := ProtocolStateDTO{
		CR:           state.CR.String(),
		CRTarget:     state.CRTarget.String(),
		ReservesR:    state.ReservesR.String(),
		SupplyF:      state.SupplyF.String(),
		SupplyX:      state.SupplyX.String(),
		Px:           state.Px,
		PegDeviation: state.PegDeviation.String(),
		OracleAgeSec: state.OracleAgeSec,
		Mode:         state.Mode,
		AsOf:         state.AsOf.Unix(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

func (h *Handler) GetProtocolHealth(w http.ResponseWriter, r *http.Request) {
	health, err := h.protocolSvc.GetHealth(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "HEALTH_CHECK_ERROR", err.Error())
		return
	}

	status := "ok"
	if len(health.Reasons) > 0 {
		status = "warn"
		// Check for critical issues
		for _, reason := range health.Reasons {
			if reason == "CR_BELOW_MINIMUM" || reason == "ORACLE_STALE" {
				status = "danger"
				break
			}
		}
	}

	dto := HealthDTO{
		Status:  status,
		Reasons: health.Reasons,
	}

	h.writeJSON(w, http.StatusOK, dto)
}

// GetTransactionBuildInfo returns the essential IDs needed for transaction building
func (h *Handler) GetTransactionBuildInfo(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	// Get all the required IDs from config
	packageId, err := h.config.Sui.GetLeafsiiPackageId()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "CONFIG_ERROR", fmt.Sprintf("Failed to get package ID: %v", err))
		return
	}

	protocolId, err := h.config.Sui.GetProtocolId()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "CONFIG_ERROR", fmt.Sprintf("Failed to get protocol ID: %v", err))
		return
	}

	poolId, err := h.config.Sui.GetPoolId()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "CONFIG_ERROR", fmt.Sprintf("Failed to get pool ID: %v", err))
		return
	}

	ftokenPackageId, err := h.config.Sui.GetFtokenPackageId()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "CONFIG_ERROR", fmt.Sprintf("Failed to get ftoken package ID: %v", err))
		return
	}

	xtokenPackageId, err := h.config.Sui.GetXtokenPackageId()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "CONFIG_ERROR", fmt.Sprintf("Failed to get xtoken package ID: %v", err))
		return
	}

	adminCapId, err := h.config.Sui.GetAdminCapId()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "CONFIG_ERROR", fmt.Sprintf("Failed to get admin cap ID: %v", err))
		return
	}

	dto := TransactionBuildInfoResponse{
		PackageId:       packageId.String(),
		ProtocolId:      protocolId.String(),
		PoolId:          poolId.String(),
		FtokenPackageId: ftokenPackageId.String(),
		XtokenPackageId: xtokenPackageId.String(),
		AdminCapId:      adminCapId.String(),
		FtokenTreasuryCapId: h.config.Sui.FTTreasuryCapId,
		XtokenTreasuryCapId: h.config.Sui.XTTreasuryCapId,
		FtokenAuthorityId:   h.config.Sui.FTAuthorityId,
		XtokenAuthorityId:   h.config.Sui.XTAuthorityId,
		Network:         h.config.Sui.Network,
		RpcUrl:          h.config.Sui.RPCURL,
		WsUrl:           h.config.Sui.WSURL,
		EvmRpcUrl:       getEvmRpcForNetwork(h.config.Sui.Network),
		EvmChainId:      getEvmChainId(h.config.Sui.Network),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

// Prefer explicit env overrides; fall back to Sepolia RPC for testnet and Mainnet Infura-style placeholder for mainnet.
func getEvmRpcForNetwork(network string) string {
	switch network {
	case "mainnet":
		if v := strings.TrimSpace(os.Getenv("LFS_EVM_MAINNET_RPC_URL")); v != "" {
			return v
		}
		if v := strings.TrimSpace(os.Getenv("LFS_ETH_RPC_URL")); v != "" {
			return v
		}
		return ""
	case "testnet":
		if v := strings.TrimSpace(os.Getenv("LFS_EVM_TESTNET_RPC_URL")); v != "" {
			return v
		}
		if v := strings.TrimSpace(os.Getenv("LFS_SEPOLIA_RPC_URL")); v != "" {
			return v
		}
		return ""
	default:
		if v := strings.TrimSpace(os.Getenv("LFS_EVM_LOCAL_RPC_URL")); v != "" {
			return v
		}
		return ""
	}
}

func getEvmChainId(network string) string {
	switch network {
	case "mainnet":
		if v := strings.TrimSpace(os.Getenv("LFS_EVM_MAINNET_CHAIN_ID")); v != "" {
			return v
		}
		return "0x1"
	case "testnet":
		if v := strings.TrimSpace(os.Getenv("LFS_EVM_TESTNET_CHAIN_ID")); v != "" {
			return v
		}
		return "0xaa36a7" // Sepolia
	default:
		if v := strings.TrimSpace(os.Getenv("LFS_EVM_LOCAL_CHAIN_ID")); v != "" {
			return v
		}
		return ""
	}
}

func (h *Handler) GetProtocolMetrics(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	// Try to get live data from protocol and SP services
	state, stateErr := h.protocolSvc.GetState(r.Context())
	sp, spErr := h.spSvc.GetIndex(r.Context())

	// If either call fails or we can't decode, return dummy data with X-Mocked header
	if stateErr != nil || spErr != nil {
		h.logger.Warnf("Failed to get live protocol metrics (state err: %v, sp err: %v), returning mocked data", stateErr, spErr)

		// Set mocked header
		w.Header().Set("X-Mocked", "true")

		// Return constant dummy data
		dto := ProtocolMetricsDTO{
			CurrentCR:    "1.45",
			TargetCR:     "1.50",
			PegDeviation: "0.0015",
			ReservesR:    "12500000",
			SupplyF:      "8500000",
			SupplyX:      "1200000",
			SPTVL:        "5600000",
			RewardAPR:    "8.5",
			IndexDelta:   "0.0012",
			AsOf:         time.Now().Unix(),
		}

		h.writeJSON(w, http.StatusOK, dto)
		return
	}

	// Calculate index delta (current - previous24h), or "0" if unavailable
	indexDelta := "0"
	if !sp.Current.IsZero() && !sp.Previous24h.IsZero() {
		indexDelta = sp.Current.Sub(sp.Previous24h).String()
	}

	// Build response from live data
	dto := ProtocolMetricsDTO{
		CurrentCR:    state.CR.String(),
		TargetCR:     state.CRTarget.String(),
		PegDeviation: state.PegDeviation.String(),
		ReservesR:    state.ReservesR.String(),
		SupplyF:      state.SupplyF.String(),
		SupplyX:      state.SupplyX.String(),
		SPTVL:        sp.TVLF.String(),
		RewardAPR:    sp.APR.String(),
		IndexDelta:   indexDelta,
		AsOf:         state.AsOf.Unix(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

// Quote endpoints
func (h *Handler) GetQuoteMintF(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	amountRStr := r.URL.Query().Get("amountR")
	if amountRStr == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "amountR is required")
		return
	}

	amountR, err := decimal.NewFromString(amountRStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid amountR format")
		return
	}

	if err := calc.ValidateAmount(amountR, "mint"); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", err.Error())
		return
	}

	quote, err := h.quoteSvc.GetMintQuote(r.Context(), amountR)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "QUOTE_ERROR", err.Error())
		return
	}

	dto := QuoteMintDTO{
		FOut:   quote.FOut.String(),
		Fee:    quote.Fee.String(),
		PostCR: quote.PostCR.String(),
		TTL:    quote.TTLSec,
		ID:     quote.QuoteID,
		AsOf:   quote.AsOf.Unix(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

func (h *Handler) GetQuoteRedeemF(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	amountFStr := r.URL.Query().Get("amountF")
	if amountFStr == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "amountF is required")
		return
	}

	amountF, err := decimal.NewFromString(amountFStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid amountF format")
		return
	}

	if err := calc.ValidateAmount(amountF, "redeem"); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", err.Error())
		return
	}

	quote, err := h.quoteSvc.GetRedeemQuote(r.Context(), amountF)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "QUOTE_ERROR", err.Error())
		return
	}

	dto := QuoteRedeemDTO{
		ROut:   quote.ROut.String(),
		Fee:    quote.Fee.String(),
		PostCR: quote.PostCR.String(),
		TTL:    quote.TTLSec,
		ID:     quote.QuoteID,
		AsOf:   quote.AsOf.Unix(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

func (h *Handler) GetQuoteMintX(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	amountRStr := r.URL.Query().Get("amountR")
	if amountRStr == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "amountR is required")
		return
	}

	amountR, err := decimal.NewFromString(amountRStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid amountR format")
		return
	}

	if err := calc.ValidateAmount(amountR, "mint"); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", err.Error())
		return
	}

	quote, err := h.quoteSvc.GetMintXQuote(r.Context(), amountR)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "QUOTE_ERROR", err.Error())
		return
	}

	dto := QuoteMintXDTO{
		XOut:   quote.XOut.String(),
		Fee:    quote.Fee.String(),
		PostCR: quote.PostCR.String(),
		TTL:    quote.TTLSec,
		ID:     quote.QuoteID,
		AsOf:   quote.AsOf.Unix(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

func (h *Handler) GetQuoteRedeemX(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	amountXStr := r.URL.Query().Get("amountX")
	if amountXStr == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "amountX is required")
		return
	}

	amountX, err := decimal.NewFromString(amountXStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid amountX format")
		return
	}

	if err := calc.ValidateAmount(amountX, "redeem"); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", err.Error())
		return
	}

	quote, err := h.quoteSvc.GetRedeemXQuote(r.Context(), amountX)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "QUOTE_ERROR", err.Error())
		return
	}

	dto := QuoteRedeemXDTO{
		ROut:   quote.ROut.String(),
		Fee:    quote.Fee.String(),
		PostCR: quote.PostCR.String(),
		TTL:    quote.TTLSec,
		ID:     quote.QuoteID,
		AsOf:   quote.AsOf.Unix(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

// Stability Pool endpoints
func (h *Handler) GetSPIndex(w http.ResponseWriter, r *http.Request) {
	index, err := h.spSvc.GetIndex(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "SP_INDEX_ERROR", err.Error())
		return
	}

	dto := SPIndexDTO{
		IndexNow:    index.Current.String(),
		Index24hAgo: index.Previous24h.String(),
		APR:         index.APR.String(),
		TVLF:        index.TVLF.String(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

func (h *Handler) GetSPUser(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "address is required")
		return
	}

	userSP, err := h.spSvc.GetUserPosition(r.Context(), address)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "SP_USER_ERROR", err.Error())
		return
	}

	dto := SPUserDTO{
		StakeF:            userSP.StakeF.String(),
		EnteredAt:         userSP.EnteredAt.Unix(),
		IndexAtJoin:       userSP.IndexAtJoin.String(),
		ClaimableR:        userSP.ClaimableR.String(),
		PendingIndexDelta: userSP.PendingIndexDelta.String(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

// User endpoints
func (h *Handler) GetUserPositions(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "address is required")
		return
	}

	positions, err := h.userSvc.GetPositions(r.Context(), address)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "USER_POSITIONS_ERROR", err.Error())
		return
	}

	dto := UserPositionsDTO{
		Address: positions.Address,
		Balances: map[string]string{
			"f": positions.BalanceF.String(),
			"x": positions.BalanceX.String(),
			"r": positions.BalanceR.String(),
		},
		UpdatedAt: positions.UpdatedAt.Unix(),
	}

	// Add SP stake if user has any
	if !positions.StakeF.IsZero() {
		spUser, _ := h.spSvc.GetUserPosition(r.Context(), address)
		if spUser != nil {
			dto.SPStake = &SPUserDTO{
				StakeF:            spUser.StakeF.String(),
				EnteredAt:         spUser.EnteredAt.Unix(),
				IndexAtJoin:       spUser.IndexAtJoin.String(),
				ClaimableR:        spUser.ClaimableR.String(),
				PendingIndexDelta: spUser.PendingIndexDelta.String(),
			}
		}
	}

	h.writeJSON(w, http.StatusOK, dto)
}

func (h *Handler) GetUserBalances(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "address is required")
		return
	}

	balances, err := h.userSvc.GetBalances(r.Context(), address)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "USER_BALANCES_ERROR", err.Error())
		return
	}

	addr, err := sui.AddressFromHex(address)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_ADDRESS", "invalid address format")
		return
	}

	dto := UserBalancesDTO{
		Address: addr,
		Balances: map[string]string{
			"f": balances.F.String(),
			"x": balances.X.String(),
			"r": balances.R.String(),
		},
		UpdatedAt: time.Now().Unix(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

func (h *Handler) GetUserTransactions(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "address is required")
		return
	}

	// Parse query parameters
	limit := 20 // default
	cursor := r.URL.Query().Get("cursor")
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	events, nextCursor, err := h.userSvc.GetTransactions(r.Context(), address, limit, cursor)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "USER_TRANSACTIONS_ERROR", err.Error())
		return
	}

	// Convert events to TransactionItems
	items := make([]TransactionItem, 0, len(events))
	for _, event := range events {
		// For now, create minimal transaction items from events
		// In a real implementation, this would parse event data properly
		item := TransactionItem{
			Hash:      event.TxDigest,
			Type:      event.Type,
			Amount:    "0",      // Would parse from event data
			Token:     "fToken", // Would parse from event data
			Timestamp: event.Timestamp.Unix(),
			Status:    "success", // Would determine from event data
		}
		items = append(items, item)
	}

	addr, err := sui.AddressFromHex(address)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_ADDRESS", "invalid address format")
		return
	}

	dto := UserTransactionsDTO{
		Address:    addr,
		Items:      items,
		NextCursor: nextCursor,
		UpdatedAt:  time.Now().Unix(),
	}

	h.writeJSON(w, http.StatusOK, dto)
}

// Health and ops endpoints
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	// TODO: Add readiness checks (DB connection, Redis, etc.)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("READY"))
}

// WebSocket endpoint
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	h.wsHub.HandleWebSocket(w, r)
}

// Chart data endpoints are now in candles.go

// SSE endpoint
func (h *Handler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	h.sseHandler.HandleSSE(w, r)
}

// Utility methods
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeJSONWithLog(w http.ResponseWriter, status int, data any, requestID string) {
	// Serialize the response data for logging
	responseBytes, err := json.Marshal(data)
	if err != nil {
		h.logger.Errorw("Failed to marshal response for logging", "request_id", requestID, "error", err)
	} else {
		responseStr := string(responseBytes)
		if len(responseStr) > 1000 {
			responseStr = responseStr[:1000] + "...[truncated]"
		}
		h.logger.Infow("Sending JSON response",
			"request_id", requestID,
			"status", status,
			"response", responseStr,
			"response_length", len(responseBytes),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.logger.Errorw("API error", "code", code, "message", message, "status", status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := ErrorResponse{
		Code:    code,
		Message: message,
	}
	json.NewEncoder(w).Encode(err)
}

func (h *Handler) writeErrorWithLog(w http.ResponseWriter, status int, code, message, requestID string) {
	h.logger.Errorw("API error",
		"request_id", requestID,
		"code", code,
		"message", message,
		"status", status,
	)

	err := ErrorResponse{
		Code:    code,
		Message: message,
	}

	// Log the error response being sent
	h.logger.Infow("Sending error response",
		"request_id", requestID,
		"status", status,
		"error_code", code,
		"error_message", message,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(err)
}

func generateQuoteID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Transaction building endpoint
func (h *Handler) BuildUnsignedTransaction(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Get request ID for correlation
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = generateQuoteID()
	}

	// Log the incoming request
	h.logger.Infow("Transaction build request received",
		"request_id", requestID,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent(),
		"content_length", r.ContentLength,
	)

	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	// Read the body first to log it, then create a new reader for decoding
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Errorw("Failed to read request body",
			"request_id", requestID,
			"error", err,
			"remote_addr", r.RemoteAddr,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to read request body", requestID)
		return
	}

	// Log the raw request body (truncated if too long)
	bodyStr := string(bodyBytes)
	if len(bodyStr) > 500 {
		bodyStr = bodyStr[:500] + "...[truncated]"
	}
	h.logger.Infow("Request body received",
		"request_id", requestID,
		"body", bodyStr,
		"body_length", len(bodyBytes),
	)

	// Create a new reader from the body bytes for JSON decoding
	bodyReader := strings.NewReader(string(bodyBytes))

	var req UnsignedTransactionRequest
	if err := json.NewDecoder(bodyReader).Decode(&req); err != nil {
		h.logger.Errorw("Failed to decode transaction build request",
			"request_id", requestID,
			"error", err,
			"raw_body", bodyStr,
			"remote_addr", r.RemoteAddr,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body", requestID)
		return
	}

	// Log request details
	h.logger.Infow("Processing transaction build request",
		"request_id", requestID,
		"action", req.Action,
		"token_type", req.TokenType,
		"amount", req.Amount,
	)

	// Validate action
	if req.Action != "mint" && req.Action != "redeem" {
		h.logger.Errorw("Invalid action in transaction build request",
			"request_id", requestID,
			"action", req.Action,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_ACTION", "action must be 'mint' or 'redeem'", requestID)
		return
	}

	// Validate token type
	if req.TokenType != "xtoken" && req.TokenType != "ftoken" {
		h.logger.Errorw("Invalid token type in transaction build request",
			"request_id", requestID,
			"token_type", req.TokenType,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_TOKEN_TYPE", "tokenType must be 'xtoken' or 'ftoken'", requestID)
		return
	}

	// Parse amount
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		h.logger.Errorw("Invalid amount format in transaction build request",
			"request_id", requestID,
			"amount", req.Amount,
			"error", err,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_AMOUNT", "Invalid amount format", requestID)
		return
	}

	if amount.IsZero() || amount.IsNegative() {
		h.logger.Errorw("Invalid amount value in transaction build request",
			"request_id", requestID,
			"amount", req.Amount,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_AMOUNT", "Amount must be positive", requestID)
		return
	}

	// Get user address from request headers or query params
	userAddressStr := r.Header.Get("X-User-Address")
	if userAddressStr == "" {
		userAddressStr = r.URL.Query().Get("userAddress")
	}
	if userAddressStr == "" {
		h.logger.Errorw("Missing user address in transaction build request",
			"request_id", requestID,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "MISSING_USER_ADDRESS", "User address is required in X-User-Address header or userAddress query parameter", requestID)
		return
	}

	userAddress, err := sui.AddressFromHex(userAddressStr)
	if err != nil {
		h.logger.Errorw("Invalid user address format in transaction build request",
			"request_id", requestID,
			"user_address", userAddressStr,
			"error", err,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_USER_ADDRESS", "Invalid user address format", requestID)
		return
	}

	// Determine mode from query parameter
	mode := onchain.TxBuildModeExecution
	if r.URL.Query().Get("mode") == "devinspect" {
		mode = onchain.TxBuildModeDevInspect
	}

	// Build the transaction based on action
	var unsignedTx *onchain.UnsignedTransaction
	var selectedMarket markets.Market
	var hasMarket bool

	if req.MarketID != "" {
		if h.marketsSvc == nil {
			h.writeErrorWithLog(w, http.StatusInternalServerError, "MARKETS_ERROR", "Markets service unavailable", requestID)
			return
		}
		market, ok := h.marketsSvc.Get(req.MarketID)
		if !ok {
			h.writeErrorWithLog(w, http.StatusNotFound, "MARKET_NOT_FOUND", "marketId not recognized", requestID)
			return
		}
		selectedMarket = market
		hasMarket = true
	}

	switch req.Action {
	case "mint":
		unsignedTx, err = h.txBuilder.BuildMintTransaction(r.Context(), onchain.MintTxRequest{
			OutTokenType: req.TokenType,
			Amount:       amount,
			UserAddress:  userAddress,
			Mode:         mode,
		})
	case "redeem":
		unsignedTx, err = h.txBuilder.BuildRedeemTransaction(r.Context(), onchain.RedeemTxRequest{
			InTokenType: req.TokenType,
			Amount:      amount,
			UserAddress: userAddress,
			Mode:        mode,
		})
	}

	if err != nil {
		h.logger.Errorw("Failed to build transaction",
			"request_id", requestID,
			"error", err,
			"action", req.Action,
			"token_type", req.TokenType,
			"amount", req.Amount,
			"user_address", userAddressStr,
		)
		h.writeErrorWithLog(w, http.StatusInternalServerError, "TRANSACTION_BUILD_ERROR", "Failed to build unsigned transaction", requestID)
		return
	}

	// Attach market metadata and Walrus checkpoint hints for cross-chain markets.
	if unsignedTx.Metadata == nil {
		unsignedTx.Metadata = map[string]string{}
	}

	if hasMarket {
		unsignedTx.Metadata["marketId"] = selectedMarket.ID
		unsignedTx.Metadata["marketMode"] = selectedMarket.Mode
		if selectedMarket.Mode == "crosschain" && h.crosschainSvc != nil {
			cp, err := h.crosschainSvc.GetLatestCheckpoint(r.Context(), crosschain.ChainID(selectedMarket.ChainID), selectedMarket.Asset)
			if err == nil && cp != nil {
				unsignedTx.Metadata["walrusProofCid"] = cp.WalrusBlobID
				unsignedTx.Metadata["walrusUpdateId"] = fmt.Sprintf("%d", cp.UpdateID)
				unsignedTx.Metadata["walrusChainId"] = selectedMarket.ChainID
				unsignedTx.Metadata["walrusAsset"] = selectedMarket.Asset
			}
		}
	}

	// Generate quote ID for tracking
	quoteID := generateQuoteID()

	h.logger.Infow("Transaction build successful",
		"request_id", requestID,
		"quote_id", quoteID,
		"action", req.Action,
		"token_type", req.TokenType,
		"amount", req.Amount,
		"gas_estimate", unsignedTx.GasEstimate,
		"tx_bytes_length", len(unsignedTx.TransactionBlockBytes),
		"duration", time.Since(start),
	)

	// Create response
	response := UnsignedTransactionResponse{
		TransactionBlockBytes: unsignedTx.TransactionBlockBytes,
		GasEstimate:           fmt.Sprintf("%d", unsignedTx.GasEstimate),
		QuoteID:               quoteID,
		Metadata:              unsignedTx.Metadata,
	}

	h.writeJSONWithLog(w, http.StatusOK, response, requestID)
}

// SubmitSignedTransaction handles submission of signed transactions
func (h *Handler) SubmitSignedTransaction(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Get request ID for correlation
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = generateQuoteID()
	}

	// Log the incoming request
	h.logger.Infow("Transaction submission request received",
		"request_id", requestID,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent(),
		"content_length", r.ContentLength,
	)

	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	// Read the body first to log it, then create a new reader for decoding
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Errorw("Failed to read request body for transaction submission",
			"request_id", requestID,
			"error", err,
			"remote_addr", r.RemoteAddr,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to read request body", requestID)
		return
	}

	// Log the raw request body (truncated if too long for security)
	bodyStr := string(bodyBytes)
	if len(bodyStr) > 1000 {
		bodyStr = bodyStr[:1000] + "...[truncated]"
	}
	h.logger.Infow("Transaction submission body received",
		"request_id", requestID,
		"body", bodyStr,
		"body_length", len(bodyBytes),
	)

	// Create a new reader from the body bytes for JSON decoding
	bodyReader := strings.NewReader(string(bodyBytes))

	var req SignedTransactionRequest
	if err := json.NewDecoder(bodyReader).Decode(&req); err != nil {
		h.logger.Errorw("Failed to decode transaction submission request",
			"request_id", requestID,
			"error", err,
			"raw_body", bodyStr,
			"remote_addr", r.RemoteAddr,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body", requestID)
		return
	}

	// Log transaction details (first few chars for security)
	txBytesPreview := req.TxBytes
	if len(txBytesPreview) > 100 {
		txBytesPreview = txBytesPreview[:100] + "..."
	}
	signaturePreview := req.Signature
	if len(signaturePreview) > 50 {
		signaturePreview = signaturePreview[:50] + "..."
	}

	h.logger.Infow("Processing transaction submission",
		"request_id", requestID,
		"quote_id", req.QuoteID,
		"tx_bytes_preview", txBytesPreview,
		"tx_bytes_length", len(req.TxBytes),
		"signature_preview", signaturePreview,
		"signature_length", len(req.Signature),
	)

	// Validate required fields
	if req.TxBytes == "" {
		h.logger.Errorw("Transaction submission missing required field",
			"request_id", requestID,
			"missing_field", "tx_bytes",
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "MISSING_PARAMETER", "tx_bytes is required", requestID)
		return
	}
	if req.Signature == "" {
		h.logger.Errorw("Transaction submission missing required field",
			"request_id", requestID,
			"missing_field", "signature",
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "MISSING_PARAMETER", "signature is required", requestID)
		return
	}

	// Submit the signed transaction
	result, err := h.txSubmitter.SubmitSignedTransaction(r.Context(), req.TxBytes, req.Signature)
	if err != nil {
		h.logger.Errorw("Transaction submission failed",
			"request_id", requestID,
			"quote_id", req.QuoteID,
			"error", err.Error(),
			"tx_bytes_length", len(req.TxBytes),
			"signature_length", len(req.Signature),
			"remote_addr", r.RemoteAddr,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "SUBMISSION_ERROR", err.Error(), requestID)
		return
	}

	h.logger.Infow("Transaction submission successful",
		"request_id", requestID,
		"quote_id", req.QuoteID,
		"transaction_digest", result.TransactionDigest,
		"status", result.Status,
		"duration", time.Since(start),
	)

	// Create response
	response := SignedTransactionResponse{
		TransactionDigest: result.TransactionDigest,
		Status:            result.Status,
	}

	h.writeJSONWithLog(w, http.StatusOK, response, requestID)
}

// TransactionMonitor endpoint for frontend to report transaction attempts
func (h *Handler) ReportTransactionAttempt(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = generateQuoteID()
	}

	h.logger.Infow("Transaction monitoring report received",
		"request_id", requestID,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent(),
	)

	// Read the body to log transaction attempt details
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Errorw("Failed to read transaction monitoring report body",
			"request_id", requestID,
			"error", err,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to read request body", requestID)
		return
	}

	// Log the transaction monitoring data
	bodyStr := string(bodyBytes)
	if len(bodyStr) > 2000 {
		bodyStr = bodyStr[:2000] + "...[truncated]"
	}

	h.logger.Infow("Transaction monitoring data",
		"request_id", requestID,
		"report", bodyStr,
		"report_length", len(bodyBytes),
	)

	// Parse the monitoring report
	type TransactionMonitoringReport struct {
		EventType         string `json:"eventType"`       // "attempt", "success", "error"
		TransactionType   string `json:"transactionType"` // "mint", "redeem", etc.
		UserAddress       string `json:"userAddress"`
		TransactionDigest string `json:"transactionDigest,omitempty"`
		ErrorMessage      string `json:"errorMessage,omitempty"`
		ErrorCode         string `json:"errorCode,omitempty"`
		Amount            string `json:"amount,omitempty"`
		TokenType         string `json:"tokenType,omitempty"`
		Timestamp         int64  `json:"timestamp"`
	}

	bodyReader := strings.NewReader(string(bodyBytes))
	var report TransactionMonitoringReport
	if err := json.NewDecoder(bodyReader).Decode(&report); err != nil {
		h.logger.Errorw("Failed to decode transaction monitoring report",
			"request_id", requestID,
			"error", err,
			"raw_body", bodyStr,
		)
		h.writeErrorWithLog(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body", requestID)
		return
	}

	// Log the parsed transaction monitoring report with structured data
	logLevel := "Infow"
	logMsg := "Transaction monitoring report processed"

	if report.EventType == "error" {
		logLevel = "Errorw"
		logMsg = "Transaction error reported by frontend"
	}

	logData := map[string]interface{}{
		"request_id":       requestID,
		"event_type":       report.EventType,
		"transaction_type": report.TransactionType,
		"user_address":     report.UserAddress,
		"amount":           report.Amount,
		"token_type":       report.TokenType,
		"timestamp":        report.Timestamp,
	}

	if report.TransactionDigest != "" {
		logData["transaction_digest"] = report.TransactionDigest
	}

	if report.ErrorMessage != "" {
		logData["error_message"] = report.ErrorMessage
		logData["error_code"] = report.ErrorCode
	}

	// Log the structured monitoring data
	if logLevel == "Errorw" {
		h.logger.Errorw(logMsg,
			"request_id", requestID,
			"event_type", report.EventType,
			"transaction_type", report.TransactionType,
			"user_address", report.UserAddress,
			"error_message", report.ErrorMessage,
			"error_code", report.ErrorCode,
			"amount", report.Amount,
			"token_type", report.TokenType,
			"timestamp", report.Timestamp,
		)
	} else {
		h.logger.Infow(logMsg,
			"request_id", requestID,
			"event_type", report.EventType,
			"transaction_type", report.TransactionType,
			"user_address", report.UserAddress,
			"transaction_digest", report.TransactionDigest,
			"amount", report.Amount,
			"token_type", report.TokenType,
			"timestamp", report.Timestamp,
		)
	}

	// Return success response
	response := map[string]string{
		"status":     "logged",
		"request_id": requestID,
	}

	h.writeJSONWithLog(w, http.StatusOK, response, requestID)
}

func toSuiBalanceScale(d decimal.Decimal) decimal.Decimal {
	return d.Div(decimal.NewFromBigInt(big.NewInt(1), unit.SuiDecimal))
}

// BuildUpdateOracleTransaction builds unsigned transaction for oracle updates
func (h *Handler) BuildUpdateOracleTransaction(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	var req UpdateOracleBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	// Convert mode string to TxBuildMode type
	var mode onchain.TxBuildMode
	switch req.Mode {
	case "execution":
		mode = onchain.TxBuildModeExecution
	case "devinspect":
		mode = onchain.TxBuildModeDevInspect
	default:
		h.writeError(w, http.StatusBadRequest, "INVALID_MODE", "mode must be 'execution' or 'devinspect'")
		return
	}

	txReq := onchain.UpdateOracleTxRequest{
		NewPrice: req.Price,
		Mode:     mode,
	}
	unsignedTx, err := h.txBuilder.BuildUpdateOracleTransaction(r.Context(), txReq)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "TRANSACTION_BUILD_ERROR", err.Error())
		return
	}

	response := UpdateOracleBuildResponse{
		TransactionBlockBytes: unsignedTx.TransactionBlockBytes,
		GasEstimate:           fmt.Sprintf("%d", unsignedTx.GasEstimate),
		Metadata:              unsignedTx.Metadata,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// SubmitUpdateOracleTransaction submits signed oracle update transaction
func (h *Handler) SubmitUpdateOracleTransaction(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	var req UpdateOracleSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.TxBytes == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "tx_bytes is required")
		return
	}
	if req.Signature == "" {
		h.writeError(w, http.StatusBadRequest, "MISSING_PARAMETER", "signature is required")
		return
	}

	result, err := h.txSubmitter.SubmitSignedTransaction(r.Context(), req.TxBytes, req.Signature)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "SUBMISSION_ERROR", err.Error())
		return
	}

	response := UpdateOracleSubmitResponse{
		TransactionDigest: result.TransactionDigest,
		Status:            result.Status,
	}

	h.writeJSON(w, http.StatusOK, response)
}
