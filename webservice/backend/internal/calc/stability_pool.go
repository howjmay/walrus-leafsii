package calc

import (
	"github.com/shopspring/decimal"
)

// CalculateNewSPIndex calculates the new stability pool index after rewards accrual
// indexNew = indexPrev + payout_r / totalStakeF
func CalculateNewSPIndex(previousIndex, payoutR, totalStakeF decimal.Decimal) decimal.Decimal {
	if totalStakeF.IsZero() {
		return previousIndex
	}
	
	indexDelta := payoutR.Div(totalStakeF)
	return previousIndex.Add(indexDelta)
}

// CalculateClaimableRewards calculates how much a user can claim from SP
func CalculateClaimableRewards(stakeF, indexAtJoin, currentIndex decimal.Decimal) decimal.Decimal {
	if stakeF.IsZero() || currentIndex.LessThanOrEqual(indexAtJoin) {
		return decimal.Zero
	}
	
	indexDelta := currentIndex.Sub(indexAtJoin)
	return stakeF.Mul(indexDelta)
}

// CalculateAPR estimates the stability pool APR based on recent rewards
func CalculateAPR(rewardsLast24h, tvlF decimal.Decimal) decimal.Decimal {
	if tvlF.IsZero() {
		return decimal.Zero
	}
	
	dailyReturn := rewardsLast24h.Div(tvlF)
	annualReturn := dailyReturn.Mul(decimal.NewFromInt(365))
	return annualReturn.Mul(decimal.NewFromInt(100)) // Convert to percentage
}

// CalculateStakePreview estimates the impact of staking fTokens
func CalculateStakePreview(stakeAmount, currentIndex, currentTVL decimal.Decimal) (newTVL, expectedIndexDelta decimal.Decimal) {
	newTVL = currentTVL.Add(stakeAmount)
	
	// Expected index delta is 0 for new stakes (they join at current index)
	expectedIndexDelta = decimal.Zero
	
	return newTVL, expectedIndexDelta
}

// CalculateUnstakeOutput calculates what a user gets when unstaking
func CalculateUnstakeOutput(unstakeAmount, indexAtJoin, currentIndex decimal.Decimal) (fTokens, rewards decimal.Decimal) {
	fTokens = unstakeAmount
	rewards = CalculateClaimableRewards(unstakeAmount, indexAtJoin, currentIndex)
	return fTokens, rewards
}

// SimulateRewardsDistribution simulates the effect of distributing rewards to SP
func SimulateRewardsDistribution(rewardAmount, totalStakeF, currentIndex decimal.Decimal) decimal.Decimal {
	return CalculateNewSPIndex(currentIndex, rewardAmount, totalStakeF)
}