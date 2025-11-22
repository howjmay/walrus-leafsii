package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/leafsii/leafsii-backend/internal/prices"
	"github.com/leafsii/leafsii-backend/internal/prices/binance"
	"github.com/leafsii/leafsii-backend/internal/prices/mock"
	"github.com/leafsii/leafsii-backend/internal/store"
	"go.uber.org/zap"
)

type PricePublisher struct {
	provider     prices.Provider
	mockProvider prices.Provider
	registry     *prices.Registry
	cache        *store.Cache
	logger       *zap.SugaredLogger
	config       PricePublisherConfig

	mu             sync.RWMutex
	currentCandles map[string]*CandleAggregator // symbol -> aggregator
	usingMock      bool
	cancelCtx      context.CancelFunc
}

type PricePublisherConfig struct {
	ProviderType   string        // "binance" or "mock"
	RetryInterval  time.Duration // How long to wait before retrying failed provider
	MaxTicksPerSym int           // Maximum ticks to keep per symbol in cache
	TTL            time.Duration // Cache TTL for latest prices
	MockVolatility float64       // Volatility for mock data
	MockBasePrice  float64       // Base price for mock data
}

// CandleAggregator aggregates ticks into candles
type CandleAggregator struct {
	interval      time.Duration
	currentCandle *prices.Candle
	lastUpdate    time.Time
}

func NewPricePublisher(cache *store.Cache, logger *zap.SugaredLogger, config PricePublisherConfig) *PricePublisher {
	// Create primary provider
	var provider prices.Provider
	switch config.ProviderType {
	case "binance":
		provider = binance.NewProvider(logger)
	case "mock":
		provider = mock.NewGenerator(logger, config.MockBasePrice, config.MockVolatility)
	default:
		provider = binance.NewProvider(logger) // Default to Binance
	}

	// Always create mock provider as fallback
	mockProvider := mock.NewGenerator(logger, config.MockBasePrice, config.MockVolatility)

	return &PricePublisher{
		provider:       provider,
		mockProvider:   mockProvider,
		registry:       prices.NewRegistry(),
		cache:          cache,
		logger:         logger,
		config:         config,
		currentCandles: make(map[string]*CandleAggregator),
		usingMock:      false,
	}
}

func (p *PricePublisher) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	p.cancelCtx = cancel

	symbols := p.registry.GetProviderSymbols()
	if len(symbols) == 0 {
		symbols = []string{"SUIUSDT"}
	}

	p.logger.Infow("Starting price publisher",
		"provider", p.provider.Name(),
		"symbols", symbols,
		"mappings", p.registry.GetAllMappings(),
	)

	for _, symbol := range symbols {
		go p.subscribeLiveData(ctx, symbol)
	}

	// Health check and retry loop
	retryTicker := time.NewTicker(p.config.RetryInterval)
	defer retryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Infow("Price publisher stopping due to context cancellation")
			return ctx.Err()
		case <-retryTicker.C:
			p.checkProviderHealth(ctx, symbols)
		}
	}
}

func (p *PricePublisher) Stop() {
	if p.cancelCtx != nil {
		p.cancelCtx()
	}
}

// subscribeLiveData subscribes to live price data for a symbol
func (p *PricePublisher) subscribeLiveData(ctx context.Context, symbol string) {
	tickChan := make(chan prices.Tick, 100) // Buffer for ticks

	p.logger.Infow("Starting live subscription", "symbol", symbol, "provider", p.getCurrentProvider().Name())

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		currentProvider := p.getCurrentProvider()

		// Subscribe to live data
		go func() {
			if err := currentProvider.SubscribeLive(ctx, symbol, tickChan); err != nil {
				p.logger.Warnw("Live subscription failed", "symbol", symbol, "provider", currentProvider.Name(), "error", err)

				// Switch to mock provider if primary fails
				if !p.usingMock && currentProvider.Name() != "mock" {
					p.switchToMock(symbol, "live subscription failed")
				}
			}
		}()

		// Process ticks
		for tick := range tickChan {
			p.processTick(ctx, tick)
		}

		// If we get here, the channel was closed, retry after delay
		p.logger.Warnw("Tick channel closed, retrying", "symbol", symbol)
		time.Sleep(p.config.RetryInterval)
	}
}

// processTick handles incoming price ticks
func (p *PricePublisher) processTick(ctx context.Context, tick prices.Tick) {
	// Cache latest price
	cacheKey := fmt.Sprintf("fx:oracle:price:%s", tick.Symbol)
	if err := p.cache.Set(ctx, cacheKey, tick, p.config.TTL); err != nil {
		p.logger.Warnw("Failed to cache tick", "symbol", tick.Symbol, "error", err)
	}

	// Add to tick history
	if err := p.addToTickHistory(ctx, tick.Symbol, tick); err != nil {
		p.logger.Warnw("Failed to add tick to history", "symbol", tick.Symbol, "error", err)
	}

	// Update candle aggregators
	p.updateCandleAggregators(ctx, tick)

	// Publish to pub/sub channel
	channel := fmt.Sprintf("fx:oracle:price:%s", tick.Symbol)
	if err := p.cache.Publish(ctx, channel, tick); err != nil {
		p.logger.Warnw("Failed to publish tick", "symbol", tick.Symbol, "channel", channel, "error", err)
	} else {
		p.logger.Debugw("Published tick", "symbol", tick.Symbol, "price", tick.Price)
	}
}

