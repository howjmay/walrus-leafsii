package mock

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/leafsii/leafsii-backend/internal/prices"
	"go.uber.org/zap"
)

// Generator provides mock price data for testing and fallback scenarios
type Generator struct {
	logger     *zap.SugaredLogger
	mu         sync.RWMutex
	basePrice  float64
	volatility float64
	health     prices.ProviderHealth
	rng        *rand.Rand
}

// NewGenerator creates a new mock data generator
func NewGenerator(logger *zap.SugaredLogger, basePrice, volatility float64) *Generator {
	if basePrice <= 0 {
		basePrice = 1.00 // Default SUI price
	}
	if volatility <= 0 {
		volatility = 0.002 // 0.2% volatility
	}
	
	return &Generator{
		logger:     logger,
		basePrice:  basePrice,
		volatility: volatility,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
		health: prices.ProviderHealth{
			Healthy:     true,
			LastSuccess: time.Now(),
		},
	}
}

// Name returns the provider identifier
func (g *Generator) Name() string {
	return "mock"
}

// Health returns current provider health status
func (g *Generator) Health() prices.ProviderHealth {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.health
}

// FetchHistory generates mock historical candle data
func (g *Generator) FetchHistory(ctx context.Context, symbol string, interval time.Duration, limit int) ([]prices.Candle, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.health.LastSuccess = time.Now()
	
	candles := make([]prices.Candle, limit)
	
	// Start from current time and go backwards
	currentTime := time.Now()
	alignedTime := prices.AlignTime(currentTime, interval)
	
	// Initialize with base price
	lastClose := g.basePrice
	
	// Generate candles backwards in time
	for i := 0; i < limit; i++ {
		candleTime := alignedTime.Add(-time.Duration(limit-i-1) * interval)
		
		candle := g.generateCandle(candleTime, lastClose, interval)
		candles[i] = candle
		lastClose = candle.Close
	}
	
	g.logger.Debugw("Generated mock history", 
		"symbol", symbol, 
		"interval", interval, 
		"candles", len(candles),
		"basePrice", g.basePrice,
	)
	
	return candles, nil
}

// SubscribeLive generates mock real-time price updates
func (g *Generator) SubscribeLive(ctx context.Context, symbol string, out chan<- prices.Tick) error {
	g.mu.Lock()
	g.health.LastSuccess = time.Now()
	currentPrice := g.basePrice
	g.mu.Unlock()
	
	g.logger.Infow("Starting mock live price feed", "symbol", symbol, "basePrice", currentPrice)
	
	ticker := time.NewTicker(1500 * time.Millisecond) // ~1.5s intervals
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Generate price movement
			change := g.generatePriceChange()
			currentPrice *= (1 + change)
			
			// Ensure price stays within reasonable bounds (±50% of base)
			minPrice := g.basePrice * 0.5
			maxPrice := g.basePrice * 1.5
			if currentPrice < minPrice {
				currentPrice = minPrice
			} else if currentPrice > maxPrice {
				currentPrice = maxPrice
			}
			
			tick := prices.Tick{
				Symbol: symbol,
				Price:  currentPrice,
				TsMs:   time.Now().UnixMilli(),
			}
			
			// Send tick (non-blocking)
			select {
			case out <- tick:
			case <-ctx.Done():
				return ctx.Err()
			default:
				// Channel full, skip this tick
			}
			
			g.mu.Lock()
			g.health.LastSuccess = time.Now()
			g.mu.Unlock()
		}
	}
}

// generateCandle creates a single mock candle
func (g *Generator) generateCandle(candleTime time.Time, basePrice float64, interval time.Duration) prices.Candle {
	// Scale volatility by interval duration
	intervalMinutes := interval.Minutes()
	scaledVolatility := g.volatility * math.Sqrt(intervalMinutes)
	
	// Generate OHLC with realistic relationships
	open := basePrice
	
	// Generate random walk for the candle period
	numTicks := int(math.Max(1, intervalMinutes)) // At least 1 tick per candle
	tickPrices := make([]float64, numTicks+1)
	tickPrices[0] = open
	
	for i := 1; i <= numTicks; i++ {
		change := g.rng.NormFloat64() * scaledVolatility / math.Sqrt(float64(numTicks))
		tickPrices[i] = tickPrices[i-1] * (1 + change)
	}
	
	// Extract OHLC from the price series
	high := tickPrices[0]
	low := tickPrices[0]
	for _, p := range tickPrices {
		if p > high {
			high = p
		}
		if p < low {
			low = p
		}
	}
	close := tickPrices[len(tickPrices)-1]
	
	// Generate realistic volume
	baseVolume := 10000.0
	volumeMultiplier := 1 + g.rng.Float64() // 1-2x base volume
	volume := baseVolume * volumeMultiplier * intervalMinutes
	
	return prices.Candle{
		Time:   candleTime.Unix(),
		Open:   open,
		High:   high,
		Low:    low,
		Close:  close,
		Volume: volume,
	}
}

// generatePriceChange creates a realistic price movement
func (g *Generator) generatePriceChange() float64 {
	// Use normal distribution for price changes
	// Scale by volatility per second, then scale by actual time interval
	baseChange := g.rng.NormFloat64() * g.volatility
	
	// Add some trending behavior occasionally
	if g.rng.Float64() < 0.1 { // 10% chance of trend
		trend := (g.rng.Float64() - 0.5) * g.volatility * 2 // ±volatility trend
		baseChange += trend
	}
	
	// Clamp extreme movements
	maxChange := g.volatility * 5 // Max 5x volatility in one tick
	if baseChange > maxChange {
		baseChange = maxChange
	} else if baseChange < -maxChange {
		baseChange = -maxChange
	}
	
	return baseChange
}

// SetBasePrice updates the base price for mock generation
func (g *Generator) SetBasePrice(price float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if price > 0 {
		g.basePrice = price
	}
}

// GetBasePrice returns the current base price
func (g *Generator) GetBasePrice() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.basePrice
}