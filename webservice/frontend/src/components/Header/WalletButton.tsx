import { ConnectModal, useCurrentWallet, useDisconnectWallet } from '@mysten/dapp-kit'
import { Wallet, LogOut, Copy } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'

function addressEllipsis(address: string): string {
  if (!address) return ''
  return `${address.slice(0, 6)}...${address.slice(-4)}`
}

export function WalletButton() {
  const currentWallet = useCurrentWallet()
  const { mutate: disconnect } = useDisconnectWallet()
  const [isOpen, setIsOpen] = useState(false)
  const [connectModalOpen, setConnectModalOpen] = useState(false)

  if (!currentWallet.currentWallet) {
    return (
      <ConnectModal
        trigger={
          <Button
            variant="primary"
            size="md"
            className="gap-2"
          >
            <Wallet className="w-4 h-4" />
            Connect Wallet
          </Button>
        }
        open={connectModalOpen}
        onOpenChange={setConnectModalOpen}
      />
    )
  }

  const handleCopy = async () => {
    if (currentWallet.currentWallet?.accounts[0]?.address) {
      await navigator.clipboard.writeText(currentWallet.currentWallet.accounts[0].address)
      toast.success('Address copied to clipboard')
      setIsOpen(false)
    }
  }

  const handleDisconnect = () => {
    disconnect()
    setIsOpen(false)
  }

  return (
    <div className="relative">
      <Button
        variant="primary"
        size="md"
        onClick={() => setIsOpen(!isOpen)}
        className="gap-2"
      >
        <Wallet className="w-4 h-4" />
        {addressEllipsis(currentWallet.currentWallet?.accounts[0]?.address || '')}
      </Button>

      {isOpen && (
        <div className="absolute top-full mt-2 right-0 min-w-48 bg-bg-card border border-border-subtle rounded-xl shadow-card z-50">
          <div className="p-4 border-b border-border-subtle">
            <div className="text-text-muted text-sm">Connected Account</div>
            <div className="text-text-primary font-mono text-sm mt-1">
              {addressEllipsis(currentWallet.currentWallet?.accounts[0]?.address || '')}
            </div>
          </div>
          
          <button
            onClick={handleCopy}
            className="w-full px-4 py-3 text-left hover:bg-bg-card2 transition-colors flex items-center gap-2 text-text-primary"
          >
            <Copy className="w-4 h-4" />
            Copy Address
          </button>
          
          <button
            onClick={handleDisconnect}
            className="w-full px-4 py-3 text-left hover:bg-bg-card2 transition-colors flex items-center gap-2 text-danger rounded-b-xl"
          >
            <LogOut className="w-4 h-4" />
            Disconnect
          </button>
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