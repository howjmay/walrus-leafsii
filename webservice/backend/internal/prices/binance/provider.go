package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/leafsii/leafsii-backend/internal/prices"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

const (
	BinanceRestAPI = "https://api.binance.com"
	BinanceWS      = "wss://stream.binance.com:9443/ws"
	BinanceScale   = 1_000_000
)

// Provider implements the prices.Provider interface for Binance
type Provider struct {
	logger *zap.SugaredLogger
	client *http.Client

	mu     sync.RWMutex
	health prices.ProviderHealth
}

// NewProvider creates a new Binance provider
func NewProvider(logger *zap.SugaredLogger) *Provider {
	return &Provider{
		logger: logger,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		health: prices.ProviderHealth{
			Healthy:     true,
			LastSuccess: time.Now(),
		},
	}
}

// Name returns the provider identifier
func (p *Provider) Name() string {
	return "binance"
}

// Health returns current provider health status
func (p *Provider) Health() prices.ProviderHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.health
}

// updateHealth updates the provider health status
func (p *Provider) updateHealth(healthy bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.health.Healthy = healthy
	if healthy {
		p.health.LastSuccess = time.Now()
		p.health.LastError = ""
	} else if err != nil {
		p.health.LastError = err.Error()
	}
}

// FetchHistory retrieves historical kline data from Binance
func (p *Provider) FetchHistory(ctx context.Context, symbol string, interval time.Duration, limit int) ([]prices.Candle, error) {
	// Build request URL
	baseURL := fmt.Sprintf("%s/api/v3/klines", BinanceRestAPI)
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("interval", binanceInterval(interval))
	params.Set("limit", strconv.Itoa(limit))

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		p.updateHealth(false, err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		p.updateHealth(false, err)
		return nil, fmt.Errorf("failed to fetch from Binance: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("Binance API error: %d", resp.StatusCode)
		p.updateHealth(false, err)
		return nil, err
	}

	// Parse response
	var klines [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&klines); err != nil {
		p.updateHealth(false, err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to our candle format
	candles := make([]prices.Candle, 0, len(klines))
	for _, kline := range klines {
		candle, err := parseKline(kline)
		if err != nil {
			p.logger.Warnw("Failed to parse kline", "error", err, "kline", kline)
			continue
		}

		// Align timestamp to interval boundary
		alignedTime := prices.AlignTime(time.UnixMilli(int64(candle.Time*1000)), interval)
		candle.Time = alignedTime.Unix()

		candles = append(candles, candle)
	}

	p.updateHealth(true, nil)
	p.logger.Debugw("Fetched history from Binance", "symbol", symbol, "interval", interval, "candles", len(candles))

	return candles, nil
}

// SubscribeLive subscribes to real-time trade data via WebSocket
func (p *Provider) SubscribeLive(ctx context.Context, symbol string, out chan<- prices.Tick) error {
	// Convert symbol to lowercase for WebSocket
	wsSymbol := symbol + "@trade"
	wsURL := fmt.Sprintf("%s/%s", BinanceWS, wsSymbol)

	p.logger.Infow("Connecting to Binance WebSocket", "url", wsURL)

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		p.updateHealth(false, err)
		return fmt.Errorf("failed to connect to Binance WebSocket: %w", err)
	}
	defer conn.Close()

	p.updateHealth(true, nil)
	p.logger.Infow("Connected to Binance WebSocket", "symbol", symbol)

	// Read messages
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		_, message, err := conn.ReadMessage()
		if err != nil {
			p.updateHealth(false, err)
			p.mu.Lock()
			p.health.Reconnects++
			p.mu.Unlock()
			return fmt.Errorf("WebSocket read error: %w", err)
		}

		// Parse trade message
		var trade BinanceTrade
		if err := json.Unmarshal(message, &trade); err != nil {
			p.logger.Warnw("Failed to parse trade message", "error", err, "message", string(message))
			continue
		}

		// Convert to tick
		price, err := strconv.ParseFloat(trade.Price, 64)
		if err != nil {
			p.logger.Warnw("Failed to parse trade price", "error", err, "price", trade.Price)
			continue
		}

		tick := prices.Tick{
			Symbol: symbol,
			Price:  price,
			TsMs:   trade.EventTime,
		}

		// Send tick (non-blocking)
		select {
		case out <- tick:
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Channel full, skip this tick
			p.logger.Debugw("Tick channel full, skipping", "symbol", symbol)
		}

		p.updateHealth(true, nil)
	}
}

// BinanceTrade represents a trade message from Binance WebSocket
type BinanceTrade struct {
	EventType     string `json:"e"`
	EventTime     int64  `json:"E"`
	Symbol        string `json:"s"`
	TradeID       int64  `json:"t"`
	Price         string `json:"p"`
	Quantity      string `json:"q"`
	BuyerOrderID  int64  `json:"b"`
	SellerOrderID int64  `json:"a"`
	TradeTime     int64  `json:"T"`
	IsBuyerMaker  bool   `json:"m"`
}

// parseKline converts Binance kline array to our Candle struct
func parseKline(kline []interface{}) (prices.Candle, error) {
	if len(kline) < 11 {
		return prices.Candle{}, fmt.Errorf("invalid kline format: expected 11 fields, got %d", len(kline))
	}

	// Parse timestamp
	openTimeFloat, ok := kline[0].(float64)
	if !ok {
		return prices.Candle{}, fmt.Errorf("invalid open time format")
	}
	openTime := int64(openTimeFloat) / 1000 // Convert ms to seconds

	// Parse OHLCV
	open, err := parseFloat(kline[1])
	if err != nil {
		return prices.Candle{}, fmt.Errorf("invalid open price: %w", err)
	}

	high, err := parseFloat(kline[2])
	if err != nil {
		return prices.Candle{}, fmt.Errorf("invalid high price: %w", err)
	}

	low, err := parseFloat(kline[3])
	if err != nil {
		return prices.Candle{}, fmt.Errorf("invalid low price: %w", err)
	}

	close, err := parseFloat(kline[4])
	if err != nil {
		return prices.Candle{}, fmt.Errorf("invalid close price: %w", err)
	}

	volume, err := parseFloat(kline[5])
	if err != nil {
		return prices.Candle{}, fmt.Errorf("invalid volume: %w", err)
	}

	return prices.Candle{
		Time:   openTime,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  close,
		Volume: volume,
	}, nil
}

// parseFloat safely converts interface{} to float64
func parseFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case string:
		return strconv.ParseFloat(val, 64)
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// GetLatestPrice fetches the latest price for a symbol and returns it as decimal.Decimal
func (p *Provider) GetLatestPrice(ctx context.Context, symbol string) (decimal.Decimal, error) {
	// Fetch the latest 1-minute candle
	candles, err := p.FetchHistory(ctx, symbol, time.Minute, 1)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to fetch latest price from Binance: %w", err)
	}

	if len(candles) == 0 {
		return decimal.Zero, fmt.Errorf("no price data returned from Binance for symbol %s", symbol)
	}

	p.logger.Debugw("Binance returns: ", candles[0].Close)
	// Return the close price of the latest candle as decimal
	closePrice := decimal.NewFromInt(int64(candles[0].Close * float64(BinanceScale)))

	p.logger.Debugw("Fetched latest price from Binance", "symbol", symbol, "price", closePrice)

	return closePrice, nil
}

// binanceInterval converts time.Duration to Binance interval string
func binanceInterval(d time.Duration) string {
	switch d {
	case time.Minute:
		return "1m"
	case 5 * time.Minute:
		return "5m"
	case 15 * time.Minute:
		return "15m"
	case time.Hour:
		return "1h"
	case 4 * time.Hour:
		return "4h"
	case 24 * time.Hour:
		return "1d"
	default:
		return "1h" // default fallback
	}
}
