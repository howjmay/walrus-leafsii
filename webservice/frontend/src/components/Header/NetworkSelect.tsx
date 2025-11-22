import { useState, useContext } from 'react'
import { ChevronDown, Globe } from 'lucide-react'
import { SuiClientContext } from '@mysten/dapp-kit'
import { Button } from '@/components/ui/Button'

export function NetworkSelect() {
  const [isOpen, setIsOpen] = useState(false)
  const ctx = useContext(SuiClientContext)

  if (!ctx) {
    throw new Error('NetworkSelect must be used within SuiClientProvider')
  }

  const { network: currentNetwork, selectNetwork } = ctx

  const networks = [
    { value: 'localnet' as const, label: 'Localnet', color: 'text-info' },
    { value: 'testnet' as const, label: 'Testnet', color: 'text-warn' },
    { value: 'mainnet' as const, label: 'Mainnet', color: 'text-success' },
  ]

  const activeNetwork = networks.find(n => n.value === currentNetwork)

  return (
    <div className="relative">
      <Button
        variant="secondary"
        size="md"
        onClick={() => setIsOpen(!isOpen)}
        className="gap-2"
      >
        <Globe className="w-4 h-4" />
        <span className={activeNetwork?.color}>{activeNetwork?.label}</span>
        <ChevronDown className={`w-4 h-4 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
      </Button>

      {isOpen && (
        <div className="absolute top-full mt-2 left-0 min-w-full bg-bg-card border border-border-subtle rounded-xl shadow-card z-50">
          {networks.map((net) => (
            <button
              key={net.value}
              onClick={() => {
                selectNetwork(net.value)
                setIsOpen(false)
              }}
              className={`w-full px-4 py-3 text-left hover:bg-bg-card2 transition-colors first:rounded-t-xl last:rounded-b-xl ${
                net.value === currentNetwork ? 'bg-bg-card2' : ''
              }`}
            >
              <div className="flex items-center gap-2">
                <Globe className="w-4 h-4" />
                <span className={net.color}>{net.label}</span>
                {net.value === currentNetwork && (
                  <div className="ml-auto w-2 h-2 bg-brand-primary rounded-full" />
                )}
              </div>
            </button>
          ))}
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
