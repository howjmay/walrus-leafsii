package onchain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/leafsii/leafsii-backend/internal/calc"
	"github.com/leafsii/leafsii-backend/internal/config"
	"github.com/leafsii/leafsii-backend/internal/store"
	"github.com/leafsii/leafsii-backend/internal/util"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type ProtocolService struct {
	chain  ChainReader
	cache  *store.Cache
	config *config.Config
	logger *zap.SugaredLogger
	sf     *util.Group // singleflight to dedupe expensive calls
}

type ProtocolHealth struct {
	Reasons []string
}

func NewProtocolService(
	chain ChainReader,
	cache *store.Cache,
	config *config.Config,
	logger *zap.SugaredLogger,
) *ProtocolService {
	return &ProtocolService{
		chain:  chain,
		cache:  cache,
		config: config,
		logger: logger,
		sf:     &util.Group{},
	}
}

func (s *ProtocolService) GetState(ctx context.Context) (*ProtocolState, error) {
	result, err, _ := s.sf.Do("protocol-state", func() (interface{}, error) {
		return s.getStateInternal(ctx)
	})
	if err != nil {
		return nil, err
	}
	return result.(*ProtocolState), nil
}

func (s *ProtocolService) getStateInternal(ctx context.Context) (*ProtocolState, error) {
	// Try cache first
	var cachedState ProtocolState
	if err := s.cache.GetProtocolState(ctx, &cachedState); err == nil {
		return &cachedState, nil
	}

	// Cache miss - fetch from chain
	state, err := s.chain.ProtocolState(ctx)
	if err != nil {
		s.logger.Errorw("Failed to fetch protocol state from chain", "error", err)
		return nil, fmt.Errorf("failed to fetch protocol state: %w", err)
	}

	// Validate the state
	if err := calc.ValidateProtocolState(state.ReservesR, state.SupplyF, state.CR); err != nil {
		s.logger.Warnw("Invalid protocol state received", "error", err, "state", state)
		// Don't cache invalid state, but still return it for debugging
		return state, nil
	}

	// Cache the valid state
	if err := s.cache.SetProtocolState(ctx, state); err != nil {
		s.logger.Warnw("Failed to cache protocol state", "error", err)
		// Continue even if caching fails
	}

	return state, nil
}

func (s *ProtocolService) GetHealth(ctx context.Context) (*ProtocolHealth, error) {
	state, err := s.GetState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get protocol state for health check: %w", err)
	}

	var reasons []string

	// Check oracle age
	if state.OracleAgeSec > int64(s.config.Oracle.MaxAge.Seconds()) {
		reasons = append(reasons, "ORACLE_STALE")
	}

	// Check CR against minimum (assuming 110% minimum)
	minCR := decimal.NewFromFloat(1.1)
	if state.CR.LessThan(minCR) {
		reasons = append(reasons, "CR_BELOW_MINIMUM")
	}

	// Check peg deviation (warn if > 5%)
	maxDeviation := decimal.NewFromFloat(0.05)
	if state.PegDeviation.GreaterThan(maxDeviation) {
		reasons = append(reasons, "PEG_DEVIATION_HIGH")
	}

	// Check if reserves are dangerously low
	reserveRatio := state.ReservesR.Div(state.SupplyF)
	if reserveRatio.LessThan(decimal.NewFromFloat(0.5)) {
		reasons = append(reasons, "RESERVES_LOW")
	}

	return &ProtocolHealth{Reasons: reasons}, nil
}

// QuoteService handles quote generation with TTL and caching
type QuoteService struct {
	chain    ChainReader
	cache    *store.Cache
	protocol *ProtocolService
	config   *config.Config
	logger   *zap.SugaredLogger
	sf       *util.Group
}

type MintQuote struct {
	FOut    decimal.Decimal
	Fee     decimal.Decimal
	PostCR  decimal.Decimal
	TTLSec  int
	QuoteID string
	AsOf    time.Time
}

type RedeemQuote struct {
	ROut    decimal.Decimal
	Fee     decimal.Decimal
	PostCR  decimal.Decimal
	TTLSec  int
	QuoteID string
	AsOf    time.Time
}

type MintXQuote struct {
	XOut    decimal.Decimal
	Fee     decimal.Decimal
	PostCR  decimal.Decimal
	TTLSec  int
	QuoteID string
	AsOf    time.Time
}

type RedeemXQuote struct {
	ROut    decimal.Decimal
	Fee     decimal.Decimal
	PostCR  decimal.Decimal
	TTLSec  int
	QuoteID string
	AsOf    time.Time
}

func NewQuoteService(
	chain ChainReader,
	cache *store.Cache,
	protocol *ProtocolService,
	config *config.Config,
	logger *zap.SugaredLogger,
) *QuoteService {
	return &QuoteService{
		chain:    chain,
		cache:    cache,
		protocol: protocol,
		config:   config,
		logger:   logger,
		sf:       &util.Group{},
	}
}

