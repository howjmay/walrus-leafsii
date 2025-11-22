package calc

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// ValidateOracleAge checks if oracle data is fresh enough
func ValidateOracleAge(oracleTimestamp time.Time, maxAge time.Duration) error {
	age := time.Since(oracleTimestamp)
	if age > maxAge {
		return fmt.Errorf("oracle data too stale: %v > %v", age, maxAge)
	}
	return nil
}

// ValidateMinReceived checks if the output meets minimum requirements (slippage protection)
func ValidateMinReceived(actualOutput, minReceived decimal.Decimal, operation string) error {
	if actualOutput.LessThan(minReceived) {
		return fmt.Errorf("%s output %s less than minimum required %s",
			operation, actualOutput.String(), minReceived.String())
	}
	return nil
}

// ValidateAmount checks if an amount is positive and within reasonable bounds
func ValidateAmount(amount decimal.Decimal, operation string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("invalid %s amount: must be positive", operation)
	}

	// Check for reasonable upper bounds (prevent overflow issues)
	maxAmount := decimal.New(1, 30) // 10^30
	if amount.GreaterThan(maxAmount) {
		return fmt.Errorf("invalid %s amount: too large", operation)
	}

	return nil
}

// ValidateQuote checks if a quote is still valid (not expired)
func ValidateQuote(quoteTimestamp time.Time, ttlSeconds int) error {
	expirationTime := quoteTimestamp.Add(time.Duration(ttlSeconds) * time.Second)
	if time.Now().After(expirationTime) {
		return fmt.Errorf("quote expired at %v", expirationTime)
	}
	return nil
}

// ValidateProtocolState performs basic sanity checks on protocol state
func ValidateProtocolState(reserves, supply, cr decimal.Decimal) error {
	if reserves.LessThan(decimal.Zero) {
		return fmt.Errorf("invalid reserves: cannot be negative")
	}

	if supply.LessThan(decimal.Zero) {
		return fmt.Errorf("invalid supply: cannot be negative")
	}

	if cr.LessThan(decimal.Zero) {
		return fmt.Errorf("invalid collateral ratio: cannot be negative")
	}

	// // Check if CR matches reserves/supply calculation
	// if !supply.IsZero() {
	// 	expectedCR := reserves.Div(supply)
	// 	tolerance := decimal.NewFromFloat(0.0001) // 0.01% tolerance
	// 	if expectedCR.Sub(cr).Abs().GreaterThan(tolerance) {
	// 		return fmt.Errorf("CR mismatch: expected %s, got %s", expectedCR, cr)
	// 	}
	// }

	return nil
}

// ValidateRebalanceParams checks rebalance operation parameters
func ValidateRebalanceParams(fBurn, payoutR, supplyF, reservesR decimal.Decimal) error {
	if fBurn.LessThan(decimal.Zero) || payoutR.LessThan(decimal.Zero) {
		return fmt.Errorf("rebalance amounts cannot be negative")
	}

	if fBurn.GreaterThan(supplyF) {
		return fmt.Errorf("cannot burn more fTokens than total supply")
	}

	if payoutR.GreaterThan(reservesR) {
		return fmt.Errorf("cannot payout more reserves than available")
	}

	return nil
}
