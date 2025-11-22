import { Clock, AlertTriangle, CheckCircle, Wifi } from 'lucide-react'
import { Card } from '@/components/ui/Card'

export function OracleStatus() {
  // Mock data - would come from hooks in real app
  const oracleData = {
    lastUpdate: new Date(Date.now() - 23000), // 23 seconds ago
    provider: 'Pyth Network',
    price: 0.9985,
    staleness: 23, // seconds
    isStale: false,
    failoverActive: false,
  }

  const getStalenessStatus = () => {
    if (oracleData.staleness < 60) return 'fresh'
    if (oracleData.staleness < 180) return 'warning'
    return 'stale'
  }

  const status = getStalenessStatus()
  
  const statusConfig = {
    fresh: {
      color: 'text-success',
      bgColor: 'bg-success/10',
      borderColor: 'border-success/20',
      icon: CheckCircle,
      label: 'Active'
    },
    warning: {
      color: 'text-warn',
      bgColor: 'bg-warn/10',
      borderColor: 'border-warn/20',
      icon: Clock,
      label: 'Delayed'
    },
    stale: {
      color: 'text-danger',
      bgColor: 'bg-danger/10',
      borderColor: 'border-danger/20',
      icon: AlertTriangle,
      label: 'Stale'
    }
  }

  const config = statusConfig[status]
  const StatusIcon = config.icon

  const formatTimeAgo = (date: Date) => {
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    const minutes = Math.floor(seconds / 60)
    if (minutes < 60) return `${minutes}m ago`
    const hours = Math.floor(minutes / 60)
    return `${hours}h ago`
  }

  return (
    <Card className="p-4">
      <div className="flex items-center gap-2 mb-4">
        <Wifi className="w-5 h-5 text-brand-primary" />
        <h3 className="text-text-primary font-semibold">Oracle Status</h3>
      </div>

      <div className="space-y-3">
        {/* Status Indicator */}
        <div className={`flex items-center justify-between p-3 rounded-xl border ${config.bgColor} ${config.borderColor}`}>
          <div className="flex items-center gap-2">
            <StatusIcon className={`w-4 h-4 ${config.color}`} />
            <span className={`font-medium ${config.color}`}>{config.label}</span>
          </div>
          <span className="text-text-primary text-sm font-mono">
            ${oracleData.price.toFixed(4)}
          </span>
        </div>

        {/* Details */}
        <div className="space-y-2 text-sm">
          <div className="flex items-center justify-between">
            <span className="text-text-muted">Last Update</span>
            <span className="text-text-primary font-medium">
              {formatTimeAgo(oracleData.lastUpdate)}
            </span>
          </div>

          <div className="flex items-center justify-between">
            <span className="text-text-muted">Provider</span>
            <span className="text-text-primary font-medium">
              {oracleData.provider}
            </span>
          </div>

          {oracleData.failoverActive && (
            <div className="flex items-center gap-2 p-2 bg-warn/10 border border-warn/20 rounded-lg">
              <AlertTriangle className="w-4 h-4 text-warn" />
              <span className="text-warn text-xs font-medium">Failover Source Active</span>
            </div>
          )}
        </div>

        {/* Staleness Warning */}
        {status === 'stale' && (
          <div className="bg-danger/10 border border-danger/20 rounded-lg p-3">
            <div className="text-danger text-xs font-medium mb-1">
              Oracle Data Stale
            </div>
            <div className="text-text-muted text-xs">
              Transactions may be disabled until fresh data is available
            </div>
          </div>
        )}
      </div>
    </Card>
  )
}