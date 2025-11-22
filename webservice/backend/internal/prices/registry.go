package prices

import (
	"fmt"
	"strings"
)

// PairMapping maps UI pairs to provider symbols
type PairMapping struct {
	UIPair         string // e.g., "SUI/USD"
	ProviderSymbol string // e.g., "SUIUSDT"
}

// Registry manages provider selection and pair mapping
type Registry struct {
	mappings map[string]string // UI pair -> provider symbol
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	r := &Registry{
		mappings: make(map[string]string),
	}

	// Default mappings
	r.AddMapping("SUI/USD", "SUIUSDT")
	r.AddMapping("SUI/USDT", "SUIUSDT")
	r.AddMapping("SUI/fToken", "SUIUSDT") // Map to real SUI price
	r.AddMapping("ETH/USD", "ETHUSDT")
	r.AddMapping("ETH/USDT", "ETHUSDT")

	return r
}

// AddMapping adds a UI pair to provider symbol mapping
func (r *Registry) AddMapping(uiPair, providerSymbol string) {
	r.mappings[strings.ToUpper(uiPair)] = strings.ToUpper(providerSymbol)
}

// GetProviderSymbol returns the provider symbol for a UI pair
func (r *Registry) GetProviderSymbol(uiPair string) (string, error) {
	symbol, exists := r.mappings[strings.ToUpper(uiPair)]
	if !exists {
		return "", fmt.Errorf("no mapping found for pair: %s", uiPair)
	}
	return symbol, nil
}

// GetAllMappings returns all configured mappings
func (r *Registry) GetAllMappings() map[string]string {
	result := make(map[string]string)
	for k, v := range r.mappings {
		result[k] = v
	}
	return result
}

// GetProviderSymbols returns the unique provider symbols we need to subscribe to
func (r *Registry) GetProviderSymbols() []string {
	seen := make(map[string]struct{})
	symbols := make([]string, 0, len(r.mappings))

	for _, sym := range r.mappings {
		upper := strings.ToUpper(sym)
		if _, exists := seen[upper]; exists {
			continue
		}
		seen[upper] = struct{}{}
		symbols = append(symbols, upper)
	}

	return symbols
}

// ValidatePair checks if a UI pair is supported
func (r *Registry) ValidatePair(uiPair string) bool {
	_, exists := r.mappings[strings.ToUpper(uiPair)]
	return exists
}
