import { useEffect, useMemo, useState } from 'react'
import { ExternalLink, Loader2, ShieldCheck } from 'lucide-react'
import { Card } from '@/components/ui/Card'
import { apiUrl } from '@/utils/api'
import type { WalrusCheckpoint } from '@/types/crosschain'

interface WalrusCheckpointInfoProps {
  chainId: string
  asset: string
}

export function WalrusCheckpointInfo({ chainId, asset }: WalrusCheckpointInfoProps) {
  const [checkpoint, setCheckpoint] = useState<WalrusCheckpoint | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    const load = async () => {
      setLoading(true)
      try {
        const res = await fetch(apiUrl(`/v1/crosschain/checkpoint?chainId=${chainId}&asset=${asset}`))
        if (res.ok) {
          const data = await res.json()
          setCheckpoint(data.checkpoint || data.Checkpoint || data)
        }
      } catch (error) {
        console.warn('Failed to fetch Walrus checkpoint', error)
      } finally {
        setLoading(false)
      }
    }

    load()
    const interval = setInterval(load, 20_000)
    return () => clearInterval(interval)
  }, [chainId, asset])

  const ageMinutes = useMemo(() => {
    if (!checkpoint?.timestamp) return null
    const ageMs = Date.now() - checkpoint.timestamp * 1000
    return Math.floor(ageMs / 60000)
  }, [checkpoint?.timestamp])

  return (
    <Card className="p-5 border border-border-subtle bg-bg-card">
      <div className="flex items-center justify-between gap-2">
        <div>
          <div className="text-xs uppercase tracking-wide text-text-muted">Walrus Checkpoint</div>
          <div className="text-lg font-semibold text-text-primary">
            {asset} · {chainId}
          </div>
        </div>
        <ShieldCheck className="w-5 h-5 text-emerald-400" />
      </div>

      {loading && !checkpoint && (
        <div className="text-sm text-text-secondary flex items-center gap-2 mt-3">
          <Loader2 className="w-4 h-4 animate-spin" /> Loading latest checkpoint...
        </div>
      )}

      {checkpoint && (
        <div className="grid grid-cols-2 gap-3 mt-4">
          <div className="bg-bg-card2 rounded-lg border border-border-subtle p-3">
            <div className="text-xs text-text-muted">Update ID</div>
            <div className="text-lg font-semibold text-text-primary">#{checkpoint.updateId}</div>
          </div>
          <div className="bg-bg-card2 rounded-lg border border-border-subtle p-3">
            <div className="text-xs text-text-muted">Block</div>
            <div className="text-lg font-semibold text-text-primary">{checkpoint.blockNumber}</div>
          </div>
          <div className="bg-bg-card2 rounded-lg border border-border-subtle p-3">
            <div className="text-xs text-text-muted">Index</div>
            <div className="text-lg font-semibold text-text-primary">{checkpoint.index}</div>
            <div className="text-xs text-text-muted">Total shares {checkpoint.totalShares}</div>
          </div>
          <div className="bg-bg-card2 rounded-lg border border-border-subtle p-3">
            <div className="text-xs text-text-muted">Status</div>
            <div className="text-lg font-semibold text-text-primary capitalize">
              {checkpoint.status}
            </div>
            {ageMinutes !== null && (
              <div className={`text-xs mt-1 ${ageMinutes > 10 ? 'text-amber-400' : 'text-text-muted'}`}>
                Age: {ageMinutes}m {ageMinutes > 10 && ' · stale'}
              </div>
            )}
          </div>
        </div>
      )}

      {checkpoint?.walrusBlobId && (
        <a
          href={`https://walrus.xyz/blob/${checkpoint.walrusBlobId}`}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-2 text-sm text-brand-primary mt-3"
        >
          View Walrus blob <ExternalLink className="w-4 h-4" />
        </a>
      )}
    </Card>
  )
}