// updateCandleAggregators updates candle aggregators for all intervals
func (p *PricePublisher) updateCandleAggregators(ctx context.Context, tick prices.Tick) {
	intervals := []time.Duration{
		time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		time.Hour,
		4 * time.Hour,
		24 * time.Hour,
	}

	for _, interval := range intervals {
		aggregatorKey := fmt.Sprintf("%s:%s", tick.Symbol, prices.IntervalString(interval))

		p.mu.Lock()
		aggregator, exists := p.currentCandles[aggregatorKey]
		if !exists {
			aggregator = &CandleAggregator{
				interval: interval,
			}
			p.currentCandles[aggregatorKey] = aggregator
		}
		p.mu.Unlock()

		// Update aggregator
		candle := aggregator.AddTick(tick, interval)
		if candle != nil {
			// Cache the latest candle
			candleKey := fmt.Sprintf("fx:candles:%s:%s:latest", tick.Symbol, prices.IntervalString(interval))
			if err := p.cache.Set(ctx, candleKey, candle, p.config.TTL); err != nil {
				p.logger.Warnw("Failed to cache candle", "symbol", tick.Symbol, "interval", interval, "error", err)
			}
		}
	}
}

// AddTick adds a tick to the candle aggregator
func (a *CandleAggregator) AddTick(tick prices.Tick, interval time.Duration) *prices.Candle {
	tickTime := time.UnixMilli(tick.TsMs)
	alignedTime := prices.AlignTime(tickTime, interval)

	// Check if we need a new candle
	if a.currentCandle == nil || a.currentCandle.Time != alignedTime.Unix() {
		// Finalize previous candle if it exists
		if a.currentCandle != nil {
			// Previous candle is complete, could be stored/published here
		}

		// Start new candle
		a.currentCandle = &prices.Candle{
			Time:   alignedTime.Unix(),
			Open:   tick.Price,
			High:   tick.Price,
			Low:    tick.Price,
			Close:  tick.Price,
			Volume: 0, // Volume not available from ticks
		}
	} else {
		// Update existing candle
		if tick.Price > a.currentCandle.High {
			a.currentCandle.High = tick.Price
		}
		if tick.Price < a.currentCandle.Low {
			a.currentCandle.Low = tick.Price
		}
		a.currentCandle.Close = tick.Price
	}

	a.lastUpdate = tickTime
	return a.currentCandle
}

// getCurrentProvider returns the currently active provider
func (p *PricePublisher) getCurrentProvider() prices.Provider {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.usingMock {
		return p.mockProvider
	}
	return p.provider
}

// switchToMock switches to mock provider with logging
func (p *PricePublisher) switchToMock(symbol, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.usingMock {
		p.usingMock = true
		p.logger.Warnw("Switching to mock provider",
			"symbol", symbol,
			"reason", reason,
			"provider", p.provider.Name(),
		)

		// Try to get last real price for mock continuity
		cacheKey := fmt.Sprintf("fx:oracle:price:%s", symbol)
		var lastTick prices.Tick
		if err := p.cache.Get(context.Background(), cacheKey, &lastTick); err == nil {
			if mockGen, ok := p.mockProvider.(*mock.Generator); ok {
				mockGen.SetBasePrice(lastTick.Price)
			}
		}
	}
}

// checkProviderHealth checks and potentially switches providers
func (p *PricePublisher) checkProviderHealth(ctx context.Context, symbols []string) {
	providerHealth := p.provider.Health()

	if !providerHealth.Healthy && !p.usingMock {
		p.logger.Warnw("Primary provider unhealthy, switching to mock",
			"provider", p.provider.Name(),
			"lastError", providerHealth.LastError,
			"reconnects", providerHealth.Reconnects,
		)

		for _, symbol := range symbols {
			p.switchToMock(symbol, "provider health check failed")
		}
	} else if providerHealth.Healthy && p.usingMock {
		p.logger.Infow("Primary provider recovered, switching back",
			"provider", p.provider.Name(),
		)

		p.mu.Lock()
		p.usingMock = false
		p.mu.Unlock()

		// Restart live subscriptions for all symbols
		for _, symbol := range symbols {
			go p.subscribeLiveData(ctx, symbol)
		}
	}
}

func (p *PricePublisher) addToTickHistory(ctx context.Context, symbol string, tick prices.Tick) error {
	historyKey := fmt.Sprintf("fx:ticks:%s", symbol)

	// Get existing ticks
	var existingTicks []prices.Tick
	err := p.cache.Get(ctx, historyKey, &existingTicks)
	if err != nil && err != store.ErrCacheMiss {
		return fmt.Errorf("failed to get existing ticks: %w", err)
	}

	// Add new tick
	existingTicks = append(existingTicks, tick)

	// Maintain maximum number of ticks
	if len(existingTicks) > p.config.MaxTicksPerSym {
		// Remove oldest ticks
		existingTicks = existingTicks[len(existingTicks)-p.config.MaxTicksPerSym:]
	}

	// Save back to cache
	if err := p.cache.Set(ctx, historyKey, existingTicks, p.config.TTL); err != nil {
		return fmt.Errorf("failed to save tick history: %w", err)
	}

	return nil
}

// DefaultConfig returns a reasonable default configuration
func DefaultPricePublisherConfig() PricePublisherConfig {
	return PricePublisherConfig{
		ProviderType:   "binance",
		RetryInterval:  5 * time.Second,
		MaxTicksPerSym: 10000,           // Keep last 10k ticks per symbol
		TTL:            5 * time.Second, // Cache TTL for latest price
		MockVolatility: 0.002,           // 0.2% volatility for mock data
		MockBasePrice:  1.00,            // Default SUI price
	}
}
