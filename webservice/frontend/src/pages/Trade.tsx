import { useState, useEffect, useContext } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Header } from '@/components/Header/Header'
import { ChartPanel } from '@/components/Charts/ChartPanel'
import { TradePanel } from '@/components/TradeBox/TradePanel'
import { ProtocolHealth } from '@/components/Widgets/ProtocolHealth'
import { RebalanceFeed } from '@/components/Widgets/RebalanceFeed'
import { Card } from '@/components/ui/Card'
import { MyPositions } from '@/components/Charts/MyPositions'
import { parseURLState, buildURL, type Tab } from '@/lib/urls'
import { CrossChainBalance, VoucherManager } from '@/components/CrossChain'
import type { Market } from '@/types/market'
import { apiUrl } from '@/utils/api'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { SuiClientContext } from '@mysten/dapp-kit'

const isSupportedNetwork = (n: string | null | undefined): n is 'mainnet' | 'testnet' | 'localnet' =>
  n === 'mainnet' || n === 'testnet' || n === 'localnet'

export default function TradePage() {
  const suiCtx = useContext(SuiClientContext)
  const [searchParams, setSearchParams] = useSearchParams()

  const [activeTab, setActiveTab] = useState<Tab>('mint')
  const [darkMode, setDarkMode] = useState(true)
  const [currency, setCurrency] = useState('USD')
  const [language, setLanguage] = useState('English')
  const [defaultSlippage, setDefaultSlippage] = useState(0.5)
  const [selectedMarket, setSelectedMarket] = useState<Market | null>(null)
  const [balanceRefresh, setBalanceRefresh] = useState(0)
  const [collateralMode, setCollateralMode] = useState<'sui' | 'eth'>('sui')

  useEffect(() => {
    const urlState = parseURLState(searchParams)
    if (urlState.tab) setActiveTab(urlState.tab)
  }, [searchParams])

  const updateURL = (updates: { tab?: Tab }) => {
    const currentState = parseURLState(searchParams)
    const newState = { ...currentState, ...updates }
    const url = buildURL(newState)
    setSearchParams(url, { replace: true })
  }

  const handleTabChange = (newTab: Tab) => {
    setActiveTab(newTab)
    updateURL({ tab: newTab })
  }

  const currentAccount = useCurrentAccount()
  const userAddress = currentAccount?.address
  const suiNetwork = isSupportedNetwork(suiCtx?.network) ? suiCtx?.network : null

  useEffect(() => {
    const loadMarkets = async () => {
      const fallbackMarket: Market = {
        id: 'crosschain-eth',
        label: 'Ethereum Cross-Chain Vault',
        pairSymbol: 'fETH/xETH',
        stableSymbol: 'fETH',
        leverageSymbol: 'xETH',
        collateralSymbol: 'ETH',
        collateralType: 'crosschain',
        collateralHighlights: [
          'Native ETH staked on Ethereum mainnet',
          'Verified via Walrus + zk light client proofs',
          'Self-custody withdrawals with signed vouchers',
          'Conservative 65% LTV, 6% liquidation penalty'
        ],
        px: 2850000000,
        cr: '1.38',
        targetCr: '1.38',
        reserves: '8500000',
        supplyStable: '6159420.29',
        supplyLeverage: '2340579.71',
        mode: 'crosschain',
        feedUrl: 'https://walrus.xyz/api/feeds/eth-vault',
        proofCid: 'bafyEthereumVaultProof',
        snapshotUrl: 'https://walrus.storage/eth/latest.json',
        chainId: 'ethereum',
        asset: 'ETH'
      }

      try {
        const res = await fetch(apiUrl('/v1/markets'))
        if (!res.ok) throw new Error('market fetch failed')
        const data: Market[] = await res.json()
        const initialMarket = data.find((m) => m.id === 'crosschain-eth') || data[0] || fallbackMarket
        setSelectedMarket(initialMarket)
        setCollateralMode(initialMarket.collateralType === 'crosschain' ? 'eth' : 'sui')
      } catch (err) {
        console.warn('Failed to fetch markets, using fallback', err)
        setSelectedMarket(fallbackMarket)
        setCollateralMode('eth')
      }
    }

    loadMarkets()
  }, [])

  return (
    <div className="min-h-screen bg-bg-base">
      <Header
        lfsBalance="1,234.56"
        darkMode={darkMode}
        currency={currency}
        language={language}
        defaultSlippage={defaultSlippage}
        onDarkModeChange={setDarkMode}
        onCurrencyChange={setCurrency}
        onLanguageChange={setLanguage}
        onSlippageChange={setDefaultSlippage}
      />

      <main className="max-w-7xl mx-auto px-6 py-6">
        <div className="grid lg:grid-cols-[1fr,420px] gap-6">
          {/* Left Column - Charts/Analytics */}
          <div className="order-2 lg:order-1 space-y-6">
            <ChartPanel
              collateralMode={collateralMode}
              onCollateralModeChange={setCollateralMode}
              isCrossChain={selectedMarket?.collateralType === 'crosschain'}
            />
            <Card className="p-0 overflow-hidden">
              <MyPositions />
            </Card>
          </div>

          {/* Right Column - Trade Panel */}
          <div className="order-1 lg:order-2 space-y-4">            
            {userAddress && selectedMarket?.collateralType === 'crosschain' && (
              <CrossChainBalance
                suiOwner={userAddress}
                chainId={selectedMarket.chainId || 'ethereum'}
                asset={selectedMarket.asset || 'ETH'}
                refreshIndex={balanceRefresh}
                pollMs={12000}
                showEmpty
              />
            )}

            <TradePanel
              activeTab={activeTab}
              onTabChange={handleTabChange}
              defaultSlippage={defaultSlippage}
              suiNetwork={suiNetwork}
              collateralMode={collateralMode}
              crossChainAsset={selectedMarket?.asset || selectedMarket?.collateralSymbol}
              crossChainChainId={selectedMarket?.chainId}
              onCrossChainMint={() => setBalanceRefresh((v) => v + 1)}
            />

            {userAddress && selectedMarket?.collateralType === 'crosschain' && (
              <VoucherManager
                suiOwner={userAddress}
                chainId={selectedMarket.chainId || 'ethereum'}
                asset={selectedMarket.asset || 'ETH'}
              />
            )}
            
            {/* Protocol Health Widgets */}
            <div className="space-y-4">
              <ProtocolHealth />
              <RebalanceFeed />
            </div>
          </div>
        </div>
      </main>

      {/* Footer */}
      <footer className="mt-12 border-t border-border-subtle py-8">
        <div className="max-w-7xl mx-auto px-6">
          <div className="flex flex-wrap justify-between items-center gap-4 text-text-muted text-sm">
            <div className="flex gap-6">
              <a href="#" className="hover:text-text-primary transition-colors">Documentation</a>
              <a href="#" className="hover:text-text-primary transition-colors">Help</a>
              <a href="#" className="hover:text-text-primary transition-colors">Audits</a>
            </div>
            <div>
              <span>Leafsii Protocol v1.0.0</span>
            </div>
          </div>
        </div>
      </footer>
    </div>
  )
}
