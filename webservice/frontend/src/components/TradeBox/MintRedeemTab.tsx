import { useState, useEffect, useContext, useMemo } from 'react'
import { AlertTriangle, Clock, ExternalLink } from 'lucide-react'
import { Input } from '@/components/ui/Input'
import { Button } from '@/components/ui/Button'
import { QuoteSummary } from './QuoteSummary'
import { useCurrentAccount, useSignTransaction, ConnectModal, SuiClientContext, useCurrentWallet } from '@mysten/dapp-kit'
import { useQueryClient } from '@tanstack/react-query'
import { formatNumber } from '@/lib/utils'
import { apiUrlForNetwork } from '@/utils/api'
import { useProtocolState } from '@/hooks/useProtocolState'
import { toast } from 'sonner'
import { createTxBuilder } from '@/txBuilder'
import { keccak_256 } from '@noble/hashes/sha3'
import { bytesToHex, utf8ToBytes } from '@noble/hashes/utils'
import { getTransactionBuildInfo } from '@/txBuilder/config'
import type { RedeemReceipt } from '@/types/crosschain'
type ActionType = 'mint' | 'redeem'
type TokenType = 'f' | 'x'
type CollateralMode = 'sui' | 'eth'

interface MintRedeemTabProps {
  demoMode?: boolean
  collateralMode?: CollateralMode
  suiNetwork?: 'mainnet' | 'testnet' | 'localnet'
  crossChainAsset?: string | null
  crossChainChainId?: string | null
  onCrossChainMint?: () => void
}

const DEPOSIT_SELECTOR = bytesToHex(keccak_256(utf8ToBytes('deposit(address,string,uint256)'))).slice(0, 8)

const explorerByChain = (chainId?: string | null) => {
  switch (chainId) {
    case '0x1':
      return 'https://etherscan.io'
    case '0xaa36a7':
      return 'https://sepolia.etherscan.io'
    default:
      return 'https://etherscan.io'
  }
}

const suiExplorerTx = (network?: string | null, digest?: string | null) => {
  if (!digest) return null
  const trimmed = digest.trim()
  const net =
    network === 'mainnet'
      ? 'mainnet'
      : network === 'testnet'
        ? 'testnet'
        : network === 'localnet'
          ? 'localnet'
          : null
  switch (net) {
    case 'mainnet':
      return `https://suivision.xyz/txblock/${trimmed}`
    case 'testnet':
      return `https://testnet.suivision.xyz/txblock/${trimmed}`
    case 'localnet':
      return null
    default:
      return `https://suivision.xyz/txblock/${trimmed}`
  }
}

function normalizeDigestList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.filter((d): d is string => typeof d === 'string' && d.trim().length > 0)
  }
  if (typeof value === 'string' && value.trim().length > 0) {
    return [value.trim()]
  }
  return []
}

function extractMintDigests(raw: unknown): string[] {
  const scopes = [(raw as any)?.receipt, (raw as any)?.Receipt, raw]
  for (const scope of scopes) {
    if (!scope || typeof scope !== 'object') continue
    const candidates = [
      (scope as any).suiTxDigests,
      (scope as any).suiTxDigest,
      (scope as any).mintTxDigests,
      (scope as any).mintTxDigest,
      (scope as any).txDigests,
      (scope as any).txDigest
    ]
    for (const candidate of candidates) {
      const digests = normalizeDigestList(candidate)
      if (digests.length) return digests
    }
  }
  return []
}

