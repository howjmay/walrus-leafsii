import { Activity, TrendingUp, TrendingDown, Clock, AlertTriangle } from 'lucide-react'
import { Card } from '@/components/ui/Card'
import { formatPercentage } from '@/lib/utils'
import { useProtocolData } from '@/hooks/useSimpleProtocol'
import { useProtocolState } from '@/hooks/useProtocolState'

export function ProtocolHealth() {
  const { data: protocolData, isLoading } = useProtocolData()
  const { data: protocolState, isLoading: isPxLoading, error: pxError } = useProtocolState()

  const formatTimeAgo = (timestamp: number) => {
    const seconds = Math.floor(Date.now() / 1000 - timestamp);
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    return `${hours}h ago`;
  };
  
  if (isLoading || !protocolData) {
    return (
      <Card className="p-4">
        <div className="animate-pulse">
          <div className="h-6 bg-bg-input rounded mb-4"></div>
          <div className="space-y-3">
            <div className="h-16 bg-bg-input rounded"></div>
            <div className="h-16 bg-bg-input rounded"></div>
          </div>
        </div>
      </Card>
    )
  }

  const healthData = {
    currentCR: protocolData.currentCR,
    targetCR: protocolData.targetCR,
    reserveRatio: protocolData.reserves / (protocolData.fTokenSupply + protocolData.xTokenSupply),
    systemMode: protocolData.systemMode,
  }

  const getCRStatus = () => {
    if (healthData.currentCR >= healthData.targetCR) return 'healthy'
    if (healthData.currentCR >= healthData.targetCR - 0.02) return 'warning'
    return 'danger'
  }

  const crStatus = getCRStatus()

  const statusColors = {
    healthy: 'text-success border-success/20 bg-success/10',
    warning: 'text-warn border-warn/20 bg-warn/10',
    danger: 'text-danger border-danger/20 bg-danger/10',
  }

  return (
    <Card className="p-4">
      <div className="flex items-center gap-2 mb-4">
        <Activity className="w-5 h-5 text-brand-primary" />
        <h3 className="text-text-primary font-semibold">Protocol Health</h3>
      </div>

      <div className="space-y-4">
        {/* Px Price */}
        <div className="border-b border-border-subtle pb-3">
          <div className="flex items-center">
            <div className="flex items-center gap-2">
              <TrendingUp className="w-4 h-4 text-brand-primary" />
              <span className="text-text-muted text-sm">Px Price</span>
            </div>

            {isPxLoading ? (
              <span className="ml-auto text-text-muted text-sm">Loading...</span>
            ) : pxError ? (
              <div className="ml-auto flex items-center gap-1">
                <AlertTriangle className="w-4 h-4 text-danger" />
                <span className="text-danger text-sm">Failed to load</span>
              </div>
            ) : (
              <span className="ml-auto inline-flex items-baseline gap-2 text-text-primary font-semibold">
                <span className="tabular-nums">{((protocolState?.px || 0) / 1000000).toFixed(4)}</span>
                <span className="text-text-muted text-sm">USD</span>
              </span>
            )}
          </div>

          {protocolState?.asOf && (
            <div className="mt-2 flex items-center justify-end gap-1">
              <Clock className="w-3 h-3 text-text-muted" />
              <span className="text-text-muted text-xs">
                {formatTimeAgo(protocolState.asOf)}
              </span>
            </div>
          )}
        </div>

        {/* Collateral Ratio */}
        <div className={`p-3 rounded-xl border ${statusColors[crStatus]}`}>
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm font-medium">Collateral Ratio</span>
            <div className="flex items-center gap-1">
              {healthData.currentCR >= healthData.targetCR ? (
                <TrendingUp className="w-4 h-4" />
              ) : (
                <TrendingDown className="w-4 h-4" />
              )}
              <span className="font-bold">{healthData.currentCR.toFixed(3)}</span>
            </div>
          </div>
          <div className="text-xs opacity-75">
            Target: {healthData.targetCR.toFixed(2)} |
            Deviation: {((healthData.currentCR / healthData.targetCR - 1) * 100).toFixed(1)}%
          </div>
        </div>

        {/* Reserve Ratio */}
        <div className="flex items-center justify-between py-2">
          <span className="text-text-muted text-sm">Reserve Ratio</span>
          <span className="text-text-primary font-medium">
            {formatPercentage(healthData.reserveRatio)}
          </span>
        </div>

        {/* System Mode */}
        <div className="flex items-center justify-between py-2">
          <span className="text-text-muted text-sm">System Mode</span>
          <div className="flex items-center gap-2">
            <div className={`w-2 h-2 rounded-full ${
              healthData.systemMode === 'normal' ? 'bg-success animate-pulse' :
              healthData.systemMode === 'rebalance' ? 'bg-warn' : 'bg-danger'
            }`} />
            <span className={`text-sm font-medium capitalize ${
              healthData.systemMode === 'normal' ? 'text-success' :
              healthData.systemMode === 'rebalance' ? 'text-warn' : 'text-danger'
            }`}>
              {healthData.systemMode}
            </span>
          </div>
        </div>
      </div>
    </Card>
  )
}