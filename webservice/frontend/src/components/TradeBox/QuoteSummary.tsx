import { Info } from 'lucide-react'

interface QuoteSummaryProps {
  rate?: string
  minReceived?: string
  slippage?: string
  priceImpact?: string
  fee?: string
  route?: string
  postTxCR?: string
}

interface RowProps {
  label: string
  value: string
  tooltip?: string
  highlight?: boolean
}

function SummaryRow({ label, value, tooltip, highlight = false }: RowProps) {
  return (
    <div className={`flex items-center justify-between py-2 ${highlight ? 'text-brand-primary' : 'text-text-muted'}`}>
      <div className="flex items-center gap-1">
        <span className="text-sm">{label}</span>
        {tooltip && (
          <div className="relative group">
            <Info className="w-3 h-3 cursor-help" />
            <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 px-2 py-1 bg-bg-card border border-border-subtle rounded text-xs whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-10">
              {tooltip}
            </div>
          </div>
        )}
      </div>
      <span className={`text-sm font-medium ${highlight ? 'text-brand-primary' : 'text-text-primary'}`}>
        {value}
      </span>
    </div>
  )
}

export function QuoteSummary({
  rate,
  minReceived,
  slippage,
  priceImpact,
  fee,
  route,
  postTxCR
}: QuoteSummaryProps) {
  return (
    <div className="bg-bg-card2/30 rounded-xl p-4 space-y-1">
      {rate && (
        <SummaryRow
          label="Price"
          value={rate}
          tooltip="Current exchange rate for this pair"
        />
      )}
      
      {minReceived && (
        <SummaryRow
          label="Min received"
          value={minReceived}
          tooltip="Minimum amount you'll receive after slippage"
        />
      )}
      
      {slippage && (
        <SummaryRow
          label="Slippage"
          value={slippage}
          tooltip="Maximum price difference you'll accept"
        />
      )}
      
      {priceImpact && (
        <SummaryRow
          label="Price impact"
          value={priceImpact}
          tooltip="How much this trade affects the market price"
          highlight={parseFloat(priceImpact) > 1}
        />
      )}
      
      {fee && (
        <SummaryRow
          label="Fee"
          value={fee}
          tooltip="Trading fee charged by the protocol"
        />
      )}
      
      {postTxCR && (
        <SummaryRow
          label="Post-tx CR"
          value={postTxCR}
          tooltip="Collateral ratio after this transaction"
          highlight={parseFloat(postTxCR) < 1.2}
        />
      )}
      
      {route && (
        <div className="pt-2 border-t border-border-subtle mt-3">
          <div className="text-text-muted text-xs">
            <span className="font-medium">Route:</span> {route}
          </div>
        </div>
      )}
    </div>
  )
}