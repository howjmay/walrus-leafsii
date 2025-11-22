import { useState, useEffect } from 'react'
import { ArrowDownUp, Settings, AlertTriangle } from 'lucide-react'
import { Input } from '@/components/ui/Input'
import { Button } from '@/components/ui/Button'
import { TokenSelector } from './TokenSelector'
import { QuoteSummary } from './QuoteSummary'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { toast } from 'sonner'
import { formatNumber } from '@/lib/utils'

interface SwapTabProps {
  defaultSlippage: number
  demoMode?: boolean
}

export function SwapTab({ defaultSlippage, demoMode = false }: SwapTabProps) {
  const currentAccount = useCurrentAccount()
  const [fromToken, setFromToken] = useState('fToken')
  const [toToken, setToToken] = useState('Sui')
  const [fromAmount, setFromAmount] = useState('')
  const [toAmount, setToAmount] = useState('')
  const [slippage, setSlippage] = useState(defaultSlippage)
  const [isAutoSlippage, setIsAutoSlippage] = useState(true)
  const [isLoading, setIsLoading] = useState(false)
  const [showSettings, setShowSettings] = useState(false)
  const [isWalletConnected, setIsWalletConnected] = useState(false)

  // Mock quote calculation
  useEffect(() => {
    if (!fromAmount || isNaN(Number(fromAmount))) {
      setToAmount('')
      return
    }

    const timeoutId = setTimeout(() => {
      // Mock exchange rate with some variation
      const rate = 0.998 + Math.random() * 0.004 // 0.998 - 1.002
      setToAmount((Number(fromAmount) * rate).toFixed(6))
    }, 300)

    return () => clearTimeout(timeoutId)
  }, [fromAmount, fromToken, toToken])

  // Check if wallet is connected (demo mode check)
  useEffect(() => {
    if (demoMode) {
      const checkWallet = () => {
        const isConnected = localStorage.getItem('demoWalletConnected') === 'true'
        setIsWalletConnected(isConnected)
      }

      checkWallet()
      window.addEventListener('storage', checkWallet)
      return () => window.removeEventListener('storage', checkWallet)
    }
  }, [demoMode])

  const handleSwapTokens = () => {
    setFromToken(toToken)
    setToToken(fromToken)
    setFromAmount(toAmount)
    setToAmount(fromAmount)
  }

  const priceImpact = 0.02 // Mock 0.02%
  const isHighImpact = priceImpact > 1
  const minReceived = toAmount ? (Number(toAmount) * (1 - slippage / 100)).toFixed(6) : '0'
  
  const canSwap = (demoMode ? isWalletConnected : currentAccount) && fromAmount && toAmount && !isLoading

  const handleSwap = async () => {
    if (!canSwap) return

    setIsLoading(true)
    try {
      await new Promise(resolve => setTimeout(resolve, 2000))
      if (demoMode) {
        toast.success(`Demo: Swapped ${fromAmount} ${fromToken} for ${toAmount} ${toToken}`)
        setFromAmount('')
        setToAmount('')
      } else {
        console.log('Swap executed:', { fromToken, toToken, fromAmount, toAmount })
      }
    } finally {
      setIsLoading(false)
    }
  }

  const handleConnectWallet = () => {
    if (demoMode) {
      localStorage.setItem('demoWalletConnected', 'true')
      setIsWalletConnected(true)
      toast.success('Demo wallet connected!')
      window.dispatchEvent(new Event('storage'))
    }
  }

  return (
    <div className="space-y-4">
      {/* From Token */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-text-muted text-sm">From</label>
          <div className="text-text-muted text-sm">
            Balance: {formatNumber(1234.56)}
          </div>
        </div>
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Input
              type="number"
              placeholder="0.0"
              value={fromAmount}
              onChange={(e) => setFromAmount(e.target.value)}
              className="pr-20"
              aria-label={`Amount (${fromToken})`}
            />
            <div className="absolute right-0 top-0 h-full flex items-center">
              <div className="px-3 text-text-muted text-sm border-l border-border-weak">
                {fromToken}
              </div>
            </div>
          </div>
          <TokenSelector
            selectedToken={fromToken}
            onTokenChange={setFromToken}
            excludeToken={toToken}
          />
        </div>
      </div>

      {/* Swap Direction Button */}
      <div className="flex justify-center">
        <button
          onClick={handleSwapTokens}
          className="w-10 h-10 bg-bg-card2 border border-border-subtle rounded-xl flex items-center justify-center hover:bg-bg-input transition-colors"
        >
          <ArrowDownUp className="w-4 h-4 text-text-primary" />
        </button>
      </div>

      {/* To Token */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-text-muted text-sm">To</label>
          <div className="text-text-muted text-sm">
            Balance: {formatNumber(987.65)}
          </div>
        </div>
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Input
              type="number"
              placeholder="0.0"
              value={toAmount}
              readOnly
              className="pr-20 bg-bg-card2/50"
              aria-label={`Amount (${toToken})`}
            />
            <div className="absolute right-0 top-0 h-full flex items-center">
              <div className="px-3 text-text-muted text-sm border-l border-border-weak">
                {toToken}
              </div>
            </div>
          </div>
          <TokenSelector
            selectedToken={toToken}
            onTokenChange={setToToken}
            excludeToken={fromToken}
          />
        </div>
      </div>

      {/* Quote Summary */}
      {fromAmount && toAmount && (
        <QuoteSummary
          rate={`1 ${fromToken} = ${(Number(toAmount) / Number(fromAmount)).toFixed(6)} ${toToken}`}
          minReceived={`${minReceived} ${toToken}`}
          slippage={`${slippage}%`}
          priceImpact={`${priceImpact.toFixed(3)}%`}
          fee="0.3%"
          route={`${fromToken} → ${toToken} via Pool A`}
        />
      )}

      {/* High Price Impact Warning */}
      {isHighImpact && (
        <div className="bg-warn/10 border border-warn/20 rounded-xl p-3">
          <div className="flex items-center gap-2 text-warn">
            <AlertTriangle className="w-4 h-4" />
            <span className="font-medium">High Price Impact</span>
          </div>
          <div className="text-text-muted text-sm mt-1">
            This trade will significantly affect the token price. Consider reducing the amount.
          </div>
        </div>
      )}

      {/* Slippage Settings */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-text-muted text-sm">Slippage Tolerance</span>
          <button
            onClick={() => setShowSettings(!showSettings)}
            className="text-text-muted hover:text-text-primary transition-colors"
          >
            <Settings className="w-4 h-4" />
          </button>
        </div>
        <div className="text-text-primary text-sm">{slippage}%</div>
      </div>

      {showSettings && (
        <div className="bg-bg-card2/30 rounded-xl p-4 space-y-3">
          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="auto-slippage"
              checked={isAutoSlippage}
              onChange={(e) => setIsAutoSlippage(e.target.checked)}
              className="w-4 h-4 text-brand-primary"
            />
            <label htmlFor="auto-slippage" className="text-text-primary text-sm">
              Auto slippage
            </label>
          </div>
          {!isAutoSlippage && (
            <div className="flex gap-2">
              {[0.1, 0.5, 1.0, 2.0].map((value) => (
                <button
                  key={value}
                  onClick={() => setSlippage(value)}
                  className={`flex-1 px-3 py-2 rounded-lg text-sm transition-colors ${
                    slippage === value
                      ? 'bg-brand-primary text-text-onBrand'
                      : 'bg-bg-input text-text-primary hover:bg-bg-card2'
                  }`}
                >
                  {value}%
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Swap Button */}
      {!(demoMode ? isWalletConnected : currentAccount) ? (
        <Button onClick={demoMode ? handleConnectWallet : undefined} className="w-full" disabled={!demoMode}>
          {demoMode ? 'Connect Demo Wallet to Swap' : 'Connect Wallet to Swap'}
        </Button>
      ) : (
        <Button
          onClick={handleSwap}
          disabled={!canSwap}
          className="w-full"
        >
          {isLoading ? 'Swapping...' : `Swap ${fromToken} for ${toToken}`}
        </Button>
      )}
    </div>
  )
}