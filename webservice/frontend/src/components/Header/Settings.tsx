import { useState } from 'react'
import { Settings as SettingsIcon, Moon, Sun, Globe2, DollarSign } from 'lucide-react'
import { Button } from '@/components/ui/Button'

interface SettingsProps {
  darkMode: boolean
  currency: string
  language: string
  defaultSlippage: number
  onDarkModeChange: (enabled: boolean) => void
  onCurrencyChange: (currency: string) => void
  onLanguageChange: (language: string) => void
  onSlippageChange: (slippage: number) => void
}

export function Settings({
  darkMode,
  currency,
  language,
  defaultSlippage,
  onDarkModeChange,
  onCurrencyChange,
  onLanguageChange,
  onSlippageChange
}: SettingsProps) {
  const [isOpen, setIsOpen] = useState(false)

  const currencies = ['USD', 'EUR', 'GBP', 'JPY']
  const languages = ['English', 'Chinese', 'Korean', 'Japanese']
  const slippageOptions = [0.1, 0.5, 1.0, 2.0]

  return (
    <div className="relative">
      <Button
        variant="ghost"
        size="md"
        onClick={() => setIsOpen(!isOpen)}
        className="w-10 h-10 p-0"
      >
        <SettingsIcon className="w-5 h-5" />
      </Button>

      {isOpen && (
        <div className="absolute top-full mt-2 right-0 w-72 bg-bg-card border border-border-subtle rounded-xl shadow-card z-50">
          <div className="p-4 border-b border-border-subtle">
            <h3 className="text-text-primary font-semibold">Settings</h3>
          </div>

          <div className="p-4 space-y-4">
            {/* Theme Toggle */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                {darkMode ? <Moon className="w-4 h-4" /> : <Sun className="w-4 h-4" />}
                <span className="text-text-primary">Dark Mode</span>
              </div>
              <button
                onClick={() => onDarkModeChange(!darkMode)}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  darkMode ? 'bg-brand-primary' : 'bg-bg-input'
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    darkMode ? 'translate-x-6' : 'translate-x-1'
                  }`}
                />
              </button>
            </div>

            {/* Currency */}
            <div>
              <div className="flex items-center gap-2 mb-2">
                <DollarSign className="w-4 h-4" />
                <span className="text-text-primary">Currency</span>
              </div>
              <div className="grid grid-cols-2 gap-2">
                {currencies.map((curr) => (
                  <button
                    key={curr}
                    onClick={() => onCurrencyChange(curr)}
                    className={`px-3 py-2 rounded-lg text-sm transition-colors ${
                      currency === curr
                        ? 'bg-brand-primary text-text-onBrand'
                        : 'bg-bg-input text-text-primary hover:bg-bg-card2'
                    }`}
                  >
                    {curr}
                  </button>
                ))}
              </div>
            </div>

            {/* Language */}
            <div>
              <div className="flex items-center gap-2 mb-2">
                <Globe2 className="w-4 h-4" />
                <span className="text-text-primary">Language</span>
              </div>
              <div className="space-y-1">
                {languages.map((lang) => (
                  <button
                    key={lang}
                    onClick={() => onLanguageChange(lang)}
                    className={`w-full px-3 py-2 rounded-lg text-sm text-left transition-colors ${
                      language === lang
                        ? 'bg-brand-primary text-text-onBrand'
                        : 'bg-bg-input text-text-primary hover:bg-bg-card2'
                    }`}
                  >
                    {lang}
                  </button>
                ))}
              </div>
            </div>

            {/* Default Slippage */}
            <div>
              <div className="text-text-primary mb-2">Default Slippage</div>
              <div className="grid grid-cols-4 gap-2">
                {slippageOptions.map((slip) => (
                  <button
                    key={slip}
                    onClick={() => onSlippageChange(slip)}
                    className={`px-2 py-2 rounded-lg text-sm transition-colors ${
                      defaultSlippage === slip
                        ? 'bg-brand-primary text-text-onBrand'
                        : 'bg-bg-input text-text-primary hover:bg-bg-card2'
                    }`}
                  >
                    {slip}%
                  </button>
                ))}
              </div>
            </div>
          </div>
        </div>
      )}

      {isOpen && (
        <div 
          className="fixed inset-0 z-40" 
          onClick={() => setIsOpen(false)}
        />
      )}
    </div>
  )
}