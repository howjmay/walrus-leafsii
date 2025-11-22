package prices

import (
	"context"
	"time"
)

// Tick represents a single price update
type Tick struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
	TsMs   int64   `json:"ts"` // milliseconds since epoch
}

// Candle represents OHLCV data for a time period
type Candle struct {
	Time   int64   `json:"time"`   // unix seconds, aligned to interval boundary
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// Provider defines the interface for price data sources
type Provider interface {
	// FetchHistory retrieves historical candle data
	// symbol: provider-specific symbol (e.g., "SUIUSDT")
	// interval: candle interval duration
	// limit: maximum number of candles to return
	FetchHistory(ctx context.Context, symbol string, interval time.Duration, limit int) ([]Candle, error)

	// SubscribeLive subscribes to real-time price updates
	// symbol: provider-specific symbol
	// out: channel to receive tick updates
	SubscribeLive(ctx context.Context, symbol string, out chan<- Tick) error

	// Name returns the provider identifier
	Name() string

	// Health returns current provider health status
	Health() ProviderHealth
}

// ProviderHealth represents the current status of a provider
type ProviderHealth struct {
	Healthy     bool      `json:"healthy"`
	LastError   string    `json:"last_error,omitempty"`
	LastSuccess time.Time `json:"last_success"`
	Reconnects  int       `json:"reconnects"`
}

// IntervalString converts time.Duration to provider-specific interval string
func IntervalString(d time.Duration) string {
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

// ParseInterval converts interval string to time.Duration
func ParseInterval(interval string) time.Duration {
	switch interval {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "4h":
		return 4 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}

// AlignTime aligns timestamp to interval boundary
func AlignTime(ts time.Time, interval time.Duration) time.Time {
	unix := ts.Unix()
	intervalSec := int64(interval.Seconds())
	aligned := (unix / intervalSec) * intervalSec
	return time.Unix(aligned, 0)
}