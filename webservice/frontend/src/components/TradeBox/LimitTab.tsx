import { useState } from 'react'
import { Clock, AlertCircle } from 'lucide-react'
import { Input } from '@/components/ui/Input'
import { Button } from '@/components/ui/Button'
import { TokenSelector } from './TokenSelector'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { formatNumber } from '@/lib/utils'
type Expiry = 'GTC' | '7d' | '1d' | '1h'

interface LimitTabProps {
  demoMode?: boolean
}

export function LimitTab({ demoMode: _demoMode = false }: LimitTabProps) {
  const currentAccount = useCurrentAccount()
  const [fromToken, setFromToken] = useState('fToken')
  const [toToken, setToToken] = useState('Sui')
  const [sellAmount, setSellAmount] = useState('')
  const [limitPrice, setLimitPrice] = useState('')
  const [expiry, setExpiry] = useState<Expiry>('GTC')
  const [allowPartialFill, setAllowPartialFill] = useState(true)
  const [isLoading, setIsLoading] = useState(false)

  const currentPrice = 0.998
  const priceAboveMarket = limitPrice ? (Number(limitPrice) - currentPrice) / currentPrice * 100 : 0
  const buyAmount = sellAmount && limitPrice ? (Number(sellAmount) * Number(limitPrice)).toFixed(6) : ''

  const expiryOptions = [
    { value: 'GTC', label: 'Good Till Cancelled', description: 'Never expires' },
    { value: '7d', label: '7 Days', description: 'Expires in 7 days' },
    { value: '1d', label: '1 Day', description: 'Expires in 1 day' },
    { value: '1h', label: '1 Hour', description: 'Expires in 1 hour' },
  ] as const

  const canCreateOrder = currentAccount && sellAmount && limitPrice && !isLoading

  const handleCreateOrder = async () => {
    if (!canCreateOrder) return
    
    setIsLoading(true)
    try {
      await new Promise(resolve => setTimeout(resolve, 2000))
      console.log('Limit order created:', { fromToken, toToken, sellAmount, limitPrice, expiry })
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="space-y-4">
      {/* Feature Notice */}
      <div className="bg-info/10 border border-info/20 rounded-xl p-3">
        <div className="flex items-center gap-2 text-info">
          <AlertCircle className="w-4 h-4" />
          <span className="font-medium">Limit Orders</span>
        </div>
        <div className="text-text-muted text-sm mt-1">
          Set your desired price and let the order execute when conditions are met.
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

      {/* Sell Amount */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-text-muted text-sm">Sell Amount</label>
          <div className="text-text-muted text-sm">
            Balance: {formatNumber(1234.56)} {fromToken}
          </div>
        </div>
        <Input
          type="number"
          placeholder="0.0"
          value={sellAmount}
          onChange={(e) => setSellAmount(e.target.value)}
        />
      </div>

      {/* Limit Price */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-text-muted text-sm">
            Limit Price (1 {fromToken} = ? {toToken})
          </label>
          <div className="text-text-muted text-sm">
            Market: {currentPrice.toFixed(4)}
          </div>
        </div>
        <Input
          type="number"
          placeholder="0.0"
          value={limitPrice}
          onChange={(e) => setLimitPrice(e.target.value)}
        />
        {limitPrice && (
          <div className={`text-xs mt-1 ${
            priceAboveMarket > 0 ? 'text-success' : priceAboveMarket < 0 ? 'text-danger' : 'text-text-muted'
          }`}>
            {priceAboveMarket > 0 ? '+' : ''}{priceAboveMarket.toFixed(2)}% vs market price
          </div>
        )}
      </div>

      {/* Buy Amount (calculated) */}
      <div>
        <label className="text-text-muted text-sm mb-2 block">You'll Receive</label>
        <Input
          type="number"
          placeholder="0.0"
          value={buyAmount}
          readOnly
          className="bg-bg-card2/50"
        />
      </div>

      {/* Expiry */}
      <div>
        <label className="text-text-muted text-sm mb-2 block">Expiry</label>
        <div className="grid grid-cols-2 gap-2">
          {expiryOptions.map((option) => (
            <button
              key={option.value}
              onClick={() => setExpiry(option.value)}
              className={`p-3 rounded-xl text-left transition-colors ${
                expiry === option.value
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'bg-bg-input text-text-primary hover:bg-bg-card2'
              }`}
            >
              <div className="font-medium text-sm">{option.label}</div>
              <div className={`text-xs mt-1 ${
                expiry === option.value ? 'text-text-onBrand/70' : 'text-text-muted'
              }`}>
                {option.description}
              </div>
            </button>
          ))}
        </div>
      </div>

      {/* Partial Fill Toggle */}
      <div className="flex items-center justify-between p-3 bg-bg-card2/30 rounded-xl">
        <div>
          <div className="text-text-primary text-sm font-medium">Allow Partial Fill</div>
          <div className="text-text-muted text-xs">
            Order can be filled in multiple smaller transactions
          </div>
        </div>
        <button
          onClick={() => setAllowPartialFill(!allowPartialFill)}
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
            allowPartialFill ? 'bg-brand-primary' : 'bg-bg-input'
          }`}
        >
          <span
            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
              allowPartialFill ? 'translate-x-6' : 'translate-x-1'
            }`}
          />
        </button>
      </div>

      {/* Order Summary */}
      {sellAmount && limitPrice && (
        <div className="bg-bg-card2/30 rounded-xl p-4 space-y-2">
          <div className="flex justify-between text-sm">
            <span className="text-text-muted">Order Type</span>
            <span className="text-text-primary">Limit Order</span>
          </div>
          <div className="flex justify-between text-sm">
            <span className="text-text-muted">Estimated Fee</span>
            <span className="text-text-primary">0.1%</span>
          </div>
          <div className="flex justify-between text-sm">
            <span className="text-text-muted">Expiry</span>
            <div className="flex items-center gap-1 text-text-primary">
              <Clock className="w-3 h-3" />
              {expiryOptions.find(e => e.value === expiry)?.label}
            </div>
          </div>
        </div>
      )}

      {/* Create Order Button */}
      {!currentAccount ? (
        <Button className="w-full" disabled>
          Connect Wallet to Create Order
        </Button>
      ) : (
        <Button
          onClick={handleCreateOrder}
          disabled={!canCreateOrder}
          className="w-full"
        >
          {isLoading ? 'Creating Order...' : 'Create Limit Order'}
        </Button>
      )}
    </div>
  )
}