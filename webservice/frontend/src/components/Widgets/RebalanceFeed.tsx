import { RotateCcw, ExternalLink, ArrowRight } from 'lucide-react'
import { Card } from '@/components/ui/Card'
import { formatNumber, truncateAddress } from '@/lib/utils'

interface RebalanceEvent {
  id: string
  timestamp: Date
  fTokenBurned: number
  payoutRToken: number
  indexSnapshot: number
  txHash: string
}

export function RebalanceFeed() {
  // Mock data - would come from hooks in real app
  const rebalanceEvents: RebalanceEvent[] = [
    {
      id: '1',
      timestamp: new Date(Date.now() - 2 * 60 * 60 * 1000), // 2 hours ago
      fTokenBurned: 125000,
      payoutRToken: 127500,
      indexSnapshot: 1.0298,
      txHash: '0x1234567890abcdef1234567890abcdef12345678'
    },
    {
      id: '2', 
      timestamp: new Date(Date.now() - 8 * 60 * 60 * 1000), // 8 hours ago
      fTokenBurned: 89000,
      payoutRToken: 90225,
      indexSnapshot: 1.0276,
      txHash: '0x2345678901bcdef12345678901bcdef123456789'
    },
    {
      id: '3',
      timestamp: new Date(Date.now() - 24 * 60 * 60 * 1000), // 1 day ago
      fTokenBurned: 203000,
      payoutRToken: 205590,
      indexSnapshot: 1.0251,
      txHash: '0x345678902cdef123456789012cdef1234567890a'
    }
  ]

  const formatTimeAgo = (date: Date) => {
    const hours = Math.floor((Date.now() - date.getTime()) / (1000 * 60 * 60))
    if (hours < 1) return 'Just now'
    if (hours < 24) return `${hours}h ago`
    const days = Math.floor(hours / 24)
    return `${days}d ago`
  }

  return (
    <Card className="p-4">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <RotateCcw className="w-5 h-5 text-brand-primary" />
          <h3 className="text-text-primary font-semibold">Recent Rebalances</h3>
        </div>
        <button className="text-brand-primary text-sm hover:text-brand-soft transition-colors">
          View all
        </button>
      </div>

      <div className="space-y-3">
        {rebalanceEvents.map((event) => (
          <div
            key={event.id}
            className="bg-bg-card2/30 rounded-xl p-3 border border-border-subtle hover:border-border-strong transition-colors"
          >
            {/* Header */}
            <div className="flex items-center justify-between mb-2">
              <div className="text-text-primary text-sm font-medium">
                System Rebalance
              </div>
              <div className="text-text-muted text-xs">
                {formatTimeAgo(event.timestamp)}
              </div>
            </div>

            {/* Details */}
            <div className="space-y-2">
              {/* Token Flow */}
              <div className="flex items-center gap-2 text-xs">
                <div className="bg-danger/10 text-danger px-2 py-1 rounded">
                  -{formatNumber(event.fTokenBurned, 0)} fToken
                </div>
                <ArrowRight className="w-3 h-3 text-text-muted" />
                <div className="bg-success/10 text-success px-2 py-1 rounded">
                  +{formatNumber(event.payoutRToken, 0)} Sui
                </div>
              </div>

              {/* Index Snapshot */}
              <div className="flex items-center justify-between text-xs">
                <span className="text-text-muted">Index Snapshot</span>
                <span className="text-text-primary font-mono">
                  {event.indexSnapshot.toFixed(4)}
                </span>
              </div>

              {/* Transaction Link */}
              <div className="flex items-center justify-between">
                <span className="text-text-muted text-xs">Transaction</span>
                <a
                  href={`#/tx/${event.txHash}`}
                  className="text-brand-primary text-xs hover:text-brand-soft transition-colors inline-flex items-center gap-1"
                >
                  {truncateAddress(event.txHash, 4, 4)}
                  <ExternalLink className="w-3 h-3" />
                </a>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Info Footer */}
      <div className="mt-4 text-text-muted text-xs bg-bg-card2/20 rounded-lg p-3">
        <div className="font-medium mb-1">Deferred Payout Model</div>
        <div>
          Rebalance payouts are calculated and stored as index snapshots.
          SP participants claim rewards based on their stake duration and index appreciation.
        </div>
      </div>
    </Card>
  )
}
