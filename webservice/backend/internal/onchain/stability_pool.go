package onchain

import (
	"context"
	"fmt"
	"time"

	"github.com/leafsii/leafsii-backend/internal/calc"
	"github.com/leafsii/leafsii-backend/internal/store"
	"github.com/leafsii/leafsii-backend/internal/util"
	"github.com/pattonkan/sui-go/sui"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type StabilityPoolService struct {
	chain  ChainReader
	cache  *store.Cache
	logger *zap.SugaredLogger
	sf     *util.Group
}

type SPIndexInfo struct {
	Current     decimal.Decimal
	Previous24h decimal.Decimal
	APR         decimal.Decimal
	TVLF        decimal.Decimal
}

type SPUserPosition struct {
	Address            string
	StakeF             decimal.Decimal
	EnteredAt          time.Time
	IndexAtJoin        decimal.Decimal
	ClaimableR         decimal.Decimal
	PendingIndexDelta  decimal.Decimal
}

func NewStabilityPoolService(
	chain ChainReader,
	cache *store.Cache,
	logger *zap.SugaredLogger,
) *StabilityPoolService {
	return &StabilityPoolService{
		chain:  chain,
		cache:  cache,
		logger: logger,
		sf:     &util.Group{},
	}
}

func (s *StabilityPoolService) GetIndex(ctx context.Context) (*SPIndexInfo, error) {
	result, err, _ := s.sf.Do("sp-index", func() (interface{}, error) {
		return s.getIndexInternal(ctx)
	})
	if err != nil {
		return nil, err
	}
	return result.(*SPIndexInfo), nil
}

func (s *StabilityPoolService) getIndexInternal(ctx context.Context) (*SPIndexInfo, error) {
	// Try cache first
	var cachedIndex SPIndexInfo
	if err := s.cache.GetSPIndex(ctx, &cachedIndex); err == nil {
		return &cachedIndex, nil
	}

	// Cache miss - fetch from chain
	currentIndex, err := s.chain.SPIndex(ctx)
	if err != nil {
		s.logger.Errorw("Failed to fetch SP index from chain", "error", err)
		return nil, fmt.Errorf("failed to fetch SP index: %w", err)
	}

	// For now, simulate 24h ago data (in reality, this would come from historical data)
	previous24h := currentIndex.IndexValue.Sub(decimal.NewFromFloat(0.001)) // Mock 0.1% growth
	if previous24h.LessThan(decimal.Zero) {
		previous24h = decimal.Zero
	}

	// Calculate APR based on 24h change
	dailyReturn := decimal.Zero
	if !previous24h.IsZero() {
		dailyReturn = currentIndex.IndexValue.Sub(previous24h).Div(previous24h)
	}
	apr := dailyReturn.Mul(decimal.NewFromInt(365)).Mul(decimal.NewFromInt(100))

	info := &SPIndexInfo{
		Current:     currentIndex.IndexValue,
		Previous24h: previous24h,
		APR:         apr,
		TVLF:        currentIndex.TVLF,
	}

	// Cache the result
	if err := s.cache.SetSPIndex(ctx, info); err != nil {
		s.logger.Warnw("Failed to cache SP index", "error", err)
	}

	return info, nil
}

func (s *StabilityPoolService) GetUserPosition(ctx context.Context, address string) (*SPUserPosition, error) {
	key := fmt.Sprintf("sp-user-%s", address)
	result, err, _ := s.sf.Do(key, func() (interface{}, error) {
		return s.getUserPositionInternal(ctx, address)
	})
	if err != nil {
		return nil, err
	}
	return result.(*SPUserPosition), nil
}

func (s *StabilityPoolService) getUserPositionInternal(ctx context.Context, address string) (*SPUserPosition, error) {
	// Try cache first
	var cachedPosition SPUserPosition
	if err := s.cache.GetUserPosition(ctx, address, &cachedPosition); err == nil {
		return &cachedPosition, nil
	}

	// Cache miss - fetch user positions from chain
	addr, err := sui.AddressFromHex(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address format: %w", err)
	}
	userPos, err := s.chain.UserPositions(ctx, addr)
	if err != nil {
		s.logger.Errorw("Failed to fetch user positions from chain", "address", address, "error", err)
		return nil, fmt.Errorf("failed to fetch user positions: %w", err)
	}

	// If user has no stake, return empty position
	if userPos.StakeF.IsZero() {
		position := &SPUserPosition{
			Address:            address,
			StakeF:             decimal.Zero,
			EnteredAt:          time.Now(),
			IndexAtJoin:        decimal.Zero,
			ClaimableR:         decimal.Zero,
			PendingIndexDelta:  decimal.Zero,
		}
		return position, nil
	}

	// Get current SP index to calculate claimable rewards
	currentIndex, err := s.chain.SPIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch current SP index: %w", err)
	}

	// Calculate claimable rewards
	claimableR := calc.CalculateClaimableRewards(userPos.StakeF, userPos.IndexAtJoin, currentIndex.IndexValue)

	// Calculate pending index delta (how much index has grown since user joined)
	pendingDelta := currentIndex.IndexValue.Sub(userPos.IndexAtJoin)
	if pendingDelta.LessThan(decimal.Zero) {
		pendingDelta = decimal.Zero
	}

	position := &SPUserPosition{
		Address:            address,
		StakeF:             userPos.StakeF,
		EnteredAt:          userPos.UpdatedAt, // Use last update time as proxy for entry
		IndexAtJoin:        userPos.IndexAtJoin,
		ClaimableR:         claimableR,
		PendingIndexDelta:  pendingDelta,
	}

	// Cache the result
	if err := s.cache.SetUserPosition(ctx, address, position); err != nil {
		s.logger.Warnw("Failed to cache user SP position", "address", address, "error", err)
	}

	return position, nil
}

func (s *StabilityPoolService) GetStakePreview(ctx context.Context, stakeAmount decimal.Decimal) (expectedIndexDelta decimal.Decimal, estAPR decimal.Decimal, err error) {
	// Get current SP info
	spInfo, err := s.GetIndex(ctx)
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("failed to get SP index: %w", err)
	}

	// New stakes enter at the current index, so expected index delta is 0
	expectedIndexDelta = decimal.Zero

	// Return current APR as estimate
	estAPR = spInfo.APR

	return expectedIndexDelta, estAPR, nil
}