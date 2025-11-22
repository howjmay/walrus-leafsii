import { useEffect, useMemo, useState } from 'react'
import { ExternalLink, Copy, Loader2, CheckCircle2, Wallet, Sparkles } from 'lucide-react'
import { keccak_256 } from '@noble/hashes/sha3'
import { bytesToHex, utf8ToBytes } from '@noble/hashes/utils'
import { Card } from '@/components/ui/Card'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { apiUrl } from '@/utils/api'
import type { BridgeReceipt, CollateralParams, VaultInfo } from '@/types/crosschain'
import type { Market } from '@/types/market'
import { toast } from 'sonner'
import { formatNumber } from '@/lib/utils'

type EthereumProvider = {
  request: (args: { method: string; params?: any[] }) => Promise<any>
  on?: (event: string, handler: (...args: any[]) => void) => void
  removeListener?: (event: string, handler: (...args: any[]) => void) => void
}

declare global {
  interface Window {
    ethereum?: EthereumProvider
  }
}

interface CrossChainDepositProps {
  market: Market
  userAddress: string
  onMinted?: (receipt?: BridgeReceipt) => void
}

const DEPOSIT_SELECTOR = bytesToHex(keccak_256(utf8ToBytes('deposit(address,string,uint256)'))).slice(0, 8)

const explorerByChain = (chainId?: string | null) => {
  switch (chainId) {
    case '0x1':
      return 'https://etherscan.io'
    case '0xaa36a7': // Sepolia
      return 'https://sepolia.etherscan.io'
    default:
      return 'https://etherscan.io'
  }
}

function pad32(hexValue: string) {
  return hexValue.replace(/^0x/, '').padStart(64, '0')
}

function parseUnits(value: string, decimals = 18): bigint {
  const sanitized = value.trim()
  if (!sanitized) throw new Error('Amount is required')
  if (!/^\d+(\.\d+)?$/.test(sanitized)) throw new Error('Invalid number')
  const [intPart, fracPart = ''] = sanitized.split('.')
  const trimmedFrac = fracPart.slice(0, decimals)
  const paddedFrac = trimmedFrac.padEnd(decimals, '0')
  const combined = `${intPart}${paddedFrac}`
  return BigInt(combined)
}

function encodeDepositCalldata(recipient: string, suiOwner: string, minShares: bigint) {
  const normalizedRecipient = recipient.startsWith('0x') ? recipient.slice(2) : recipient
  if (normalizedRecipient.length !== 40) {
    throw new Error('Recipient address must be 20 bytes')
  }

  const ownerBytes = bytesToHex(utf8ToBytes(suiOwner))
  const ownerLength = ownerBytes.length / 2
  const ownerPadded = ownerBytes.padEnd(Math.max(64, Math.ceil(ownerBytes.length / 64) * 64), '0')

  const selector = DEPOSIT_SELECTOR
  const headRecipient = pad32(normalizedRecipient)
  const headOffset = pad32((32 * 3).toString(16)) // dynamic data starts after 3 words
  const headMinShares = pad32(minShares.toString(16))
  const ownerLengthWord = pad32(ownerLength.toString(16))

  return `0x${selector}${headRecipient}${headOffset}${headMinShares}${ownerLengthWord}${ownerPadded}`
}

function shortenAddress(address: string, size = 4) {
  if (!address) return ''
  return `${address.slice(0, 2 + size)}...${address.slice(-size)}`
}

function hexToDecimal(hex?: string | null) {
  if (!hex) return ''
  try {
    return String(parseInt(hex, 16))
  } catch {
    return ''
  }
}

