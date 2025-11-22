import { useState } from 'react'
import { Header } from '@/components/Header/Header'
import { ChartPanel } from '@/components/Charts/ChartPanel'
import { TradePanel } from '@/components/TradeBox/TradePanel'
import { ProtocolHealth } from '@/components/Widgets/ProtocolHealth'
import { OracleStatus } from '@/components/Widgets/OracleStatus'
import { RebalanceFeed } from '@/components/Widgets/RebalanceFeed'
import { PxTicker } from '@/components/Widgets/PxTicker'
import { Card } from '@/components/ui/Card'
import { MyPositions } from '@/components/Charts/MyPositions'
import type { Tab } from '@/lib/urls'

export default function DemoTradePage() {
  const [activeTab, setActiveTab] = useState<Tab>('mint')
  const [darkMode, setDarkMode] = useState(true)
  const [currency, setCurrency] = useState('USD')
  const [language, setLanguage] = useState('English')
  const [defaultSlippage, setDefaultSlippage] = useState(0.5)

  const handleTabChange = (newTab: Tab) => {
    setActiveTab(newTab)
  }

  return (
    <div className="min-h-screen bg-bg-base">
      <Header
        lfsBalance="1,234.56"
        darkMode={darkMode}
        currency={currency}
        language={language}
        defaultSlippage={defaultSlippage}
        demoMode={true}
        onDarkModeChange={setDarkMode}
        onCurrencyChange={setCurrency}
        onLanguageChange={setLanguage}
        onSlippageChange={setDefaultSlippage}
      />

      <main className="max-w-7xl mx-auto px-6 py-6">
        <div className="grid lg:grid-cols-[1fr,420px] gap-6">
          {/* Left Column - Charts/Analytics */}
          <div className="order-2 lg:order-1 space-y-6">
            <ChartPanel />
            <Card className="p-0 overflow-hidden">
              <MyPositions />
            </Card>
          </div>

          {/* Right Column - Trade Panel */}
          <div className="order-1 lg:order-2 space-y-4">
            <TradePanel
              activeTab={activeTab}
              onTabChange={handleTabChange}
              defaultSlippage={defaultSlippage}
              demoMode={true}
            />
            
            {/* Protocol Health Widgets */}
            <div className="space-y-4">
              <PxTicker />
              <ProtocolHealth />
              <OracleStatus />
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
              <span>Leafsii Protocol v1.0.0 - Demo Mode</span>
            </div>
          </div>
        </div>
      </footer>
    </div>
  )
}