// fetchAndValidateOraclePrices fetches oracle prices for both tokens and validates freshness
func (s *QuoteService) fetchAndValidateOraclePrices(ctx context.Context) (rPrice, fPrice decimal.Decimal, err error) {
	// Fetch prices for both tokens
	pR, tR, err := s.chain.GetOraclePrice(ctx, "RTOKEN")
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("failed to get RTOKEN price: %w", err)
	}

	pF, tF, err := s.chain.GetOraclePrice(ctx, "FTOKEN")
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("failed to get FTOKEN price: %w", err)
	}

	// Validate oracle freshness for both prices
	maxAge := s.config.Oracle.MaxAge
	now := time.Now()

	if now.Sub(tR) > maxAge {
		return decimal.Zero, decimal.Zero, fmt.Errorf("RTOKEN oracle data too stale: %s > %s", now.Sub(tR), maxAge)
	}

	if now.Sub(tF) > maxAge {
		return decimal.Zero, decimal.Zero, fmt.Errorf("FTOKEN oracle data too stale: %s > %s", now.Sub(tF), maxAge)
	}

	// Validate prices are positive
	if pR.IsZero() || pR.IsNegative() {
		return decimal.Zero, decimal.Zero, fmt.Errorf("invalid RTOKEN price: %s", pR)
	}

	if pF.IsZero() || pF.IsNegative() {
		return decimal.Zero, decimal.Zero, fmt.Errorf("invalid FTOKEN price: %s", pF)
	}

	return pR, pF, nil
}