function getEvmChainForNetwork(network?: string | null, override?: { chainId?: string | null; rpcUrl?: string | null }) {
  // Default to Sepolia outside of mainnet to avoid accidental mainnet sends in lower environments.
  if (network === 'mainnet') {
    const rpc = override?.rpcUrl || 'https://rpc.ankr.com/eth'
    const chainId = override?.chainId || '0x1'
    return {
      chainId,
      label: 'Ethereum Mainnet',
      addParams: {
        chainId,
        chainName: 'Ethereum Mainnet',
        nativeCurrency: { name: 'Ether', symbol: 'ETH', decimals: 18 },
        rpcUrls: [rpc],
        blockExplorerUrls: ['https://etherscan.io']
      }
    }
  }
  if (network === 'testnet') {
    const chainId = override?.chainId || '0xaa36a7'
    const rpc = override?.rpcUrl || 'https://rpc.sepolia.org'
    return {
      chainId,
      label: 'Sepolia',
      addParams: {
        chainId,
        chainName: 'Sepolia',
        nativeCurrency: { name: 'Sepolia Ether', symbol: 'ETH', decimals: 18 },
        rpcUrls: [rpc],
        blockExplorerUrls: ['https://sepolia.etherscan.io']
      }
    }
  }
  const chainId = override?.chainId || '0xaa36a7'
  const rpc = override?.rpcUrl || 'https://rpc.sepolia.org'
  return {
    chainId,
    label: 'Sepolia',
    addParams: {
      chainId,
      chainName: 'Sepolia',
      nativeCurrency: { name: 'Sepolia Ether', symbol: 'ETH', decimals: 18 },
      rpcUrls: [rpc],
      blockExplorerUrls: ['https://sepolia.etherscan.io']
    }
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
  const headOffset = pad32((32 * 3).toString(16))
  const headMinShares = pad32(minShares.toString(16))
  const ownerLengthWord = pad32(ownerLength.toString(16))

  return `0x${selector}${headRecipient}${headOffset}${headMinShares}${ownerLengthWord}${ownerPadded}`
}

function shortenAddress(address: string, size = 4) {
  if (!address) return ''
  return `${address.slice(0, 2 + size)}...${address.slice(-size)}`
}

function isValidEthAddress(addr: string) {
  return /^0x[a-fA-F0-9]{40}$/.test(addr.trim())
}

function getEvmProvider(preferredWalletName?: string): any | null {
  if (typeof window === 'undefined') return null
  const w = window as any
  const eth = w.ethereum
  const candidates: any[] = []
  const normalizedPreference = preferredWalletName?.toLowerCase()
  const pushUnique = (prov: any) => {
    if (prov && !candidates.includes(prov)) candidates.push(prov)
  }
  const matchesPreference = (prov: any) => {
    if (!normalizedPreference || !prov) return false
    const names = [
      prov.name,
      prov.walletName,
      prov.id,
      prov.info?.name,
      prov.info?.label
    ]
      .filter(Boolean)
      .map((n: string) => n.toLowerCase())

    if (prov.isMetaMask) names.push('metamask')
    if (prov.isBraveWallet) names.push('brave')
    if (prov.isCoinbaseWallet) names.push('coinbase')
    if (prov.isNightly || prov.isNightlyWallet) names.push('nightly')

    return names.some((n: string) => normalizedPreference.includes(n))
  }

  if (normalizedPreference?.includes('nightly')) {
    pushUnique(w.nightly?.ethereum)
    pushUnique(w.nightly)
  }

  if (eth?.selectedProvider) pushUnique(eth.selectedProvider)

  if (eth?.providers?.length) {
    const preferred = eth.providers.find((p: any) => matchesPreference(p))
    if (preferred) pushUnique(preferred)
    eth.providers.forEach((p: any) => pushUnique(p))
  }

  if (eth) pushUnique(eth)

  // Nightly and some multi-chain wallets expose an ethereum provider separately.
  if (!normalizedPreference?.includes('nightly')) {
    pushUnique(w.nightly?.ethereum)
    pushUnique(w.nightly)
  }

  const withRequest = candidates.find((c) => c && typeof c.request === 'function')
  return withRequest || null
}

export function MintRedeemTab({
  demoMode: _demoMode = false,
  collateralMode = 'sui',
  suiNetwork,
  crossChainAsset,
  crossChainChainId,
  onCrossChainMint
}: MintRedeemTabProps) {
  const [actionType, setActionType] = useState<ActionType>('mint')
  const [tokenType, setTokenType] = useState<TokenType>('f')
  const [inputAmount, setInputAmount] = useState('')
  const [outputAmount, setOutputAmount] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [quoteExpiry, setQuoteExpiry] = useState<Date | null>(null)
  const [connectModalOpen, setConnectModalOpen] = useState(false)
  const [suiRecipient, setSuiRecipient] = useState('')
  const [ethRecipient, setEthRecipient] = useState('')
  const [evmAccount, setEvmAccount] = useState<string | null>(null)
  const [evmChainId, setEvmChainId] = useState<string | null>(null)
  const [evmTxHash, setEvmTxHash] = useState<string | null>(null)
  const [suiRedeemDigest, setSuiRedeemDigest] = useState<string | null>(null)
  const [redeemReceipt, setRedeemReceipt] = useState<RedeemReceipt | null>(null)
  const [suiMintDigests, setSuiMintDigests] = useState<string[]>([])
  const [evmStatus, setEvmStatus] = useState<'idle' | 'connecting' | 'signing' | 'submitted' | 'minting' | 'minted' | 'error'>('idle')
  const [redeemStatus, setRedeemStatus] = useState<'idle' | 'building' | 'signing' | 'submitting' | 'processing' | 'paid' | 'error'>('idle')
  const [evmRpcUrl, setEvmRpcUrl] = useState<string | null>(null)
  const [evmExpectedChainId, setEvmExpectedChainId] = useState<string | null>(null)

  const currentAccount = useCurrentAccount()
  const { currentWallet: connectedSuiWallet } = useCurrentWallet()
  const { mutateAsync: signTransaction } = useSignTransaction()
  const queryClient = useQueryClient()
  const isConnected = !!currentAccount
  const ctx = useContext(SuiClientContext)
  const walletFeatures = connectedSuiWallet?.features as Record<string, unknown> | undefined
  const walletSupportsSuiChain = connectedSuiWallet?.chains?.some((c) => c.startsWith('sui:')) ?? false
  const walletSupportsSuiTx =
    !!(
      walletFeatures?.['sui:signTransaction'] ||
      walletFeatures?.['sui:signTransactionBlock'] ||
      walletFeatures?.['sui:signAndExecuteTransaction'] ||
      walletFeatures?.['sui:signAndExecuteTransactionBlock']
    )

  const isEthMode = collateralMode === 'eth'
  const baseAsset = crossChainAsset || 'ETH'

  useEffect(() => {
    if (currentAccount?.address) {
      setSuiRecipient(currentAccount.address)
    }
  }, [currentAccount?.address])

  useEffect(() => {
    if (evmAccount && !ethRecipient) {
      setEthRecipient(evmAccount)
    }
  }, [evmAccount, ethRecipient])

  useEffect(() => {
    const eth = typeof window !== 'undefined' ? (window as any).ethereum : undefined
    if (!eth?.on) return

    const handleAccounts = (accounts: string[]) => setEvmAccount(accounts[0] || null)
    const handleChain = (chain: string) => setEvmChainId(chain)

    eth.on('accountsChanged', handleAccounts)
    eth.on('chainChanged', handleChain)

    return () => {
      eth.removeListener?.('accountsChanged', handleAccounts)
      eth.removeListener?.('chainChanged', handleChain)
    }
  }, [])

  if (!ctx) {
    throw new Error('MintRedeemTab must be used within SuiClientProvider')
  }

  const { client, network: ctxNetwork } = ctx
  const network = suiNetwork || ctxNetwork
  const chain = network === 'mainnet' ? 'sui:mainnet' : network === 'testnet' ? 'sui:testnet' : 'sui:localnet'
  const expectedEvmChain = useMemo(
    () => getEvmChainForNetwork(network, { chainId: evmExpectedChainId, rpcUrl: evmRpcUrl }),
    [network, evmExpectedChainId, evmRpcUrl]
  )
  const { data: protocolState, isLoading: isPxLoading } = useProtocolState()
  const rawPx = protocolState?.px

  // Create transaction builder instance
  const txBuilder = useMemo(() => createTxBuilder(client as any, network), [client, network])

  // Load build-info once to get EVM RPC/chain hints from backend
  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const info = await getTransactionBuildInfo(network)
        if (!mounted) return
        if (info?.evmRpcUrl) setEvmRpcUrl(info.evmRpcUrl)
        if (info?.evmChainId) setEvmExpectedChainId(info.evmChainId)
      } catch (err) {
        console.warn('Failed to load build info for EVM config', err)
      }
    })()
    return () => {
      mounted = false
    }
  }, [network])

  useEffect(() => {
    setEvmStatus('idle')
    setRedeemStatus('idle')
    setEvmTxHash(null)
    setSuiMintDigests([])
    setRedeemReceipt(null)
    setSuiRedeemDigest(null)
  }, [actionType, tokenType, collateralMode])

  const connectEvm = async () => {
    const eth = getEvmProvider(connectedSuiWallet?.name)
    if (!eth) {
      toast.error('No EVM wallet detected. Please open an EVM wallet (e.g., MetaMask or Nightly EVM) to continue.')
      return null
    }
    setEvmStatus('connecting')
    try {
      const accounts = await eth.request({ method: 'eth_requestAccounts' })
      const chainHex = await ensureCorrectEvmNetwork(eth)
      setEvmAccount(accounts[0] || null)
      setEvmChainId(chainHex)
      setEvmStatus('idle')
      return accounts[0] as string
    } catch (error) {
      console.warn('Failed to connect wallet', error)
      setEvmStatus('idle')
      toast.error('Wallet connection rejected')
      return null
    }
  }

  const ensureCorrectEvmNetwork = async (eth: any) => {
    const currentChain = await eth.request({ method: 'eth_chainId' })
    const normalizedCurrent = (currentChain || '').toLowerCase()
    const normalizedExpected = expectedEvmChain.chainId.toLowerCase()

    if (normalizedCurrent === normalizedExpected) {
      setEvmChainId(currentChain)
      return currentChain as string
    }

    try {
      await eth.request({
        method: 'wallet_switchEthereumChain',
        params: [{ chainId: expectedEvmChain.chainId }]
      })
      setEvmChainId(expectedEvmChain.chainId)
      return expectedEvmChain.chainId
    } catch (error: any) {
      // If chain is missing, try to add it before retrying the switch.
      if (error?.code === 4902 && expectedEvmChain.addParams) {
        try {
          await eth.request({
            method: 'wallet_addEthereumChain',
            params: [expectedEvmChain.addParams]
          })
          await eth.request({
            method: 'wallet_switchEthereumChain',
            params: [{ chainId: expectedEvmChain.chainId }]
          })
          setEvmChainId(expectedEvmChain.chainId)
          return expectedEvmChain.chainId
        } catch (addErr) {
          console.warn('EVM network add/switch failed', addErr)
          throw new Error(`Add and switch your EVM wallet to ${expectedEvmChain.label} to deposit`)
        }
      }

      console.warn('EVM network switch failed', error)
      throw new Error(`Switch your EVM wallet to ${expectedEvmChain.label} to deposit`)
    }
  }

  const waitForReceipt = async (hash: string) => {
    const eth = getEvmProvider(connectedSuiWallet?.name)
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

  // Helper functions
  const getInputToken = () => {
    if (isEthMode) {
      return actionType === 'mint' ? baseAsset : tokenType === 'f' ? `f${baseAsset}` : `x${baseAsset}`
    }
    return actionType === 'mint' ? 'Sui' : (tokenType === 'f' ? 'fToken' : 'xToken')
  }
  const getOutputToken = () => {
    if (isEthMode) {
      return actionType === 'mint' ? (tokenType === 'f' ? `f${baseAsset}` : `x${baseAsset}`) : baseAsset
    }
    return actionType === 'mint' ? (tokenType === 'f' ? 'fToken' : 'xToken') : 'Sui'
  }
  const getSelectedTokenLabel = () => {
    const prefix = tokenType === 'f' ? 'f' : 'x'
    return isEthMode ? `${prefix}${baseAsset}` : `${prefix}Token`
  }

  // Debounced quote fetching with AbortController
  useEffect(() => {
    if (!inputAmount || isNaN(Number(inputAmount)) || Number(inputAmount) <= 0) {
      setOutputAmount('')
      setQuoteExpiry(null)
      return
    }

    if (isEthMode) {
      if (actionType === 'mint') {
        setOutputAmount(inputAmount)
      } else {
        const parsed = Number(inputAmount)
        if (!Number.isFinite(parsed) || parsed <= 0) {
          setOutputAmount('')
          return
        }
        const ethPriceUsd = rawPx ? rawPx / 1_000_000 : null
        if (tokenType === 'f' && ethPriceUsd && ethPriceUsd > 0) {
          setOutputAmount((parsed / ethPriceUsd).toFixed(6))
        } else {
          setOutputAmount(parsed.toFixed(6))
        }
      }
      setQuoteExpiry(null)
      return
    }

    const abortController = new AbortController()
    const timeoutId = setTimeout(async () => {
      try {
        let endpoint = ''
        let outputField = ''
        
        // Map action and token combination to correct endpoint
        if (actionType === 'mint' && tokenType === 'f') {
          endpoint = `/v1/quotes/mintF?amountR=${inputAmount}`
          outputField = 'fOut'
        } else if (actionType === 'redeem' && tokenType === 'f') {
          endpoint = `/v1/quotes/redeemF?amountF=${inputAmount}`
          outputField = 'rOut'
        } else if (actionType === 'mint' && tokenType === 'x') {
          endpoint = `/v1/quotes/mintX?amountR=${inputAmount}`
          outputField = 'xOut'
        } else if (actionType === 'redeem' && tokenType === 'x') {
          endpoint = `/v1/quotes/redeemX?amountX=${inputAmount}`
          outputField = 'rOut'
        }

        const response = await fetch(apiUrlForNetwork(endpoint, network), {
          signal: abortController.signal
        })

        if (!response.ok) {
          console.warn('Quote fetch failed:', response.status, response.statusText)
          return
        }

        const data = await response.json()
        
        // Set output amount with 6 decimal precision
        setOutputAmount(Number(data[outputField]).toFixed(6))
        
        // Set quote expiry using asOf + ttlSec if present
        if (data.asOf && data.ttlSec) {
          const expiry = data.asOf * 1000 + data.ttlSec * 1000
          setQuoteExpiry(new Date(expiry))
        }

      } catch (error) {
        if (error instanceof Error && error.name === 'AbortError') {
          return // Request was aborted, ignore
        }
        console.warn('Quote fetch error:', error)
        toast.error('Failed to fetch quote')
      }
    }, 300)

    return () => {
      clearTimeout(timeoutId)
      abortController.abort()
    }
  }, [inputAmount, actionType, tokenType, isEthMode, rawPx, network])

  const currentCR = 1.45
  const minCR = 1.20

  // Mock token balances
  const tokenBalances = {
    Sui: 2100.88,
    fToken: 1500.25,
    xToken: 850.50
  }

  const getTokenBalance = (token: string) => {
    return tokenBalances[token as keyof typeof tokenBalances] || 0
  }

  const handleMaxClick = () => {
    const balance = getTokenBalance(getInputToken())
    setInputAmount(balance.toString())
  }
  
  // Mock calculation of post-transaction CR
  const postTxCR = actionType === 'mint' 
    ? currentCR - 0.02 // Minting decreases CR
    : currentCR + 0.01 // Redeeming increases CR
  
  const isBreachingMin = !isEthMode && postTxCR < minCR
  const quoteTTL = isEthMode ? null : (quoteExpiry ? Math.max(0, Math.floor((quoteExpiry.getTime() - Date.now()) / 1000)) : 0)
  const isQuoteStale = !isEthMode && quoteTTL === 0 && outputAmount

  // Check if input amount exceeds balance
  const inputBalance = getTokenBalance(getInputToken())
  const isInsufficientBalance = !isEthMode && inputAmount && Number(inputAmount) > inputBalance
  const selectedTokenLabel = getSelectedTokenLabel()
  const formattedXPriceUsd = typeof rawPx === 'number' ? (rawPx / 1_000_000).toFixed(4) : null
  const priceLabel =
    tokenType === 'f'
      ? `1 ${selectedTokenLabel} = 1.00 USD`
      : rawPx === undefined && isPxLoading
        ? 'Loading price...'
        : formattedXPriceUsd
          ? `1 ${selectedTokenLabel} = ${formattedXPriceUsd} USD`
          : `1 ${selectedTokenLabel} = -- USD`
  
  const canExecute = isEthMode
    ? actionType === 'mint'
      ? Boolean(inputAmount && suiRecipient && !isLoading)
      : Boolean(inputAmount && currentAccount && isValidEthAddress(ethRecipient) && !isLoading)
    : Boolean(isConnected && currentAccount && inputAmount && outputAmount && !isBreachingMin && !isQuoteStale && !isLoading && !isInsufficientBalance)

  const handleEvmDeposit = async () => {
    if (!inputAmount) return
    if (!suiRecipient) {
      toast.error('Add the Sui recipient address for the minted tokens.')
      return
    }
    const eth = getEvmProvider(connectedSuiWallet?.name)
    const account = evmAccount || (await connectEvm())
    if (!eth || !account) return

    let weiAmount: bigint
    try {
      weiAmount = parseUnits(inputAmount, 18)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Invalid amount')
      return
    }

    setIsLoading(true)
    setEvmStatus('connecting')
    setEvmTxHash(null)
    setSuiMintDigests([])

    try {
      await ensureCorrectEvmNetwork(eth)
      setEvmStatus('signing')

      const chainTarget = crossChainChainId || 'ethereum'
      const asset = crossChainAsset || baseAsset
      const vaultRes = await fetch(apiUrlForNetwork(`/v1/crosschain/vault?chainId=${chainTarget}&asset=${asset}`, network))
      const vaultJson = vaultRes.ok ? await vaultRes.json() : null
      const vaultAddress =
        vaultJson?.vault?.vaultAddress ||
        vaultJson?.Vault?.vaultAddress ||
        vaultJson?.vaultAddress ||
        vaultJson?.VaultAddress

      if (!vaultAddress) {
        throw new Error('Vault address unavailable')
      }

      const calldata = encodeDepositCalldata(account, suiRecipient || '', 0n)
      const hash: string = await eth.request({
        method: 'eth_sendTransaction',
        params: [
          {
            from: account,
            to: vaultAddress,
            value: `0x${weiAmount.toString(16)}`,
            data: calldata
          }
        ]
      })

      setEvmTxHash(hash)
      setEvmStatus('submitted')
      toast.success('Ethereum transaction submitted')

      const receipt = await waitForReceipt(hash)
      if (receipt && receipt.status === '0x0') {
        throw new Error('Ethereum transaction failed')
      }

      setEvmStatus('minting')
      const bridgeRes = await fetch(apiUrlForNetwork('/v1/crosschain/deposit', network), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          txHash: hash,
          suiOwner: suiRecipient || currentAccount?.address,
          chainId: chainTarget,
          asset,
          amount: inputAmount
        })
      })
      if (!bridgeRes.ok) {
        const msg = await bridgeRes.text()
        throw new Error(msg || 'Bridge mint failed')
      }
      const bridgeJson = await bridgeRes.json().catch(() => null)
      const bridgeReceipt = bridgeJson?.receipt || bridgeJson?.Receipt || bridgeJson
      const digests = extractMintDigests(bridgeReceipt || bridgeJson)
      if (digests.length) setSuiMintDigests(digests)
      setEvmStatus('minted')
      onCrossChainMint?.()
      toast.success(`Deposited ${inputAmount} ${baseAsset}. Mint will arrive on Sui shortly.`)
      setInputAmount('')
      setOutputAmount('')
    } catch (error) {
      console.error('EVM deposit failed', error)
      setEvmStatus('error')
      toast.error(error instanceof Error ? error.message : 'Deposit failed')
    } finally {
      setIsLoading(false)
    }
  }

  const handleRedeemToEth = async () => {
    if (!inputAmount) return
    if (!currentAccount) {
      setConnectModalOpen(true)
      toast.error('Connect a Sui wallet to redeem.')
      return
    }
    if (!isValidEthAddress(ethRecipient)) {
      toast.error('Enter a valid Ethereum recipient address')
      return
    }

    setIsLoading(true)
    setRedeemStatus('building')
    setRedeemReceipt(null)
    setSuiRedeemDigest(null)

    try {
      const { transactionBlockBytes } = await txBuilder.bridgeRedeem({
        tokenType,
        amount: inputAmount,
        userAddress: currentAccount.address,
        ethRecipient,
        chain
      })

      setRedeemStatus('signing')
      const signResult = await signTransaction({
        transaction: transactionBlockBytes,
        chain
      })

      const { bytes, signature } = signResult

      setRedeemStatus('submitting')
      const submitResponse = await fetch(apiUrlForNetwork('/v1/transactions/submit', network), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          tx_bytes: bytes,
          signature
        })
      })

      if (!submitResponse.ok) {
        const errorText = await submitResponse.text()
        throw new Error(errorText || 'Failed to submit redemption transaction')
      }

      const submitResult = await submitResponse.json().catch(() => ({}))
      const digest =
        submitResult.transactionDigest ||
        submitResult.TransactionDigest ||
        submitResult.txDigest ||
        submitResult.digest
      if (!digest) {
        throw new Error('Missing transaction digest from Sui submit')
      }
      setSuiRedeemDigest(digest)
      setRedeemStatus('processing')

      const redeemRes = await fetch(apiUrlForNetwork('/v1/crosschain/redeem', network), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          suiTxDigest: digest,
          suiOwner: currentAccount.address,
          ethRecipient,
          chainId: crossChainChainId || 'ethereum',
          asset: crossChainAsset || baseAsset,
          token: tokenType,
          amount: inputAmount
        })
      })

      if (!redeemRes.ok) {
        const msg = await redeemRes.text()
        throw new Error(msg || 'Bridge redeem failed')
      }

      const redeemJson = await redeemRes.json().catch(() => null)
      const receipt: RedeemReceipt = redeemJson?.receipt || redeemJson?.Receipt || redeemJson
      setRedeemReceipt(receipt)
      setRedeemStatus('paid')
      onCrossChainMint?.()
      toast.success(`Redeem submitted. Payout will finalize on ${expectedEvmChain.label}.`)
      setInputAmount('')
      setOutputAmount('')
    } catch (error) {
      console.error('Bridge redeem failed', error)
      setRedeemStatus('error')
      toast.error(error instanceof Error ? error.message : 'Redeem failed')
    } finally {
      setIsLoading(false)
    }
  }

  const handleExecute = async () => {
    if (isEthMode) {
      if (actionType === 'mint') {
        await handleEvmDeposit()
        return
      }
      await handleRedeemToEth()
      return
    }

    if (!canExecute || !currentAccount) return
    if (!walletSupportsSuiChain || !walletSupportsSuiTx) {
      toast.error('Your connected wallet does not support Sui transactions. Please switch to a Sui-capable wallet using the top-right Connect Wallet button.')
      return
    }

    setIsLoading(true)
    try {
      const { transactionBlockBytes, quoteId } = await txBuilder.buildMintRedeem({
        action: actionType,
        tokenType,
        amount: inputAmount,
        userAddress: currentAccount.address,
        chain
      })

      let signResult
      try {
        signResult = await signTransaction({
          transaction: transactionBlockBytes,
          chain
        })
      } catch (error) {
        console.error('Transaction signing error:', error)
        toast.error('Transaction signing failed')
        setIsLoading(false)
        return
      }

      const { bytes, signature } = signResult

      try {
        const submitResponse = await fetch(apiUrlForNetwork('/v1/transactions/submit', network), {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({
            tx_bytes: bytes,
            signature: signature,
            quoteId: quoteId
          })
        })

        if (!submitResponse.ok) {
          const error = await submitResponse.json()
          throw new Error(error.message || 'Failed to submit transaction')
        }

        const submitResult = await submitResponse.json()
        toast.success(`${getActionLabel()} completed successfully! Tx: ${submitResult.transactionDigest.slice(0, 8)}...`)

        setTimeout(() => {
          queryClient.invalidateQueries({ queryKey: ['userBalances'] })
        }, 100)

        setInputAmount('')
        setOutputAmount('')
      } catch (error) {
        console.error('Transaction submission error:', error)
        toast.error(`Transaction submission failed: ${error instanceof Error ? error.message : 'Unknown error'}`)
      } finally {
        setIsLoading(false)
      }
    } catch (error) {
      console.error('Transaction error:', error)
      toast.error(`${getActionLabel()} failed: ${error instanceof Error ? error.message : 'Unknown error'}`)
      setIsLoading(false)
    }
  }

  const getActionLabel = () => {
    const tokenName = tokenType === 'f' ? 'fToken' : 'xToken'
    return `${actionType === 'mint' ? 'Mint' : 'Redeem'} ${tokenName}`
  }

  return (
    <div className="space-y-4">
      {/* Action Selector */}
      <div className="space-y-3">
        <div>
          <label className="text-text-muted text-sm mb-2 block">Action</label>
          <div className="grid grid-cols-2 gap-2 p-1 bg-bg-input rounded-xl">
            <button
              onClick={() => setActionType('mint')}
              className={`px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                actionType === 'mint'
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'text-text-secondary hover:text-text-primary'
              }`}
              aria-label="Select mint action"
            >
              Mint
            </button>
            <button
              onClick={() => setActionType('redeem')}
              className={`px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                actionType === 'redeem'
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'text-text-secondary hover:text-text-primary'
              }`}
              aria-label="Select redeem action"
            >
              Redeem
            </button>
          </div>
        </div>

        {/* Token Selector */}
        <div>
          <label className="text-text-muted text-sm mb-2 block">Token</label>
          <div className="grid grid-cols-2 gap-2 p-1 bg-bg-input rounded-xl">
            <button
              onClick={() => setTokenType('f')}
              className={`px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                tokenType === 'f'
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'text-text-secondary hover:text-text-primary'
              }`}
              aria-label="Select fToken"
            >
              fToken
            </button>
            <button
              onClick={() => setTokenType('x')}
              className={`px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                tokenType === 'x'
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'text-text-secondary hover:text-text-primary'
              }`}
              aria-label="Select xToken"
            >
              xToken
            </button>
          </div>
        </div>
      </div>

      {/* Beta=0 Indicator for fToken */}
      {tokenType === 'f' && (
        <div className="bg-info/10 border border-info/20 rounded-xl p-3">
          <div className="flex items-center gap-2 text-info text-sm font-medium">
            <div className="w-2 h-2 bg-info rounded-full" />
            Stable fToken (β = 0)
          </div>
          <div className="text-text-muted text-xs mt-1">
            fToken maintains stable value around $1.00 through protocol mechanisms
          </div>
        </div>
      )}

      {isEthMode && actionType === 'mint' && (
        <div className="bg-bg-card2 border border-border-subtle rounded-xl p-3 space-y-2">
          <div>
            <div className="text-sm text-text-primary font-semibold">Recipient on Sui</div>
            <div className="text-xs text-text-muted">Bridge will mint to this address after your ETH deposit.</div>
          </div>
          <Input
            placeholder="0x... Sui address"
            value={suiRecipient}
            onChange={(e) => setSuiRecipient(e.target.value)}
            className="bg-bg-card"
          />
          <div className="flex items-center justify-between text-xs text-text-muted">
            <div>
              {evmAccount
                ? `EVM wallet ${shortenAddress(evmAccount, 4)} · Chain ${evmChainId || ''}`
                : 'Your wallet will be requested for EVM signing when you deposit'}
            </div>
            {evmStatus === 'connecting' && <div className="text-text-primary text-[11px]">Connecting...</div>}
          </div>
          {evmTxHash && (
            <div className="text-xs text-text-primary flex items-center gap-2">
              Tx:{' '}
              <a
                href={`${explorerByChain(evmChainId)}/tx/${evmTxHash}`}
                target="_blank"
                rel="noreferrer"
                className="text-brand-primary inline-flex items-center gap-1"
              >
                {shortenAddress(evmTxHash, 6)}
                <ExternalLink className="w-3 h-3" />
              </a>
            </div>
          )}
          {evmStatus !== 'idle' && (
            <div className="text-xs text-text-muted">
              Status: {evmStatus === 'submitted' ? 'Waiting for confirmation' : evmStatus}
            </div>
          )}
          {evmStatus === 'minted' && suiMintDigests.length > 0 && (
            <div className="text-xs text-text-primary space-y-1">
              {suiMintDigests.map((digest, idx) => {
                const url = suiExplorerTx(network, digest)
                const label = suiMintDigests.length > 1 ? `Sui mint tx ${idx + 1}:` : 'Sui mint tx:'
                return (
                  <div key={`${digest}-${idx}`} className="flex items-center gap-1">
                    <span>{label}</span>
                    {url ? (
                      <a href={url} target="_blank" rel="noreferrer" className="text-brand-primary inline-flex items-center gap-1">
                        {shortenAddress(digest, 6)}
                        <ExternalLink className="w-3 h-3" />
                      </a>
                    ) : (
                      <span className="font-mono text-[11px] text-text-muted">{shortenAddress(digest, 6)}</span>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>
      )}

      {isEthMode && actionType === 'redeem' && (
        <div className="bg-bg-card2 border border-border-subtle rounded-xl p-3 space-y-2">
          <div>
            <div className="text-sm text-text-primary font-semibold">Redeem to Ethereum</div>
            <div className="text-xs text-text-muted">
              Burn {getInputToken()} on Sui and receive {baseAsset} on {expectedEvmChain.label}.
            </div>
          </div>
          <Input
            placeholder="0x... ETH recipient"
            value={ethRecipient}
            onChange={(e) => setEthRecipient(e.target.value)}
            className="bg-bg-card"
          />
          <div className="flex items-center justify-between text-xs text-text-muted">
            <div>
              {currentAccount
                ? `Sui signer ${shortenAddress(currentAccount.address, 4)}`
                : 'Connect a Sui wallet to sign the burn'}
            </div>
            {redeemStatus === 'signing' && <div className="text-text-primary text-[11px]">Awaiting signature...</div>}
          </div>
          {!currentAccount && (
            <ConnectModal
              trigger={
                <Button size="sm" variant="secondary">
                  Connect Sui Wallet
                </Button>
              }
              open={connectModalOpen}
              onOpenChange={setConnectModalOpen}
            />
          )}
          {suiRedeemDigest && (
            <div className="text-xs text-text-primary flex items-center gap-2">
              Burn tx:{' '}
              {suiExplorerTx(network, suiRedeemDigest) ? (
                <a
                  href={suiExplorerTx(network, suiRedeemDigest) || undefined}
                  target="_blank"
                  rel="noreferrer"
                  className="text-brand-primary inline-flex items-center gap-1"
                >
                  {shortenAddress(suiRedeemDigest, 6)}
                  <ExternalLink className="w-3 h-3" />
                </a>
              ) : (
                <span className="font-mono text-[11px] text-text-muted">{shortenAddress(suiRedeemDigest, 6)}</span>
              )}
            </div>
          )}
          {redeemStatus !== 'idle' && (
            <div className="text-xs text-text-muted">
              Status:{' '}
              {{
                building: 'Preparing transaction',
                signing: 'Awaiting signature',
                submitting: 'Submitting to Sui',
                processing: 'Bridge payout in progress',
                paid: 'Payout sent',
                error: 'Redeem failed',
              }[redeemStatus] || 'Ready'}
            </div>
          )}
          {redeemReceipt?.payoutEth && (
            <div className="text-xs text-text-primary">
              Payout: {Number(redeemReceipt.payoutEth).toFixed(6)} {baseAsset}
            </div>
          )}
          {redeemReceipt?.payoutTxHash && (
            <div className="text-xs text-text-primary flex items-center gap-2">
              Payout tx:{' '}
              <a
                href={`${explorerByChain(evmChainId || expectedEvmChain.chainId)}/tx/${redeemReceipt.payoutTxHash}`}
                target="_blank"
                rel="noreferrer"
                className="text-brand-primary inline-flex items-center gap-1"
              >
                {shortenAddress(redeemReceipt.payoutTxHash, 6)}
                <ExternalLink className="w-3 h-3" />
              </a>
            </div>
          )}
        </div>
      )}


      {/* Input Amount */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-text-muted text-sm">
            Amount ({getInputToken()})
          </label>
        </div>
        <div className="relative">
          <Input
            type="number"
            placeholder="0.0"
            value={inputAmount}
            onChange={(e) => setInputAmount(e.target.value)}
            className="pr-20"
            aria-label={`Amount (${getInputToken()})`}
            data-testid="mintredeem-input-amount"
          />
          <div className="absolute right-0 top-0 h-full flex items-center">
            <button
              onClick={handleMaxClick}
              className="px-2 py-1 mr-2 text-xs bg-brand-primary/10 text-brand-primary rounded hover:bg-brand-primary/20 transition-colors"
            >
              Max
            </button>
            <div className="px-3 text-text-muted text-sm border-l border-border-weak">
              {getInputToken()}
            </div>
          </div>
        </div>
      </div>

      {/* Output Amount */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-text-muted text-sm">
            {actionType === 'mint' ? 'Receive' : 'Get back'} ({getOutputToken()})
          </label>
          {quoteTTL !== null && quoteTTL > 0 && (
            <div className="flex items-center gap-1 text-text-muted text-xs">
              <Clock className="w-3 h-3" />
              Quote expires in {quoteTTL}s
            </div>
          )}
        </div>
        <div className="relative">
          <Input
            type="number"
            placeholder="0.0"
            value={outputAmount}
            readOnly
            className="bg-bg-card2/50 pr-16"
            data-testid="mintredeem-output-amount"
          />
          <div className="absolute right-0 top-0 h-full flex items-center">
            <div className="px-3 text-text-muted text-sm border-l border-border-weak">
              {getOutputToken()}
            </div>
          </div>
        </div>
      </div>

      {/* Quote Summary */}
      {inputAmount && outputAmount && (
        <QuoteSummary
          rate={priceLabel}
          fee="0.5%"
          postTxCR={postTxCR.toFixed(3)}
        />
      )}

      {/* CR Breach Warning */}
      {isBreachingMin && (
        <div className="bg-danger/10 border border-danger/20 rounded-xl p-3">
          <div className="flex items-center gap-2 text-danger">
            <AlertTriangle className="w-4 h-4" />
            <span className="font-medium">CR Breach Risk</span>
          </div>
          <div className="text-text-muted text-sm mt-1">
            This transaction would reduce CR to {postTxCR.toFixed(3)}, below minimum {minCR.toFixed(2)}
          </div>
        </div>
      )}

      {/* Stale Quote Warning */}
      {isQuoteStale && (
        <div className="bg-warn/10 border border-warn/20 rounded-xl p-3">
          <div className="flex items-center gap-2 text-warn">
            <Clock className="w-4 h-4" />
            <span className="font-medium">Quote Expired</span>
          </div>
          <div className="text-text-muted text-sm mt-1">
            Price quote has expired. Refresh to get current rates.
          </div>
        </div>
      )}

      {/* Insufficient Balance Warning */}
      {isInsufficientBalance && (
        <div className="bg-danger/10 border border-danger/20 rounded-xl p-3">
          <div className="flex items-center gap-2 text-danger">
            <AlertTriangle className="w-4 h-4" />
            <span className="font-medium">Insufficient Balance</span>
          </div>
          <div className="text-text-muted text-sm mt-1">
            You need {formatNumber(Number(inputAmount) - inputBalance)} more {getInputToken()} to complete this transaction.
          </div>
        </div>
      )}

      {/* Execute Button */}
      {isEthMode ? (
        <Button
          onClick={handleExecute}
          disabled={!canExecute}
          className="w-full"
        >
          {isLoading
            ? actionType === 'mint'
              ? 'Depositing...'
              : 'Redeeming...'
            : actionType === 'mint'
              ? `Deposit ${baseAsset} & Mint`
              : 'Redeem to Ethereum'}
        </Button>
      ) : !isConnected ? (
        <ConnectModal
          trigger={
            <Button className="w-full">
              Connect Wallet to {getActionLabel()}
            </Button>
          }
          open={connectModalOpen}
          onOpenChange={setConnectModalOpen}
        />
      ) : isQuoteStale ? (
        <Button 
          onClick={() => {
            setInputAmount(inputAmount) // Trigger quote refresh
          }}
          variant="secondary"
          className="w-full"
        >
          Refresh Quote
        </Button>
      ) : (
        <Button
          onClick={handleExecute}
          disabled={!canExecute}
          className="w-full"
        >
          {isLoading ? `${getActionLabel()}...` : getActionLabel()}
        </Button>
      )}
    </div>
  )
}
