import { useEffect, useState } from 'react'
import { RefreshCcw, Loader2 } from 'lucide-react'
import { Card } from '@/components/ui/Card'
import type { CrossChainBalance } from '@/types/crosschain'
import { apiUrl } from '@/utils/api'
import { formatCurrency, formatNumber } from '@/lib/utils'
import { Button } from '@/components/ui/Button'
import { toast } from 'sonner'

interface CrossChainBalanceProps {
  suiOwner: string
  chainId: string
  asset: string
  showEmpty?: boolean
  refreshIndex?: number
  pollMs?: number
}

export function CrossChainBalance({
  suiOwner,
  chainId,
  asset,
  showEmpty = false,
  refreshIndex,
  pollMs
}: CrossChainBalanceProps) {
  const [balance, setBalance] = useState<CrossChainBalance | null>(null)
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const res = await fetch(apiUrl(`/v1/crosschain/balance?suiOwner=${suiOwner}&chainId=${chainId}&asset=${asset}`))
      if (!res.ok) {
        throw new Error(`balance request failed: ${res.status}`)
      }
      const data = await res.json()
      setBalance(data.balance || data.Balance || data)
    } catch (error) {
      console.warn('Failed to fetch cross-chain balance', error)
      toast.error('Unable to load cross-chain balance')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [suiOwner, chainId, asset, refreshIndex])

  useEffect(() => {
    if (!pollMs || pollMs <= 0) return

    const id = setInterval(() => {
      load()
    }, pollMs)
    return () => clearInterval(id)
  }, [pollMs, suiOwner, chainId, asset])

  const hasBalance = balance && Number(balance.shares) > 0
  if (!hasBalance && !showEmpty) return null

  const updatedAt = balance?.updatedAt ? new Date(balance.updatedAt * 1000) : null

  return (
    <Card className="p-5 border border-border-subtle bg-bg-card2">
      <div className="flex items-center justify-between gap-2">
        <div>
          <div className="text-xs uppercase tracking-wide text-text-muted">Cross-Chain Collateral</div>
          <div className="text-lg font-semibold text-text-primary">
            {asset} on {chainId}
          </div>
        </div>
        <Button variant="secondary" size="sm" onClick={load} disabled={loading}>
          {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <RefreshCcw className="w-4 h-4" />}
        </Button>
      </div>

      {loading && !balance ? (
        <div className="text-sm text-text-secondary mt-3">Loading balance...</div>
      ) : (
        <div className="grid grid-cols-2 gap-3 mt-4">
          <div className="bg-bg-card rounded-lg border border-border-subtle p-3">
            <div className="text-xs text-text-muted">Shares</div>
            <div className="text-xl font-semibold text-text-primary">{Number(balance?.shares || 0).toFixed(4)}</div>
          </div>
          <div className="bg-bg-card rounded-lg border border-border-subtle p-3">
            <div className="text-xs text-text-muted">Value</div>
            <div className="text-xl font-semibold text-text-primary">
              {formatNumber(Number(balance?.value || 0), 4)} {asset}
            </div>
          </div>
          <div className="bg-bg-card rounded-lg border border-border-subtle p-3">
            <div className="text-xs text-text-muted">Collateral</div>
            <div className="text-xl font-semibold text-text-primary">
              {formatCurrency(Number(balance?.collateralUsd || 0))}
            </div>
          </div>
          <div className="bg-bg-card rounded-lg border border-border-subtle p-3">
            <div className="text-xs text-text-muted">Checkpoint</div>
            <div className="text-lg font-semibold text-text-primary">
              #{balance?.lastCheckpointId ?? '—'}
            </div>
          </div>
        </div>
      )}

      {updatedAt && (
        <div className="text-xs text-text-muted mt-3">
          Updated {updatedAt.toLocaleString()}
        </div>
      )}
    </Card>
  )
}

export function CrossChainBalanceList({ suiOwner }: { suiOwner: string }) {
  return (
    <div className="space-y-3">
      <CrossChainBalance suiOwner={suiOwner} chainId="ethereum" asset="ETH" showEmpty />
    </div>
  )
}
