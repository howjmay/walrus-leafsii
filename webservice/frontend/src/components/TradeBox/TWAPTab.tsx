import { useState } from 'react'
import { TrendingUp, AlertCircle, Clock } from 'lucide-react'
import { Input } from '@/components/ui/Input'
import { Button } from '@/components/ui/Button'
import { TokenSelector } from './TokenSelector'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { formatNumber } from '@/lib/utils'

interface TWAPTabProps {
  demoMode?: boolean
}

export function TWAPTab({ demoMode: _demoMode = false }: TWAPTabProps) {
  const currentAccount = useCurrentAccount()
  const [fromToken, setFromToken] = useState('fToken')
  const [toToken, setToToken] = useState('Sui')
  const [totalSize, setTotalSize] = useState('')
  const [chunkSize, setChunkSize] = useState('')
  const [interval, setInterval] = useState('600') // 10 minutes
  const [maxDuration, setMaxDuration] = useState('86400') // 24 hours
  const [isLoading, setIsLoading] = useState(false)

  const intervalOptions = [
    { value: '300', label: '5 minutes' },
    { value: '600', label: '10 minutes' },
    { value: '1800', label: '30 minutes' },
    { value: '3600', label: '1 hour' },
  ]

  const durationOptions = [
    { value: '3600', label: '1 hour' },
    { value: '21600', label: '6 hours' },
    { value: '43200', label: '12 hours' },
    { value: '86400', label: '24 hours' },
    { value: '259200', label: '3 days' },
  ]

  const estimatedChunks = totalSize && chunkSize ? Math.ceil(Number(totalSize) / Number(chunkSize)) : 0
  const estimatedDuration = estimatedChunks * Number(interval) / 60 // in minutes
  const wouldExceedDuration = estimatedDuration * 60 > Number(maxDuration)

  const canCreateTWAP = currentAccount && totalSize && chunkSize && !wouldExceedDuration && !isLoading

  const handleCreateTWAP = async () => {
    if (!canCreateTWAP) return
    
    setIsLoading(true)
    try {
      await new Promise(resolve => setTimeout(resolve, 2000))
      console.log('TWAP order created:', { fromToken, toToken, totalSize, chunkSize, interval, maxDuration })
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="space-y-4">
      {/* Feature Notice */}
      <div className="bg-info/10 border border-info/20 rounded-xl p-3">
        <div className="flex items-center gap-2 text-info">
          <TrendingUp className="w-4 h-4" />
          <span className="font-medium">Time-Weighted Average Price (TWAP)</span>
        </div>
        <div className="text-text-muted text-sm mt-1">
          Execute large orders over time to minimize price impact and achieve better average pricing.
        </div>
      </div>

      {/* Token Pair */}
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="text-text-muted text-sm mb-2 block">Sell</label>
          <div className="relative">
            <TokenSelector
              selectedToken={fromToken}
              onTokenChange={setFromToken}
              excludeToken={toToken}
            />
          </div>
        </div>
        <div>
          <label className="text-text-muted text-sm mb-2 block">Buy</label>
          <div className="relative">
            <TokenSelector
              selectedToken={toToken}
              onTokenChange={setToToken}
              excludeToken={fromToken}
            />
          </div>
        </div>
      </div>

      {/* Total Size */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-text-muted text-sm">Total Size</label>
          <div className="text-text-muted text-sm">
            Balance: {formatNumber(1234.56)} {fromToken}
          </div>
        </div>
        <Input
          type="number"
          placeholder="0.0"
          value={totalSize}
          onChange={(e) => setTotalSize(e.target.value)}
        />
      </div>

      {/* Chunk Size */}
      <div>
        <label className="text-text-muted text-sm mb-2 block">
          Chunk Size (per trade)
        </label>
        <Input
          type="number"
          placeholder="0.0"
          value={chunkSize}
          onChange={(e) => setChunkSize(e.target.value)}
        />
        <div className="text-xs text-text-muted mt-1">
          Smaller chunks reduce price impact but increase execution time
        </div>
      </div>

      {/* Interval */}
      <div>
        <label className="text-text-muted text-sm mb-2 block">
          Interval Between Trades
        </label>
        <div className="grid grid-cols-2 gap-2">
          {intervalOptions.map((option) => (
            <button
              key={option.value}
              onClick={() => setInterval(option.value)}
              className={`px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                interval === option.value
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'bg-bg-input text-text-primary hover:bg-bg-card2'
              }`}
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>

      {/* Max Duration */}
      <div>
        <label className="text-text-muted text-sm mb-2 block">
          Maximum Duration
        </label>
        <div className="grid grid-cols-2 lg:grid-cols-3 gap-2">
          {durationOptions.map((option) => (
            <button
              key={option.value}
              onClick={() => setMaxDuration(option.value)}
              className={`px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                maxDuration === option.value
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'bg-bg-input text-text-primary hover:bg-bg-card2'
              }`}
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>

      {/* Execution Summary */}
      {totalSize && chunkSize && (
        <div className="bg-bg-card2/30 rounded-xl p-4 space-y-3">
          <h4 className="text-text-primary font-medium">Execution Summary</h4>
          
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <div className="text-text-muted">Total Chunks</div>
              <div className="text-text-primary font-semibold">{estimatedChunks}</div>
            </div>
            <div>
              <div className="text-text-muted">Est. Duration</div>
              <div className="text-text-primary font-semibold">
                {estimatedDuration < 60 
                  ? `${Math.ceil(estimatedDuration)}m`
                  : `${(estimatedDuration / 60).toFixed(1)}h`
                }
              </div>
            </div>
          </div>

          <div className="text-xs text-text-muted">
            • Average chunk size: {chunkSize} {fromToken}
            • Execution interval: {intervalOptions.find(i => i.value === interval)?.label}
            • Can be cancelled anytime with remaining amount returned
          </div>

          {wouldExceedDuration && (
            <div className="bg-warn/10 border border-warn/20 rounded-lg p-3">
              <div className="flex items-center gap-2 text-warn">
                <AlertCircle className="w-4 h-4" />
                <span className="font-medium">Duration Warning</span>
              </div>
              <div className="text-text-muted text-xs mt-1">
                Estimated execution time exceeds maximum duration. Reduce chunk size or increase max duration.
              </div>
            </div>
          )}
        </div>
      )}

      {/* Cancel Remaining Toggle */}
      <div className="flex items-center justify-between p-3 bg-bg-card2/30 rounded-xl">
        <div>
          <div className="text-text-primary text-sm font-medium">Cancel Remaining on Fill</div>
          <div className="text-text-muted text-xs">
            Cancel unfilled chunks when total target is reached
          </div>
        </div>
        <button
          className="relative inline-flex h-6 w-11 items-center rounded-full bg-brand-primary transition-colors"
        >
          <span className="inline-block h-4 w-4 transform rounded-full bg-white translate-x-6 transition-transform" />
        </button>
      </div>

      {/* Create TWAP Button */}
      {!currentAccount ? (
        <Button className="w-full" disabled>
          Connect Wallet to Create TWAP Order
        </Button>
      ) : (
        <Button
          onClick={handleCreateTWAP}
          disabled={!canCreateTWAP}
          className="w-full"
        >
          <Clock className="w-4 h-4 mr-2" />
          {isLoading ? 'Creating TWAP Order...' : 'Create TWAP Order'}
        </Button>
      )}
    </div>
  )
}