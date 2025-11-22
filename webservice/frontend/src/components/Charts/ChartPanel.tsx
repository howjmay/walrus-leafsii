import { useEffect, useState } from 'react'
import { Card } from '@/components/ui/Card'
import { PriceChart, type Pair } from './PriceChart'
import { ProtocolMetrics } from './ProtocolMetrics'

type ChartTab = 'price' | 'protocol'
type CollateralMode = 'sui' | 'eth'

const tabs = [
  { value: 'price', label: 'Price' },
  { value: 'protocol', label: 'Protocol Metrics' },
] as const

interface ChartPanelProps {
  collateralMode?: CollateralMode
  onCollateralModeChange?: (mode: CollateralMode) => void
  isCrossChain?: boolean
}

export function ChartPanel({
  collateralMode = 'sui',
  onCollateralModeChange,
  isCrossChain = false
}: ChartPanelProps) {
  const [activeTab, setActiveTab] = useState<ChartTab>('price')
  const [selectedPair, setSelectedPair] = useState<Pair>(collateralMode === 'eth' ? 'ETH/USD' : 'SUI/USD')

  useEffect(() => {
    if (collateralMode === 'eth' && selectedPair !== 'ETH/USD') {
      setSelectedPair('ETH/USD')
    } else if (collateralMode === 'sui' && selectedPair !== 'SUI/USD') {
      setSelectedPair('SUI/USD')
    }
  }, [collateralMode, selectedPair])

  const handleCollateralChange = (mode: CollateralMode) => {
    setSelectedPair(mode === 'eth' ? 'ETH/USD' : 'SUI/USD')
    onCollateralModeChange?.(mode)
  }

  const handlePairChange = (pair: Pair) => {
    setSelectedPair(pair)
    if (pair === 'ETH/USD') {
      onCollateralModeChange?.('eth')
    } else {
      onCollateralModeChange?.('sui')
    }
  }

  return (
    <Card className="h-[500px] p-0 overflow-hidden">
      {/* Tab Bar */}
      <div className="flex items-center justify-between gap-3 border-b border-border-subtle px-3">
        <div className="flex flex-1">
          {tabs.map((tab) => (
            <button
              key={tab.value}
              onClick={() => setActiveTab(tab.value)}
              className={`flex-1 px-4 py-3 text-sm font-medium transition-colors ${
                activeTab === tab.value
                  ? 'text-brand-primary border-b-2 border-brand-primary bg-bg-card2/50'
                  : 'text-text-secondary hover:text-text-primary'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {isCrossChain && (
          <div className="flex items-center gap-1 bg-bg-card2 rounded-lg p-1">
            <button
              onClick={() => handleCollateralChange('sui')}
              className={`px-3 py-1.5 text-xs font-medium rounded-md ${
                collateralMode === 'sui'
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'text-text-secondary hover:text-text-primary'
              }`}
            >
              Sui Deposits
            </button>
            <button
              onClick={() => handleCollateralChange('eth')}
              className={`px-3 py-1.5 text-xs font-medium rounded-md ${
                collateralMode === 'eth'
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'text-text-secondary hover:text-text-primary'
              }`}
            >
              ETH Deposits
            </button>
          </div>
        )}
      </div>

      {/* Tab Content */}
      <div className="h-[calc(100%-49px)]">
        {activeTab === 'price' && (
          <PriceChart
            selectedPair={selectedPair}
            onPairChange={handlePairChange}
          />
        )}
        {activeTab === 'protocol' && <ProtocolMetrics />}
      </div>
    </Card>
  )
}
