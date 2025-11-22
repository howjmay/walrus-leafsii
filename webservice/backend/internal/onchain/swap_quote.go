package onchain

import (
	"context"
	"fmt"
	"time"

	"github.com/leafsii/leafsii-backend/internal/config"
	"github.com/leafsii/leafsii-backend/internal/store"
	"github.com/leafsii/leafsii-backend/internal/util"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type SwapQuoteService struct {
	chain     ChainReader
	cache     *store.Cache
	protocol  *ProtocolService
	config    *config.Config
	logger    *zap.SugaredLogger
	sf        *util.Group
}

type SwapQuote struct {
	AmountIn    decimal.Decimal `json:"amountIn"`
	AmountOut   decimal.Decimal `json:"amountOut"`
	Price       decimal.Decimal `json:"price"`       // Quote per 1 unit of `from`
	SlippageBps int             `json:"slippageBps"` // Basis points (100 = 1%)
	FeeBps      int             `json:"feeBps"`      // Basis points (100 = 1%)
	Timestamp   int64           `json:"timestamp"`   // Unix timestamp
}

type PriceQuote struct {
	Price     decimal.Decimal `json:"price"`     // Price per 1 unit of `from`
	Timestamp int64           `json:"timestamp"` // Unix timestamp
}

func NewSwapQuoteService(
	chain ChainReader,
	cache *store.Cache,
	protocol *ProtocolService,
	config *config.Config,
	logger *zap.SugaredLogger,
) *SwapQuoteService {
	return &SwapQuoteService{
		chain:    chain,
		cache:    cache,
		protocol: protocol,
		config:   config,
		logger:   logger,
		sf:       &util.Group{},
	}
}

// GetSwapQuote calculates a swap quote between supported tokens
func (s *SwapQuoteService) GetSwapQuote(ctx context.Context, from, to string, amount decimal.Decimal) (*SwapQuote, error) {
	// Validate oracle freshness first
	state, err := s.protocol.GetState(ctx)
	if err != nil {
		return nil, err
	}

	if state.OracleAgeSec > int64(s.config.Oracle.MaxAge.Seconds()) {
		return nil, fmt.Errorf("oracle data too stale: %ds > %s", state.OracleAgeSec, s.config.Oracle.MaxAge)
	}

	// Validate token pair
	if !s.isSupportedTokenPair(from, to) {
		return nil, fmt.Errorf("unsupported token pair: %s -> %s", from, to)
	}

	// Validate amount
	if amount.IsNegative() || amount.IsZero() {
		return nil, fmt.Errorf("amount must be positive")
	}

	// TODO: Replace with actual DEX/AMM integration
	// For now, use a deterministic mock calculation based on protocol state
	exchangeRate, slippageBps, feeBps := s.calculateMockExchangeRate(from, to, amount, state)
	
	// Calculate fee amount
	feeRate := decimal.NewFromInt(int64(feeBps)).Div(decimal.NewFromInt(10000))
	feeAmount := amount.Mul(feeRate)
	amountAfterFee := amount.Sub(feeAmount)

	// Calculate output amount
	amountOut := amountAfterFee.Mul(exchangeRate)

	// Apply slippage (for demonstration)
	slippageRate := decimal.NewFromInt(int64(slippageBps)).Div(decimal.NewFromInt(10000))
	slippageAmount := amountOut.Mul(slippageRate)
	finalAmountOut := amountOut.Sub(slippageAmount)

	// Calculate final price (output per 1 unit of input)
	price := finalAmountOut.Div(amount)

	return &SwapQuote{
		AmountIn:    amount,
		AmountOut:   finalAmountOut,
		Price:       price,
		SlippageBps: slippageBps,
		FeeBps:      feeBps,
		Timestamp:   time.Now().Unix(),
	}, nil
}

// GetPrice returns just the exchange rate between two tokens
func (s *SwapQuoteService) GetPrice(ctx context.Context, from, to string) (*PriceQuote, error) {
	// Use a unit amount to calculate price
	unitAmount := decimal.NewFromInt(1)
	quote, err := s.GetSwapQuote(ctx, from, to, unitAmount)
	if err != nil {
		return nil, err
	}

	return &PriceQuote{
		Price:     quote.Price,
		Timestamp: quote.Timestamp,
	}, nil
}

// isSupportedTokenPair checks if the token pair is supported for swapping
func (s *SwapQuoteService) isSupportedTokenPair(from, to string) bool {
	supportedTokens := map[string]bool{
		"Sui": true,
		"fToken": true,
	}

	return supportedTokens[from] && supportedTokens[to] && from != to
}

// calculateMockExchangeRate provides a deterministic mock exchange rate
// TODO: Replace with actual DEX/AMM integration
func (s *SwapQuoteService) calculateMockExchangeRate(from, to string, amount decimal.Decimal, state *ProtocolState) (rate decimal.Decimal, slippageBps int, feeBps int) {
	// Base rate calculation using protocol state for realism
	baseRate := decimal.NewFromFloat(0.995)
	
	// Add some variation based on protocol collateralization ratio
	crVariation := state.CR.Sub(decimal.NewFromFloat(1.5)).Div(decimal.NewFromInt(10))
	baseRate = baseRate.Add(crVariation.Mul(decimal.NewFromFloat(0.01)))

	// Adjust rate based on swap direction
	if from == "Sui" && to == "fToken" {
		// Sui -> fToken: slightly better rate (minting scenario)
		baseRate = baseRate.Add(decimal.NewFromFloat(0.002))
	} else if from == "fToken" && to == "Sui" {
		// fToken -> Sui: slightly worse rate (redeeming scenario)
		baseRate = baseRate.Sub(decimal.NewFromFloat(0.003))
	}

	// Calculate slippage based on trade size
	// Larger trades get higher slippage
	slippageBps = 50 // 0.5% base slippage
	if amount.GreaterThan(decimal.NewFromInt(10000)) {
		slippageBps = 100 // 1% for large trades
	}

	// Fixed fee for swaps
	feeBps = 30 // 0.3% fee

	return baseRate, slippageBps, feeBps
}