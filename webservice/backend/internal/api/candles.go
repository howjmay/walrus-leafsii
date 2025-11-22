package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/leafsii/leafsii-backend/internal/prices"
	"github.com/leafsii/leafsii-backend/internal/prices/binance"
	"github.com/leafsii/leafsii-backend/internal/prices/mock"
)

// CandleResponse represents the API response for candle data
type CandleResponse struct {
	Data   []prices.Candle `json:"data"`
	Mocked bool            `json:"mocked,omitempty"`
}

// GetCandles handles GET /api/v1/candles
func (h *Handler) GetCandles(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusOK, time.Since(start))
	}()

	// Parse query parameters
	pair := r.URL.Query().Get("pair")
	interval := r.URL.Query().Get("interval")
	limitStr := r.URL.Query().Get("limit")

	// Default values
	if pair == "" {
		pair = "SUI/USD"
	}
	if interval == "" {
		interval = "15m"
	}

	limit := 500
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 2000 {
			limit = parsedLimit
		}
	}

	// Validate interval
	intervalDuration := prices.ParseInterval(interval)
	if intervalDuration == 0 {
		h.writeError(w, http.StatusBadRequest, "INVALID_INTERVAL", "invalid interval format")
		return
	}

	// Get provider symbol from UI pair
	registry := prices.NewRegistry()
	providerSymbol, err := registry.GetProviderSymbol(pair)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_PAIR", fmt.Sprintf("unsupported pair: %s", pair))
		return
	}

	// Try to get candles from provider
	candles, mocked, err := h.fetchCandlesWithFallback(r.Context(), providerSymbol, intervalDuration, limit)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "CANDLES_ERROR", err.Error())
		return
	}

	response := CandleResponse{
		Data:   candles,
		Mocked: mocked,
	}

	// Add mocked header if using mock data
	if mocked {
		w.Header().Set("X-Mocked", "true")
	}

	h.writeJSON(w, http.StatusOK, response)
}

// fetchCandlesWithFallback attempts to fetch candles from primary provider with mock fallback
func (h *Handler) fetchCandlesWithFallback(ctx context.Context, symbol string, interval time.Duration, limit int) ([]prices.Candle, bool, error) {
	// Create primary provider (Binance)
	primaryProvider := binance.NewProvider(h.logger)

	// Try primary provider first
	candles, err := primaryProvider.FetchHistory(ctx, symbol, interval, limit)
	if err == nil && len(candles) > 0 {
		h.logger.Debugw("Fetched candles from primary provider",
			"symbol", symbol,
			"interval", interval,
			"count", len(candles),
			"provider", primaryProvider.Name(),
		)
		return candles, false, nil
	}

	// Log primary provider failure
	h.logger.Warnw("Primary provider failed, falling back to mock",
		"symbol", symbol,
		"interval", interval,
		"provider", primaryProvider.Name(),
		"error", err,
	)

	// Fall back to mock provider
	mockProvider := h.createMockProvider(symbol)
	candles, err = mockProvider.FetchHistory(ctx, symbol, interval, limit)
	if err != nil {
		return nil, true, fmt.Errorf("both primary and mock providers failed: %w", err)
	}

	h.logger.Infow("Using mock data for candles",
		"symbol", symbol,
		"interval", interval,
		"count", len(candles),
	)

	return candles, true, nil
}

// createMockProvider creates a mock provider with realistic base price
func (h *Handler) createMockProvider(symbol string) prices.Provider {
	basePrice := 1.00 // Default

	// Try to get last known real price from cache
	cacheKey := fmt.Sprintf("fx:oracle:price:%s", symbol)
	var lastTick prices.Tick
	if err := h.cache.Get(context.Background(), cacheKey, &lastTick); err == nil {
		basePrice = lastTick.Price
	} else {
		// Use symbol-specific defaults
		switch strings.ToUpper(symbol) {
		case "SUIUSDT":
			basePrice = 1.50 // Reasonable SUI price
		case "BTCUSDT":
			basePrice = 45000.0
		case "ETHUSDT":
			basePrice = 2500.0
		default:
			basePrice = 1.00
		}
	}

	return mock.NewGenerator(h.logger, basePrice, 0.002) // 0.2% volatility
}
