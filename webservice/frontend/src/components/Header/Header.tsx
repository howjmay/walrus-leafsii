import { Zap } from 'lucide-react'
import { NetworkSelect } from './NetworkSelect'
import { WalletButton } from './WalletButton'
import { Settings } from './Settings'

interface HeaderProps {
  lfsBalance: string
  darkMode: boolean
  currency: string
  language: string
  defaultSlippage: number
  demoMode?: boolean
  onDarkModeChange: (enabled: boolean) => void
  onCurrencyChange: (currency: string) => void
  onLanguageChange: (language: string) => void
  onSlippageChange: (slippage: number) => void
}

export function Header({
  lfsBalance,
  darkMode,
  currency,
  language,
  defaultSlippage,
  demoMode = false,
  onDarkModeChange,
  onCurrencyChange,
  onLanguageChange,
  onSlippageChange
}: HeaderProps) {
  return (
    <header className="sticky top-0 z-50 h-16 bg-bg-card/90 backdrop-blur-md border-b border-border-subtle">
      <div className="max-w-7xl mx-auto px-6 h-full flex items-center justify-between">
        {/* Logo */}
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 bg-gradient-to-br from-brand-primary to-brand-soft rounded-lg flex items-center justify-center">
            <Zap className="w-5 h-5 text-white" />
          </div>
          <div className="font-bold text-xl text-text-primary">
            Leafsii Protocol
          </div>
          {demoMode && (
            <div className="text-xs bg-warn/20 text-warn px-2 py-1 rounded">
              DEMO
            </div>
          )}
        </div>

        {/* Center - Network */}
        <NetworkSelect />

        {/* Right - Balance, Settings, Wallet */}
        <div className="flex items-center gap-4">
          {/* LFS Balance */}
          <div className="hidden sm:flex items-center gap-2 bg-bg-input px-3 py-2 rounded-lg">
            <div className="w-4 h-4 bg-brand-primary rounded-full" />
            <span className="text-text-primary font-semibold">{lfsBalance} LFS</span>
          </div>

          {/* Settings */}
          <Settings
            darkMode={darkMode}
            currency={currency}
            language={language}
            defaultSlippage={defaultSlippage}
            onDarkModeChange={onDarkModeChange}
            onCurrencyChange={onCurrencyChange}
            onLanguageChange={onLanguageChange}
            onSlippageChange={onSlippageChange}
          />

          {/* Wallet */}
          <WalletButton />
        </div>
      </div>
    </header>
  )
}