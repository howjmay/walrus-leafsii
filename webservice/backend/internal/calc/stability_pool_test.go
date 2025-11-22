package calc

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestCalculateNewSPIndex(t *testing.T) {
	tests := []struct {
		name          string
		previousIndex decimal.Decimal
		payoutR       decimal.Decimal
		totalStakeF   decimal.Decimal
		expected      decimal.Decimal
	}{
		{
			name:          "normal case",
			previousIndex: decimal.NewFromFloat(1.0),
			payoutR:       decimal.NewFromInt(10),
			totalStakeF:   decimal.NewFromInt(100),
			expected:      decimal.NewFromFloat(1.1), // 1.0 + 10/100
		},
		{
			name:          "zero stake",
			previousIndex: decimal.NewFromFloat(1.0),
			payoutR:       decimal.NewFromInt(10),
			totalStakeF:   decimal.Zero,
			expected:      decimal.NewFromFloat(1.0), // no change
		},
		{
			name:          "zero payout",
			previousIndex: decimal.NewFromFloat(1.0),
			payoutR:       decimal.Zero,
			totalStakeF:   decimal.NewFromInt(100),
			expected:      decimal.NewFromFloat(1.0), // no change
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateNewSPIndex(tt.previousIndex, tt.payoutR, tt.totalStakeF)
			assert.True(t, tt.expected.Equal(result), "expected %s, got %s", tt.expected, result)
		})
	}
}

func TestCalculateClaimableRewards(t *testing.T) {
	tests := []struct {
		name         string
		stakeF       decimal.Decimal
		indexAtJoin  decimal.Decimal
		currentIndex decimal.Decimal
		expected     decimal.Decimal
	}{
		{
			name:         "normal rewards",
			stakeF:       decimal.NewFromInt(100),
			indexAtJoin:  decimal.NewFromFloat(1.0),
			currentIndex: decimal.NewFromFloat(1.1),
			expected:     decimal.NewFromInt(10), // 100 * (1.1 - 1.0)
		},
		{
			name:         "no rewards",
			stakeF:       decimal.NewFromInt(100),
			indexAtJoin:  decimal.NewFromFloat(1.1),
			currentIndex: decimal.NewFromFloat(1.1),
			expected:     decimal.Zero,
		},
		{
			name:         "index decreased",
			stakeF:       decimal.NewFromInt(100),
			indexAtJoin:  decimal.NewFromFloat(1.1),
			currentIndex: decimal.NewFromFloat(1.0),
			expected:     decimal.Zero, // can't have negative rewards
		},
		{
			name:         "zero stake",
			stakeF:       decimal.Zero,
			indexAtJoin:  decimal.NewFromFloat(1.0),
			currentIndex: decimal.NewFromFloat(1.1),
			expected:     decimal.Zero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateClaimableRewards(tt.stakeF, tt.indexAtJoin, tt.currentIndex)
			assert.True(t, tt.expected.Equal(result), "expected %s, got %s", tt.expected, result)
		})
	}
}

func TestCalculateAPR(t *testing.T) {
	rewardsLast24h := decimal.NewFromInt(1) // 1 Sui reward
	tvlF := decimal.NewFromInt(100)         // 100 fTokens staked

	result := CalculateAPR(rewardsLast24h, tvlF)
	
	// Expected: (1/100) * 365 * 100 = 365% APR
	expected := decimal.NewFromInt(365)
	
	assert.True(t, expected.Equal(result), "expected %s%% APR, got %s%%", expected, result)
}

func TestCalculateAPRZeroTVL(t *testing.T) {
	rewardsLast24h := decimal.NewFromInt(1)
	tvlF := decimal.Zero

	result := CalculateAPR(rewardsLast24h, tvlF)
	
	assert.True(t, decimal.Zero.Equal(result), "expected 0%% APR when TVL is zero, got %s%%", result)
}