import { useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Card } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { apiUrl } from '@/utils/api'
import type { WithdrawalVoucher } from '@/types/crosschain'
import { toast } from 'sonner'
import { Loader2, ExternalLink } from 'lucide-react'

interface VoucherManagerProps {
  suiOwner: string
  chainId: string
  asset: string
}

export function VoucherManager({ suiOwner, chainId, asset }: VoucherManagerProps) {
  const [vouchers, setVouchers] = useState<WithdrawalVoucher[]>([])
  const [shares, setShares] = useState('1.0')
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const loadVouchers = async () => {
    setLoading(true)
    try {
      const res = await fetch(apiUrl(`/v1/crosschain/vouchers?suiOwner=${suiOwner}`))
      if (!res.ok) {
        throw new Error('voucher load failed')
      }
      const data = await res.json()
      setVouchers(data.vouchers || data.Vouchers || [])
    } catch (error) {
      console.warn('Failed to fetch vouchers', error)
      toast.error('Unable to fetch vouchers')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadVouchers()
  }, [suiOwner])

  const createVoucher = async () => {
    setSubmitting(true)
    const expiry = Math.floor(Date.now() / 1000) + 60 * 60 * 24 * 7
    try {
      const res = await fetch(apiUrl('/v1/crosschain/voucher'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ suiOwner, chainId, asset, shares, expiry })
      })
      if (!res.ok) {
        throw new Error('voucher creation failed')
      }
      const data = await res.json()
      toast.success(`Voucher created: ${data.voucher?.voucherId || data.voucherId}`)
      setShares('1.0')
      loadVouchers()
    } catch (error) {
      console.warn('Failed to create voucher', error)
      toast.error('Unable to create voucher')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card className="p-5 border border-border-subtle bg-bg-card2">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-xs uppercase tracking-wide text-text-muted">Withdrawals</div>
          <div className="text-lg font-semibold text-text-primary">Voucher Manager</div>
        </div>
        <Button size="sm" variant="secondary" onClick={loadVouchers} disabled={loading}>
          {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : 'Refresh'}
        </Button>
      </div>

      <div className="mt-3 flex flex-col sm:flex-row gap-3">
        <Input
          value={shares}
          onChange={(e) => setShares(e.target.value)}
          placeholder="Shares to withdraw"
          className="bg-bg-card"
        />
        <Button onClick={createVoucher} disabled={submitting || !shares}>
          {submitting ? <Loader2 className="w-4 h-4 animate-spin" /> : `Create ${asset} voucher`}
        </Button>
      </div>
      <div className="text-xs text-text-muted mt-2">Creates a signed voucher that can be redeemed directly on {chainId}.</div>

      <div className="mt-4 space-y-2">
        {loading && vouchers.length === 0 && <div className="text-sm text-text-secondary">Loading vouchers...</div>}
        {!loading && vouchers.length === 0 && (
          <div className="text-sm text-text-secondary">No vouchers yet. Create one to redeem your ETH on Ethereum.</div>
        )}
        {vouchers.map((voucher) => (
          <VoucherCard key={voucher.voucherId} voucher={voucher} />
        ))}
      </div>
    </Card>
  )
}

function VoucherCard({ voucher }: { voucher: WithdrawalVoucher }) {
  const expiryDate = useMemo(() => new Date(voucher.expiry * 1000), [voucher.expiry])
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(voucher, null, 2))
      toast.success('Voucher copied to clipboard')
    } catch (error) {
      console.warn('Copy failed', error)
      toast.error('Failed to copy voucher')
    }
  }

  const explorerUrl = voucher.txHash ? `https://etherscan.io/tx/${voucher.txHash}` : undefined

  return (
    <div className="border border-border-subtle rounded-lg p-3 bg-bg-card">
      <div className="flex items-center justify-between gap-2">
        <div className="font-mono text-xs text-text-primary">{voucher.voucherId}</div>
        <span className={`text-xs px-2 py-1 rounded-full ${statusStyle(voucher.status)}`}>
          {voucher.status}
        </span>
      </div>
      <div className="text-sm text-text-secondary mt-1">
        {voucher.shares} {voucher.asset} shares · expires {expiryDate.toLocaleDateString()}
      </div>
      <div className="text-xs text-text-muted mt-1">Nonce: {voucher.nonce}</div>
      <div className="flex items-center gap-3 mt-3">
        <Button size="sm" variant="secondary" onClick={handleCopy}>
          Copy JSON
        </Button>
        {explorerUrl && (
          <a href={explorerUrl} target="_blank" rel="noreferrer" className="text-sm text-brand-primary inline-flex items-center gap-1">
            View on Etherscan <ExternalLink className="w-4 h-4" />
          </a>
        )}
      </div>
    </div>
  )
}

function statusStyle(status: string) {
  switch (status) {
    case 'pending':
      return 'bg-amber-500/10 text-amber-300 border border-amber-500/30'
    case 'spent':
      return 'bg-emerald-500/10 text-emerald-300 border border-emerald-500/30'
    case 'settled':
      return 'bg-blue-500/10 text-blue-200 border border-blue-500/30'
    default:
      return 'bg-bg-card2 text-text-secondary border border-border-subtle'
  }
}