export function CrossChainDeposit({ market, userAddress, onMinted }: CrossChainDepositProps) {
  const [vault, setVault] = useState<VaultInfo | null>(null)
  const [params, setParams] = useState<CollateralParams | null>(null)
  const [amount, setAmount] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [fetching, setFetching] = useState(false)
  const [evmAccount, setEvmAccount] = useState<string | null>(null)
  const [evmChainId, setEvmChainId] = useState<string | null>(null)
  const [txHash, setTxHash] = useState<string | null>(null)
  const [bridgeReceipt, setBridgeReceipt] = useState<BridgeReceipt | null>(null)
  const [status, setStatus] = useState<'idle' | 'connecting' | 'signing' | 'submitted' | 'minting' | 'minted' | 'error'>('idle')

  const chainId = useMemo(() => market.chainId || 'ethereum', [market.chainId])
  const asset = useMemo(() => market.asset || market.collateralSymbol || 'ETH', [market.asset, market.collateralSymbol])

  useEffect(() => {
    const load = async () => {
      setFetching(true)
      try {
        const [vaultRes, paramsRes] = await Promise.all([
          fetch(apiUrl(`/v1/crosschain/vault?chainId=${chainId}&asset=${asset}`)),
          fetch(apiUrl(`/v1/crosschain/params?chainId=${chainId}&asset=${asset}`))
        ])

        if (vaultRes.ok) {
          const data = await vaultRes.json()
          setVault(data.vault || data.Vault || data)
        }

        if (paramsRes.ok) {
          const data = await paramsRes.json()
          setParams(data.params || data.Params || null)
        }
      } catch (error) {
        console.warn('Failed to load cross-chain data', error)
      } finally {
        setFetching(false)
      }
    }

    load()
  }, [chainId, asset])

  useEffect(() => {
    const eth = typeof window !== 'undefined' ? window.ethereum : undefined
    if (!eth) return

    const handleAccounts = (accounts: string[]) => {
      setEvmAccount(accounts[0] || null)
    }
    const handleChain = (chain: string) => setEvmChainId(chain)

    eth.on?.('accountsChanged', handleAccounts)
    eth.on?.('chainChanged', handleChain)

    return () => {
      eth.removeListener?.('accountsChanged', handleAccounts)
      eth.removeListener?.('chainChanged', handleChain)
    }
  }, [])

  const handleCopy = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(`${label} copied`)
    } catch (err) {
      console.warn('Clipboard copy failed', err)
      toast.error('Failed to copy')
    }
  }

  const connectEvm = async () => {
    const eth = typeof window !== 'undefined' ? window.ethereum : undefined
    if (!eth) {
      toast.error('No EVM wallet detected. Open MetaMask or another EVM wallet.')
      return null
    }
    setStatus('connecting')
    try {
      const accounts = await eth.request({ method: 'eth_requestAccounts' })
      const chainHex = await eth.request({ method: 'eth_chainId' })
      setEvmAccount(accounts[0])
      setEvmChainId(chainHex)
      setStatus('idle')
      return accounts[0] as string
    } catch (error) {
      console.warn('Failed to connect wallet', error)
      setStatus('idle')
      toast.error('Wallet connection rejected')
      return null
    }
  }

  const waitForReceipt = async (hash: string) => {
    const eth = typeof window !== 'undefined' ? window.ethereum : undefined
    if (!eth) return null
    for (let i = 0; i < 12; i += 1) {
      try {
        const receipt = await eth.request({ method: 'eth_getTransactionReceipt', params: [hash] })
        if (receipt) return receipt
      } catch (error) {
        console.warn('receipt poll failed', error)
      }
      await new Promise((resolve) => setTimeout(resolve, 1500))
    }
    return null
  }

  const submitBridgeMint = async (hash: string) => {
    setStatus('minting')
    const res = await fetch(apiUrl('/v1/crosschain/deposit'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        txHash: hash,
        suiOwner: userAddress,
        chainId,
        asset,
        amount
      })
    })

    if (!res.ok) {
      const errorText = await res.text()
      throw new Error(errorText || 'Bridge mint failed')
    }

    const data = await res.json()
    const receipt: BridgeReceipt = data.receipt || data.Receipt || data
    setBridgeReceipt(receipt)
    setStatus('minted')
    toast.success(`Minted ${formatNumber(Number(receipt.minted || amount), 4)} ${asset} to your Sui account`)
    onMinted?.(receipt)
    return receipt
  }

  const handleDeposit = async () => {
    if (!vault?.vaultAddress) {
      toast.error('Vault address unavailable. Try again in a moment.')
      return
    }
    if (!amount) {
      toast.error('Enter an amount to deposit')
      return
    }

    const eth = typeof window !== 'undefined' ? window.ethereum : undefined
    const account = evmAccount || (await connectEvm())
    if (!eth || !account) return

    let weiAmount: bigint
    try {
      weiAmount = parseUnits(amount, 18)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Invalid amount')
      return
    }

    setSubmitting(true)
    setStatus('signing')
    setBridgeReceipt(null)
    setTxHash(null)

    try {
      const calldata = encodeDepositCalldata(account, userAddress, 0n)
      const hash: string = await eth.request({
        method: 'eth_sendTransaction',
        params: [
          {
            from: account,
            to: vault.vaultAddress,
            value: `0x${weiAmount.toString(16)}`,
            data: calldata
          }
        ]
      })

      setTxHash(hash)
      setStatus('submitted')
      toast.success('Ethereum transaction submitted')

      const receipt = await waitForReceipt(hash)
      if (receipt && receipt.status === '0x0') {
        throw new Error('Ethereum transaction failed')
      }

      await submitBridgeMint(hash)
      setAmount('')
    } catch (error) {
      console.error('Deposit failed', error)
      setStatus('error')
      const message = error instanceof Error ? error.message : 'Deposit failed'
      toast.error(message)
    } finally {
      setSubmitting(false)
    }
  }

  const ltv = params ? (Number(params.ltv) * 100).toFixed(0) : '65'
  const maintenance = params ? (Number(params.maintenanceThreshold) * 100).toFixed(0) : '72'

  return (
    <Card className="p-5 bg-gradient-to-br from-slate-950/60 via-slate-900/40 to-slate-900 border border-border-subtle">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-xs uppercase tracking-wide text-text-muted">Cross-Chain Collateral</div>
          <div className="text-lg font-semibold text-text-primary mt-1">{market.label}</div>
          <div className="text-xs text-text-muted">Deposit on Ethereum, mint to Sui automatically.</div>
        </div>
        <div className="flex items-center gap-2 text-xs text-emerald-400 bg-emerald-500/10 px-3 py-1 rounded-full">
          <CheckCircle2 className="w-4 h-4" />
          Walrus Proofs
        </div>
      </div>

      <div className="grid sm:grid-cols-[1.4fr,1fr] gap-4 mt-4">
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm text-text-secondary">Vault address</span>
            {fetching && <Loader2 className="w-4 h-4 animate-spin text-text-muted" />}
          </div>
          <div className="bg-bg-card2 border border-border-subtle rounded-lg p-3 flex items-center justify-between">
            <div className="font-mono text-sm break-all text-text-primary">
              {vault?.vaultAddress || 'Loading...'}
            </div>
            {vault?.vaultAddress && (
              <button
                onClick={() => handleCopy(vault.vaultAddress, 'Vault address')}
                className="ml-3 p-2 rounded hover:bg-bg-card"
              >
                <Copy className="w-4 h-4 text-text-secondary" />
              </button>
            )}
          </div>

          <div className="bg-bg-card2 border border-border-subtle rounded-lg p-3">
            <div className="text-sm text-text-secondary mb-1">Memo (Sui Address)</div>
            <div className="flex items-center justify-between gap-2">
              <code className="text-xs sm:text-sm text-text-primary break-all">{userAddress}</code>
              <button
                onClick={() => handleCopy(userAddress, 'Sui address')}
                className="p-2 rounded hover:bg-bg-card"
              >
                <Copy className="w-4 h-4 text-text-secondary" />
              </button>
            </div>
          </div>

          <div className="flex items-center justify-between bg-bg-card2 border border-border-subtle rounded-lg p-3">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-full bg-bg-card">
                <Wallet className="w-4 h-4 text-brand-primary" />
              </div>
              <div>
                <div className="text-sm text-text-primary">
                  {evmAccount ? `EVM wallet ${shortenAddress(evmAccount)}` : 'Connect an EVM wallet'}
                </div>
                <div className="text-xs text-text-muted">
                  {evmChainId ? `Chain ${hexToDecimal(evmChainId)} (${evmChainId})` : 'Ethereum / Sepolia preferred'}
                </div>
              </div>
            </div>
            <Button size="sm" variant="secondary" onClick={connectEvm} disabled={status === 'connecting'}>
              {status === 'connecting' ? <Loader2 className="w-4 h-4 animate-spin" /> : evmAccount ? 'Reconnect' : 'Connect'}
            </Button>
          </div>

          <div className="flex flex-col sm:flex-row gap-3">
            <Input
              placeholder={`Amount in ${asset}`}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              className="bg-bg-card2 border-border-subtle"
            />
            <Button onClick={handleDeposit} disabled={submitting || !amount}>
              {submitting ? <Loader2 className="w-4 h-4 animate-spin" /> : `Deposit & Mint ${asset}`}
            </Button>
          </div>

          <div className="text-xs text-text-muted">
            We call the vault&apos;s <code>deposit</code> function on Ethereum and forward the receipt to Walrus.
            Your {asset} position mints on Sui once the bridge sees the transaction.
          </div>

          <MintStatus
            status={status}
            suiOwner={userAddress}
            txHash={txHash}
            receipt={bridgeReceipt}
            chainHex={evmChainId}
            asset={asset}
          />
        </div>

        <div className="rounded-lg border border-border-subtle bg-bg-card2 p-4 space-y-3">
          <div>
            <div className="text-sm text-text-secondary">Risk Parameters</div>
            <div className="text-text-primary text-lg font-semibold">{ltv}% LTV</div>
            <div className="text-xs text-text-muted">Liquidation at {maintenance}% CR, 6% penalty</div>
          </div>
          <div className="space-y-2">
            {(market.collateralHighlights || []).map((item) => (
              <div
                key={item}
                className="flex items-start gap-2 text-sm text-text-primary bg-bg-card px-3 py-2 rounded border border-border-subtle/60"
              >
                <div className="w-1.5 h-10 rounded bg-emerald-500/70" />
                <span>{item}</span>
              </div>
            ))}
          </div>
          {vault?.snapshotUrl && (
            <a
              href={vault.snapshotUrl}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 text-sm text-brand-primary hover:text-brand-soft"
            >
              Latest Walrus snapshot
              <ExternalLink className="w-4 h-4" />
            </a>
          )}
        </div>
      </div>
    </Card>
  )
}

