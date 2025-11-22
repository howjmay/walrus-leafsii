package calc

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// CollateralRatio calculates CR = reserves_r / liability_r
// Where liability_r is the total value that needs to be backed by reserves
// For simplicity, assuming liability = supply_f (fToken supply at peg 1.0)
func CollateralRatio(reservesR, supplyF decimal.Decimal) decimal.Decimal {
	if supplyF.IsZero() {
		return decimal.Zero
	}
	return reservesR.Div(supplyF)
}

// PegDeviation calculates |oracle_price(fToken) - 1.0|
func PegDeviation(fTokenPrice decimal.Decimal) decimal.Decimal {
	target := decimal.NewFromInt(1)
	deviation := fTokenPrice.Sub(target).Abs()
	return deviation
}

// PostMintCR calculates the collateral ratio after a mint operation
func PostMintCR(currentReservesR, currentSupplyF, mintAmountR decimal.Decimal) decimal.Decimal {
	newReservesR := currentReservesR.Add(mintAmountR)

	// Assuming mint gives 1:1 fTokens for Sui (minus fees)
	// This would be more complex in reality with dynamic pricing
	newSupplyF := currentSupplyF.Add(mintAmountR)

	return CollateralRatio(newReservesR, newSupplyF)
}

// PostRedeemCR calculates the collateral ratio after a redeem operation
func PostRedeemCR(currentReservesR, currentSupplyF, redeemAmountF decimal.Decimal) decimal.Decimal {
	if redeemAmountF.GreaterThan(currentSupplyF) {
		return decimal.Zero // Invalid operation
	}

	// Redeeming fTokens for Sui reduces both supply and reserves
	newSupplyF := currentSupplyF.Sub(redeemAmountF)
	newReservesR := currentReservesR.Sub(redeemAmountF) // Simplified 1:1 redemption

	if newSupplyF.IsZero() {
		return decimal.Zero
	}

	return CollateralRatio(newReservesR, newSupplyF)
}

// CalculateMintOutput calculates how many fTokens a user gets for a given Sui input
func CalculateMintOutput(amountR, feeRate decimal.Decimal) (fOut, fee decimal.Decimal) {
	fee = amountR.Mul(feeRate)
	fOut = amountR.Sub(fee)
	return fOut, fee
}

// CalculateRedeemOutput calculates how many Sui a user gets for redeeming fTokens
func CalculateRedeemOutput(amountF, feeRate decimal.Decimal) (rOut, fee decimal.Decimal) {
	fee = amountF.Mul(feeRate)
	rOut = amountF.Sub(fee)
	return rOut, fee
}

// ValidateCRConstraint checks if an operation would violate minimum CR requirements
func ValidateCRConstraint(postCR, minCR decimal.Decimal) error {
	if postCR.LessThan(minCR) {
		return fmt.Errorf("operation would breach minimum collateral ratio: %s < %s",
			postCR, minCR)
	}
	return nil
}

// IsRebalanceNeeded checks if the protocol needs rebalancing based on CR deviation
func IsRebalanceNeeded(currentCR, targetCR, tolerance decimal.Decimal) bool {
	deviation := currentCR.Sub(targetCR).Abs()
	maxDeviation := targetCR.Mul(tolerance)
	return deviation.GreaterThan(maxDeviation)
}

// CalculateRebalanceAmounts calculates f_burn and payout_r for rebalancing
func CalculateRebalanceAmounts(currentCR, targetCR, supplyF, reservesR decimal.Decimal) (fBurn, payoutR decimal.Decimal) {
	if currentCR.LessThanOrEqual(targetCR) {
		return decimal.Zero, decimal.Zero
	}

	// Simplified rebalance calculation
	// In reality, this would be more sophisticated based on protocol mechanics
	excessCR := currentCR.Sub(targetCR)
	excessValue := excessCR.Mul(supplyF)

	// Burn a portion of fTokens and payout reserves to SP
	fBurn = excessValue.Div(decimal.NewFromInt(2))
	payoutR = fBurn

	// Ensure we don't exceed available supplies
	if fBurn.GreaterThan(supplyF) {
		fBurn = supplyF
	}
	if payoutR.GreaterThan(reservesR) {
		payoutR = reservesR
	}

	return fBurn, payoutR
}
