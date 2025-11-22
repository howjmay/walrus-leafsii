import { useState } from 'react'
import { TrendingUp, Gift } from 'lucide-react'
import { Input } from '@/components/ui/Input'
import { Button } from '@/components/ui/Button'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { formatNumber, formatPercentage } from '@/lib/utils'
type StakeAction = 'stake' | 'unstake'

interface StakeTabProps {
  demoMode?: boolean
}

export function StakeTab({ demoMode: _demoMode = false }: StakeTabProps) {
  const currentAccount = useCurrentAccount()
  const [action, setAction] = useState<StakeAction>('stake')
  const [amount, setAmount] = useState('')
  const [isLoading, setIsLoading] = useState(false)

  // Mock data - would come from hooks in real app
  const stakingData = {
    userStake: 850.0,
    claimableRewards: 12.45,
    currentIndex: 1.0298,
    indexAtJoin: 1.0234,
    fTokenBalance: 1250.45,
    historicalAPR: 0.085, // 8.5%
    poolTVL: 5600000,
  }

  const indexGain = ((stakingData.currentIndex / stakingData.indexAtJoin - 1) * 100)
  const canExecute = currentAccount && amount && !isLoading
  const canClaim = currentAccount && stakingData.claimableRewards > 0

  const handleStakeAction = async () => {
    if (!canExecute) return
    
    setIsLoading(true)
    try {
      await new Promise(resolve => setTimeout(resolve, 2000))
      console.log(`${action} executed:`, { amount })
    } finally {
      setIsLoading(false)
    }
  }

  const handleClaim = async () => {
    if (!canClaim) return
    
    setIsLoading(true)
    try {
      await new Promise(resolve => setTimeout(resolve, 1500))
      console.log('Rewards claimed:', { amount: stakingData.claimableRewards })
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      {/* Your Staking Position */}
      <div className="card--elevated p-4">
        <div className="flex items-center gap-2 mb-4">
          <TrendingUp className="w-5 h-5 text-brand-primary" />
          <h3 className="text-text-primary font-semibold">Your Stake</h3>
        </div>
        
        <div className="grid grid-cols-2 gap-4 mb-4">
          <div>
            <div className="text-text-muted text-sm mb-1">Staked Amount</div>
            <div className="text-2xl font-bold text-text-primary">
              {formatNumber(stakingData.userStake)}
              <span className="text-lg text-text-muted ml-1">fToken</span>
            </div>
          </div>
          <div>
            <div className="text-text-muted text-sm mb-1">Claimable Rewards</div>
            <div className="text-2xl font-bold text-success">
              {formatNumber(stakingData.claimableRewards)}
              <span className="text-lg text-text-muted ml-1">Sui</span>
            </div>
          </div>
        </div>

        <div className="bg-bg-input rounded-lg p-3 mb-4">
          <div className="flex items-center justify-between text-sm">
            <span className="text-text-muted">Index Gain</span>
            <span className="text-success font-medium">+{indexGain.toFixed(3)}%</span>
          </div>
          <div className="flex items-center justify-between text-xs mt-1">
            <span className="text-text-muted">
              Index: {stakingData.indexAtJoin.toFixed(4)} → {stakingData.currentIndex.toFixed(4)}
            </span>
          </div>
        </div>

        {/* Claim Button */}
        <Button
          onClick={handleClaim}
          disabled={!canClaim || isLoading}
          variant="secondary"
          className="w-full mb-3"
        >
          <Gift className="w-4 h-4 mr-2" />
          {isLoading ? 'Claiming...' : `Claim ${formatNumber(stakingData.claimableRewards)} Sui`}
        </Button>

        {/* Mini Chart Placeholder */}
        <div className="h-14 bg-bg-input rounded-lg flex items-center justify-center">
          <div className="text-text-muted text-xs">Index History (7d)</div>
        </div>
      </div>

      {/* Stake/Unstake Actions */}
      <div className="space-y-4">
        {/* Action Selector */}
        <div className="grid grid-cols-2 gap-2 p-1 bg-bg-input rounded-xl">
          <button
            onClick={() => setAction('stake')}
            className={`px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
              action === 'stake'
                ? 'bg-brand-primary text-text-onBrand'
                : 'text-text-secondary hover:text-text-primary'
            }`}
          >
            Stake
          </button>
          <button
            onClick={() => setAction('unstake')}
            className={`px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
              action === 'unstake'
                ? 'bg-brand-primary text-text-onBrand'
                : 'text-text-secondary hover:text-text-primary'
            }`}
          >
            Unstake
          </button>
        </div>

        {/* Amount Input */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="text-text-muted text-sm">Amount</label>
            <div className="text-text-muted text-sm">
              {action === 'stake' 
                ? `Balance: ${formatNumber(stakingData.fTokenBalance)} fToken`
                : `Staked: ${formatNumber(stakingData.userStake)} fToken`
              }
            </div>
          </div>
          <Input
            type="number"
            placeholder="0.0"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
          />
        </div>

        {/* Info Box */}
        <div className="bg-bg-card2/30 rounded-xl p-4">
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <div className="text-text-muted">Historical APR</div>
              <div className="text-text-primary font-semibold">
                {formatPercentage(stakingData.historicalAPR)}
              </div>
            </div>
            <div>
              <div className="text-text-muted">Pool TVL</div>
              <div className="text-text-primary font-semibold">
                ${formatNumber(stakingData.poolTVL)}
              </div>
            </div>
          </div>
          <div className="text-text-muted text-xs mt-3">
            Rewards are accrued through index appreciation and distributed via deferred payout model.
            Claim rewards when index snapshots are available.
          </div>
        </div>

        {/* Execute Button */}
        {!currentAccount ? (
          <Button className="w-full" disabled>
            Connect Wallet to Stake
          </Button>
        ) : (
          <Button
            onClick={handleStakeAction}
            disabled={!canExecute}
            className="w-full"
          >
            {isLoading 
              ? `${action === 'stake' ? 'Staking' : 'Unstaking'}...`
              : `${action === 'stake' ? 'Stake' : 'Unstake'} fToken`
            }
          </Button>
        )}
      </div>
    </div>
  )
}