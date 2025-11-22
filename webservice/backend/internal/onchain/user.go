package onchain

import (
	"context"
	"fmt"

	"github.com/leafsii/leafsii-backend/internal/store"
	"github.com/leafsii/leafsii-backend/internal/util"
	"github.com/pattonkan/sui-go/sui"
	"go.uber.org/zap"
)

type UserService struct {
	chain  ChainReader
	cache  *store.Cache
	logger *zap.SugaredLogger
	sf     *util.Group
}

func NewUserService(
	chain ChainReader,
	cache *store.Cache,
	logger *zap.SugaredLogger,
) *UserService {
	return &UserService{
		chain:  chain,
		cache:  cache,
		logger: logger,
		sf:     &util.Group{},
	}
}

func (s *UserService) GetPositions(ctx context.Context, address string) (*UserPositions, error) {
	key := fmt.Sprintf("user-positions-%s", address)
	result, err, _ := s.sf.Do(key, func() (interface{}, error) {
		return s.getPositionsInternal(ctx, address)
	})
	if err != nil {
		return nil, err
	}
	return result.(*UserPositions), nil
}

func (s *UserService) getPositionsInternal(ctx context.Context, address string) (*UserPositions, error) {
	// Try cache first
	var cachedPositions UserPositions
	if err := s.cache.GetUserPosition(ctx, address, &cachedPositions); err == nil {
		return &cachedPositions, nil
	}

	// Cache miss - fetch from chain
	addr, err := sui.AddressFromHex(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address format: %w", err)
	}
	positions, err := s.chain.UserPositions(ctx, addr)
	if err != nil {
		s.logger.Errorw("Failed to fetch user positions from chain", "address", address, "error", err)
		return nil, fmt.Errorf("failed to fetch user positions: %w", err)
	}

	// Cache the result with shorter TTL for user data
	if err := s.cache.SetUserPosition(ctx, address, positions); err != nil {
		s.logger.Warnw("Failed to cache user positions", "address", address, "error", err)
	}

	return positions, nil
}

func (s *UserService) GetBalances(ctx context.Context, address string) (*Balances, error) {
	key := fmt.Sprintf("user-balances-%s", address)
	result, err, _ := s.sf.Do(key, func() (interface{}, error) {
		return s.getBalancesInternal(ctx, address)
	})
	if err != nil {
		return nil, err
	}
	return result.(*Balances), nil
}

func (s *UserService) getBalancesInternal(ctx context.Context, address string) (*Balances, error) {
	// Fetch from chain - skipping cache for simplicity
	addr, err := sui.AddressFromHex(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address format: %w", err)
	}
	balances, err := s.chain.GetAllBalances(ctx, addr)
	if err != nil {
		s.logger.Errorw("Failed to fetch user balances from chain", "address", address, "error", err)
		return nil, fmt.Errorf("failed to fetch user balances: %w", err)
	}

	return balances, nil
}

// GetTransactions fetches user's recent transactions
// For now, this is a placeholder - in reality, it would query the events table
func (s *UserService) GetTransactions(ctx context.Context, address string, limit int, cursor string) ([]Event, string, error) {
	// TODO: Implement actual database query for user transactions
	// This would typically involve:
	// 1. Query events table filtered by user address
	// 2. Parse cursor for pagination
	// 3. Return events and next cursor

	s.logger.Debugw("GetTransactions called", "address", address, "limit", limit, "cursor", cursor)

	// Return empty results for now
	return []Event{}, "", nil
}
