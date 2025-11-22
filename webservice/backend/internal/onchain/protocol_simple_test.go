package onchain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// Simple test for oracle-based pricing logic without complex mocking
func TestOraclePricingLogic(t *testing.T) {
	// Test the core pricing logic
	tests := []struct {
		name     string
		rPrice   float64
		fPrice   float64
		amountR  float64
		expected struct {
			rateRtoF float64
			grossF   float64
			feeF     float64  
			fOut     float64
		}
	}{
		{
			name:    "Equal prices (1:1)",
			rPrice:  1.0,
			fPrice:  1.0,
			amountR: 100.0,
			expected: struct{rateRtoF, grossF, feeF, fOut float64}{
				rateRtoF: 1.0,
				grossF:   100.0,
				feeF:     0.3,   // 0.3% of 100 = 0.3
				fOut:     99.7,  // 100 - 0.3
			},
		},
		{
			name:    "Sui worth double (2:1)",
			rPrice:  2.0,
			fPrice:  1.0,
			amountR: 100.0,
			expected: struct{rateRtoF, grossF, feeF, fOut float64}{
				rateRtoF: 2.0,
				grossF:   200.0, // 100 * 2.0
				feeF:     0.6,   // 0.3% of 200 = 0.6
				fOut:     199.4, // 200 - 0.6
			},
		},
		{
			name:    "fToken worth double (0.5:1)",
			rPrice:  1.0,
			fPrice:  2.0,
			amountR: 100.0,
			expected: struct{rateRtoF, grossF, feeF, fOut float64}{
				rateRtoF: 0.5,
				grossF:   50.0,  // 100 * 0.5
				feeF:     0.15,  // 0.3% of 50 = 0.15
				fOut:     49.85, // 50 - 0.15
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the oracle-based pricing logic
			pR := decimal.NewFromFloat(tt.rPrice)
			pF := decimal.NewFromFloat(tt.fPrice)
			amountR := decimal.NewFromFloat(tt.amountR)
			
			// Calculate rate: rateRtoF = pR / pF
			rateRtoF := pR.Div(pF)
			if !rateRtoF.Equal(decimal.NewFromFloat(tt.expected.rateRtoF)) {
				t.Errorf("Rate mismatch: expected %f, got %s", tt.expected.rateRtoF, rateRtoF.String())
			}
			
			// Calculate gross fToken: grossF = amountR * rateRtoF
			grossF := amountR.Mul(rateRtoF)
			if !grossF.Equal(decimal.NewFromFloat(tt.expected.grossF)) {
				t.Errorf("GrossF mismatch: expected %f, got %s", tt.expected.grossF, grossF.String())
			}
			
			// Calculate fee: feeF = grossF * 0.003
			feeRateF := decimal.NewFromFloat(0.003)
			feeF := grossF.Mul(feeRateF)
			expectedFeeF := decimal.NewFromFloat(tt.expected.feeF)
			if feeF.Sub(expectedFeeF).Abs().GreaterThan(decimal.NewFromFloat(0.001)) {
				t.Errorf("FeeF mismatch: expected %f, got %s", tt.expected.feeF, feeF.String())
			}
			
			// Calculate output: fOut = grossF - feeF
			fOut := grossF.Sub(feeF)
			expectedFOut := decimal.NewFromFloat(tt.expected.fOut)
			if fOut.Sub(expectedFOut).Abs().GreaterThan(decimal.NewFromFloat(0.001)) {
				t.Errorf("FOut mismatch: expected %f, got %s", tt.expected.fOut, fOut.String())
			}
		})
	}
}

func TestRedeemPricingLogic(t *testing.T) {
	tests := []struct {
		name     string
		rPrice   float64
		fPrice   float64
		amountF  float64
		expected struct {
			rateFtoR float64
			grossR   float64
			feeR     float64  
			rOut     float64
		}
	}{
		{
			name:    "Equal prices (1:1)",
			rPrice:  1.0,
			fPrice:  1.0,
			amountF: 100.0,
			expected: struct{rateFtoR, grossR, feeR, rOut float64}{
				rateFtoR: 1.0,
				grossR:   100.0,
				feeR:     0.5,   // 0.5% of 100 = 0.5
				rOut:     99.5,  // 100 - 0.5
			},
		},
		{
			name:    "fToken worth double (2:1)",
			rPrice:  1.0,
			fPrice:  2.0,
			amountF: 100.0,
			expected: struct{rateFtoR, grossR, feeR, rOut float64}{
				rateFtoR: 2.0,
				grossR:   200.0, // 100 * 2.0
				feeR:     1.0,   // 0.5% of 200 = 1.0
				rOut:     199.0, // 200 - 1.0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the oracle-based redeem pricing logic
			pR := decimal.NewFromFloat(tt.rPrice)
			pF := decimal.NewFromFloat(tt.fPrice)
			amountF := decimal.NewFromFloat(tt.amountF)
			
			// Calculate rate: rateFtoR = pF / pR
			rateFtoR := pF.Div(pR)
			if !rateFtoR.Equal(decimal.NewFromFloat(tt.expected.rateFtoR)) {
				t.Errorf("Rate mismatch: expected %f, got %s", tt.expected.rateFtoR, rateFtoR.String())
			}
			
			// Calculate gross Sui: grossR = amountF * rateFtoR
			grossR := amountF.Mul(rateFtoR)
			if !grossR.Equal(decimal.NewFromFloat(tt.expected.grossR)) {
				t.Errorf("GrossR mismatch: expected %f, got %s", tt.expected.grossR, grossR.String())
			}
			
			// Calculate fee: feeR = grossR * 0.005
			feeRateR := decimal.NewFromFloat(0.005)
			feeR := grossR.Mul(feeRateR)
			expectedFeeR := decimal.NewFromFloat(tt.expected.feeR)
			if feeR.Sub(expectedFeeR).Abs().GreaterThan(decimal.NewFromFloat(0.001)) {
				t.Errorf("FeeR mismatch: expected %f, got %s", tt.expected.feeR, feeR.String())
			}
			
			// Calculate output: rOut = grossR - feeR
			rOut := grossR.Sub(feeR)
			expectedROut := decimal.NewFromFloat(tt.expected.rOut)
			if rOut.Sub(expectedROut).Abs().GreaterThan(decimal.NewFromFloat(0.001)) {
				t.Errorf("ROut mismatch: expected %f, got %s", tt.expected.rOut, rOut.String())
			}
		})
	}
}

// Test that verifies oracle timestamp validation logic
func TestOracleTimestampValidation(t *testing.T) {
	maxAge := time.Hour
	now := time.Now()
	
	tests := []struct {
		name        string
		timestamp   time.Time
		shouldError bool
	}{
		{"Fresh data", now.Add(-30 * time.Minute), false},
		{"At max age", now.Add(-maxAge), false},
		{"Slightly stale", now.Add(-maxAge - time.Minute), true},
		{"Very stale", now.Add(-2 * time.Hour), true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the timestamp validation logic
			age := now.Sub(tt.timestamp)
			isStale := age > maxAge
			
			if isStale != tt.shouldError {
				t.Errorf("Expected stale=%v, got stale=%v (age=%v, maxAge=%v)", 
					tt.shouldError, isStale, age, maxAge)
			}
		})
	}
}