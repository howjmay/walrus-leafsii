import { Card } from '@/components/ui/Card'
import { SwapTab } from './SwapTab'
import { MintRedeemTab } from './MintRedeemTab'
import { StakeTab } from './StakeTab'
import { LimitTab } from './LimitTab'
import { TWAPTab } from './TWAPTab'
import { featureFlags } from '@/lib/featureFlags'
import type { Tab } from '@/lib/urls'

interface TradePanelProps {
  activeTab: Tab
  onTabChange: (tab: Tab) => void
  defaultSlippage: number
  demoMode?: boolean
  suiNetwork?: 'mainnet' | 'testnet' | 'localnet' | null
  collateralMode?: 'sui' | 'eth'
  crossChainAsset?: string | null
  crossChainChainId?: string | null
  onCrossChainMint?: () => void
}

const tabs = [
  { value: 'mint', label: 'Mint/Redeem', enabled: true },
  { value: 'swap', label: 'Swap', enabled: true },
  { value: 'stake', label: 'Stake', enabled: true },
  { value: 'limit', label: 'Limit', enabled: featureFlags.limit },
  { value: 'twap', label: 'TWAP', enabled: featureFlags.twap },
] as const

export function TradePanel({
  activeTab,
  onTabChange,
  defaultSlippage,
  demoMode = false,
  suiNetwork = null,
  collateralMode = 'sui',
  crossChainAsset,
  crossChainChainId,
  onCrossChainMint
}: TradePanelProps) {
  const enabledTabs = tabs.filter(tab => tab.enabled)

  return (
    <Card className="p-0 overflow-hidden">
      {/* Tab Navigation */}
      <div className="flex border-b border-border-subtle">
        {enabledTabs.map((tab) => (
          <button
            key={tab.value}
            onClick={() => onTabChange(tab.value)}
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

      {/* Tab Content */}
      <div className="p-6">
        {activeTab === 'swap' && (
          <SwapTab defaultSlippage={defaultSlippage} demoMode={demoMode} />
        )}
        {activeTab === 'mint' && (
          <MintRedeemTab
            demoMode={demoMode}
            collateralMode={collateralMode}
            crossChainAsset={crossChainAsset}
            crossChainChainId={crossChainChainId}
            suiNetwork={suiNetwork || undefined}
            onCrossChainMint={onCrossChainMint}
          />
        )}
        {activeTab === 'stake' && (
          <StakeTab demoMode={demoMode} />
        )}
        {activeTab === 'limit' && featureFlags.limit && (
          <LimitTab demoMode={demoMode} />
        )}
        {activeTab === 'twap' && featureFlags.twap && (
          <TWAPTab demoMode={demoMode} />
        )}
      </div>
    </Card>
  )
}