function MintStatus({
  status,
  txHash,
  receipt,
  suiOwner,
  chainHex,
  asset
}: {
  status: 'idle' | 'connecting' | 'signing' | 'submitted' | 'minting' | 'minted' | 'error'
  txHash: string | null
  receipt: BridgeReceipt | null
  suiOwner: string
  chainHex: string | null
  asset: string
}) {
  const explorer = txHash ? `${explorerByChain(chainHex)}/tx/${txHash}` : null

  const renderStatus = () => {
    switch (status) {
      case 'connecting':
        return 'Connecting wallet...'
      case 'signing':
        return 'Awaiting wallet signature'
      case 'submitted':
        return 'Waiting for Ethereum confirmation'
      case 'minting':
        return 'Publishing Walrus checkpoint & minting'
      case 'minted':
        return 'Minted to Sui'
      case 'error':
        return 'Deposit failed'
      default:
        return 'Ready to deposit'
    }
  }

  return (
    <div className="flex items-start gap-3 bg-bg-card2/70 border border-dashed border-border-subtle rounded-lg p-3">
      {status === 'minted' ? (
        <Sparkles className="w-5 h-5 mt-0.5 text-emerald-400" />
      ) : (
        <Loader2 className={`w-4 h-4 mt-0.5 ${status === 'idle' ? 'text-text-muted' : 'text-brand-primary animate-spin'}`} />
      )}
      <div className="text-sm text-text-secondary">
        <div className="text-text-primary font-medium">{renderStatus()}</div>
        {txHash && (
          <div className="mt-1">
            Tx: {explorer ? (
              <a href={explorer} target="_blank" rel="noreferrer" className="text-brand-primary inline-flex items-center gap-1">
                {shortenAddress(txHash, 6)}
                <ExternalLink className="w-3 h-3" />
              </a>
            ) : (
              <span className="font-mono text-xs text-text-primary">{shortenAddress(txHash, 6)}</span>
            )}
          </div>
        )}
        {receipt && (
          <div className="mt-1 text-text-primary">
            Minted {formatNumber(Number(receipt.minted || 0), 4)} {asset} to {shortenAddress(suiOwner, 6)}
          </div>
        )}
        {!receipt && status !== 'minted' && (
          <div className="text-xs text-text-muted mt-1">
            Watching Walrus for the next checkpoint. Your minted balance auto-updates once finalized.
          </div>
        )}
      </div>
    </div>
  )
}
