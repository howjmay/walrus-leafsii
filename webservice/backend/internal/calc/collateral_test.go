package calc

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestCollateralRatio(t *testing.T) {
	tests := []struct {
		name      string
		reservesR decimal.Decimal
		supplyF   decimal.Decimal
		expected  decimal.Decimal
	}{
		{
			name:      "normal case",
			reservesR: decimal.NewFromInt(150),
			supplyF:   decimal.NewFromInt(100),
			expected:  decimal.NewFromFloat(1.5),
		},
		{
			name:      "zero supply",
			reservesR: decimal.NewFromInt(100),
			supplyF:   decimal.Zero,
			expected:  decimal.Zero,
		},
		{
			name:      "equal reserves and supply",
			reservesR: decimal.NewFromInt(100),
			supplyF:   decimal.NewFromInt(100),
			expected:  decimal.NewFromInt(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CollateralRatio(tt.reservesR, tt.supplyF)
			assert.True(t, tt.expected.Equal(result), "expected %s, got %s", tt.expected, result)
		})
	}
}

func TestPegDeviation(t *testing.T) {
	tests := []struct {
		name         string
		fTokenPrice  decimal.Decimal
		expected     decimal.Decimal
	}{
		{
			name:        "perfect peg",
			fTokenPrice: decimal.NewFromInt(1),
			expected:    decimal.Zero,
		},
		{
			name:        "positive deviation",
			fTokenPrice: decimal.NewFromFloat(1.05),
			expected:    decimal.NewFromFloat(0.05),
		},
		{
			name:        "negative deviation",
			fTokenPrice: decimal.NewFromFloat(0.95),
			expected:    decimal.NewFromFloat(0.05),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PegDeviation(tt.fTokenPrice)
			assert.True(t, tt.expected.Equal(result), "expected %s, got %s", tt.expected, result)
		})
	}
}

func TestPostMintCR(t *testing.T) {
	currentReservesR := decimal.NewFromInt(150)
	currentSupplyF := decimal.NewFromInt(100)
	mintAmount := decimal.NewFromInt(50)

	result := PostMintCR(currentReservesR, currentSupplyF, mintAmount)
	expected := decimal.NewFromFloat(1.333333) // (150+50)/(100+50) = 200/150

	// Use approximate comparison for floating point
	diff := result.Sub(expected).Abs()
	tolerance := decimal.NewFromFloat(0.0001)
	assert.True(t, diff.LessThan(tolerance), "expected ~%s, got %s", expected, result)
}

func TestCalculateMintOutput(t *testing.T) {
	amountR := decimal.NewFromInt(100)
	feeRate := decimal.NewFromFloat(0.003) // 0.3%

	fOut, fee := CalculateMintOutput(amountR, feeRate)

	expectedFee := decimal.NewFromFloat(0.3)   // 100 * 0.003
	expectedFOut := decimal.NewFromFloat(99.7) // 100 - 0.3

	assert.True(t, expectedFee.Equal(fee), "expected fee %s, got %s", expectedFee, fee)
	assert.True(t, expectedFOut.Equal(fOut), "expected fOut %s, got %s", expectedFOut, fOut)
}

func TestValidateCRConstraint(t *testing.T) {
	minCR := decimal.NewFromFloat(1.1)

	// Valid case
	err := ValidateCRConstraint(decimal.NewFromFloat(1.2), minCR)
	assert.NoError(t, err)

	// Invalid case
	err = ValidateCRConstraint(decimal.NewFromFloat(1.05), minCR)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "breach minimum collateral ratio")
}

func TestIsRebalanceNeeded(t *testing.T) {
	targetCR := decimal.NewFromFloat(1.3)
	tolerance := decimal.NewFromFloat(0.1) // 10%

	// No rebalance needed
	needed := IsRebalanceNeeded(decimal.NewFromFloat(1.25), targetCR, tolerance)
	assert.False(t, needed)

	// Rebalance needed
	needed = IsRebalanceNeeded(decimal.NewFromFloat(1.5), targetCR, tolerance)
	assert.True(t, needed)
}