func (s *QuoteService) GetMintQuote(ctx context.Context, amountR decimal.Decimal) (*MintQuote, error) {
	// Get protocol state
	amountR = amountR.Mul(decimal.NewFromFloat(1000_000_000))

	state, err := s.protocol.GetState(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch and validate oracle prices
	pR, pF, err := s.fetchAndValidateOraclePrices(ctx)
	if err != nil {
		return nil, err
	}

	// Calculate cross-token exchange rate: rateRtoF = pR / pF (amount of f per 1 r)
	rateRtoF := pR.Div(pF)

	// Calculate mint quote with oracle-based pricing
	feeRateF := decimal.NewFromFloat(0.003) // 0.3% fee in fToken units

	// grossF = amountR * rateRtoF (gross fToken before fee)
	grossF := amountR.Mul(rateRtoF)

	// feeF = grossF * feeRateF (fee in fToken units)
	feeF := grossF.Mul(feeRateF)

	// fOut = grossF - feeF (net fToken output)
	fOut := grossF.Sub(feeF)

	// Calculate post-transaction CR using new reserves and supply
	// newReservesR = current reserves + full amountR input (fees are in fToken units)
	newReservesR := state.ReservesR.Add(amountR)
	// newSupplyF = current supply + net fToken output
	newSupplyF := state.SupplyF.Add(fOut)

	postCR := calc.CollateralRatio(newReservesR.Mul(decimal.NewFromInt(int64(state.P))), newSupplyF.Mul(decimal.NewFromInt(int64(state.Pf))))

	// Validate CR constraint (assuming 110% minimum)
	minCR := decimal.NewFromFloat(1.1)
	if err := calc.ValidateCRConstraint(postCR, minCR); err != nil {
		return nil, fmt.Errorf("mintF would breach CR constraint: %w", err)
	}

	quote := &MintQuote{
		FOut:    fOut.Div(decimal.NewFromInt(1000_000_000)),
		Fee:     feeF,
		PostCR:  postCR,
		TTLSec:  30, // 30 second TTL for quotes
		QuoteID: generateQuoteID(),
		AsOf:    time.Now(),
	}

	// Cache the quote for the TTL period
	if err := s.cache.SetQuote(ctx, "mint", quote.QuoteID, quote, time.Duration(quote.TTLSec)*time.Second); err != nil {
		s.logger.Warnw("Failed to cache mint quote", "error", err)
	}

	return quote, nil
}

func (s *QuoteService) GetRedeemQuote(ctx context.Context, amountF decimal.Decimal) (*RedeemQuote, error) {
	// Get protocol state
	state, err := s.protocol.GetState(ctx)
	if err != nil {
		return nil, err
	}

	// Validate sufficient supply
	if amountF.GreaterThan(state.SupplyF) {
		return nil, fmt.Errorf("insufficient fToken supply: requested %s > available %s", amountF, state.SupplyF)
	}

	// Fetch and validate oracle prices
	pR, pF, err := s.fetchAndValidateOraclePrices(ctx)
	if err != nil {
		return nil, err
	}

	// Calculate cross-token exchange rate: rateFtoR = pF / pR (amount of r per 1 f)
	rateFtoR := pF.Div(pR)

	// Calculate redeem quote with oracle-based pricing
	feeRateR := decimal.NewFromFloat(0.005) // 0.5% fee in Sui units

	// grossR = amountF * rateFtoR (gross Sui before fee)
	grossR := amountF.Mul(rateFtoR)

	// feeR = grossR * feeRateR (fee in Sui units)
	feeR := grossR.Mul(feeRateR)

	// rOut = grossR - feeR (net Sui output)
	rOut := grossR.Sub(feeR)

	// Calculate post-transaction CR using new reserves and supply
	// newReservesR = current reserves - net Sui output
	newReservesR := state.ReservesR.Sub(rOut)
	// newSupplyF = current supply - redeemed fToken amount
	newSupplyF := state.SupplyF.Sub(amountF)

	postCR := calc.CollateralRatio(newReservesR.Mul(decimal.NewFromInt(int64(state.P))), newSupplyF.Mul(decimal.NewFromInt(int64(state.Pf))))

	// Validate CR constraint
	minCR := decimal.NewFromFloat(1.1)
	if err := calc.ValidateCRConstraint(postCR, minCR); err != nil {
		return nil, fmt.Errorf("redeem would breach CR constraint: %w", err)
	}

	quote := &RedeemQuote{
		ROut:    rOut,
		Fee:     feeR,
		PostCR:  postCR,
		TTLSec:  30, // 30 second TTL for quotes
		QuoteID: generateQuoteID(),
		AsOf:    time.Now(),
	}

	// Cache the quote for the TTL period
	if err := s.cache.SetQuote(ctx, "redeem", quote.QuoteID, quote, time.Duration(quote.TTLSec)*time.Second); err != nil {
		s.logger.Warnw("Failed to cache redeem quote", "error", err)
	}

	return quote, nil
}

func (s *QuoteService) GetMintXQuote(ctx context.Context, amountR decimal.Decimal) (*MintXQuote, error) {
	// Validate oracle freshness first
	state, err := s.protocol.GetState(ctx)
	if err != nil {
		return nil, err
	}

	if state.OracleAgeSec > int64(s.config.Oracle.MaxAge.Seconds()) {
		return nil, fmt.Errorf("oracle data too stale: %ds > %s", state.OracleAgeSec, s.config.Oracle.MaxAge)
	}

	// Calculate mint X quote with higher fees
	feeRate := decimal.NewFromFloat(0.02)           // 2% fee for xToken
	xOut := amountR.Mul(decimal.NewFromFloat(0.98)) // 98% conversion rate
	fee := amountR.Mul(feeRate)
	postCR := calc.PostMintCR(state.ReservesR, state.SupplyX, amountR) // Use SupplyX for xToken

	// Validate CR constraint (assuming 110% minimum)
	minCR := decimal.NewFromFloat(1.1)
	if err := calc.ValidateCRConstraint(postCR, minCR); err != nil {
		return nil, fmt.Errorf("mintX would breach CR constraint: %w", err)
	}

	quote := &MintXQuote{
		XOut:    xOut,
		Fee:     fee,
		PostCR:  postCR,
		TTLSec:  30, // 30 second TTL for quotes
		QuoteID: generateQuoteID(),
		AsOf:    time.Now(),
	}

	// Cache the quote for the TTL period
	if err := s.cache.SetQuote(ctx, "mintX", quote.QuoteID, quote, time.Duration(quote.TTLSec)*time.Second); err != nil {
		s.logger.Warnw("Failed to cache mintX quote", "error", err)
	}

	return quote, nil
}

func (s *QuoteService) GetRedeemXQuote(ctx context.Context, amountX decimal.Decimal) (*RedeemXQuote, error) {
	// Validate oracle freshness first
	state, err := s.protocol.GetState(ctx)
	if err != nil {
		return nil, err
	}

	if state.OracleAgeSec > int64(s.config.Oracle.MaxAge.Seconds()) {
		return nil, fmt.Errorf("oracle data too stale: %ds > %s", state.OracleAgeSec, s.config.Oracle.MaxAge)
	}

	// Validate sufficient supply
	if amountX.GreaterThan(state.SupplyX) {
		return nil, fmt.Errorf("insufficient xToken supply: requested %s > available %s", amountX, state.SupplyX)
	}

	// Calculate redeem X quote - xToken can be profitable to redeem
	feeRate := decimal.NewFromFloat(0.02)           // 2% fee
	rOut := amountX.Mul(decimal.NewFromFloat(1.02)) // 102% conversion rate (profitable)
	fee := amountX.Mul(feeRate)
	postCR := calc.PostRedeemCR(state.ReservesR, state.SupplyX, amountX) // Use SupplyX for xToken

	// Validate CR constraint
	minCR := decimal.NewFromFloat(1.1)
	if err := calc.ValidateCRConstraint(postCR, minCR); err != nil {
		return nil, fmt.Errorf("redeemX would breach CR constraint: %w", err)
	}

	quote := &RedeemXQuote{
		ROut:    rOut,
		Fee:     fee,
		PostCR:  postCR,
		TTLSec:  30, // 30 second TTL for quotes
		QuoteID: generateQuoteID(),
		AsOf:    time.Now(),
	}

	// Cache the quote for the TTL period
	if err := s.cache.SetQuote(ctx, "redeemX", quote.QuoteID, quote, time.Duration(quote.TTLSec)*time.Second); err != nil {
		s.logger.Warnw("Failed to cache redeemX quote", "error", err)
	}

	return quote, nil
}

func generateQuoteID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
