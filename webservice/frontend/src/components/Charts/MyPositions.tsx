import { useState, useEffect } from 'react'
import { ExternalLink } from 'lucide-react'
import { formatNumber, formatTokenAmount, truncateAddress } from '@/lib/utils'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { useUserBalances } from '@/hooks/useUserBalances'
import { useSpUser } from '@/hooks/useSpUser'
import { useUserTransactions } from '@/hooks/useUserTransactions'

const DECIMALS = 9 // Token decimals

interface TransactionRowProps {
  hash: string
  type: string
  amount: string
  timestamp: string
  status: 'success' | 'pending' | 'failed'
}

function TransactionRow({ hash, type, amount, timestamp, status }: TransactionRowProps) {
  const statusColor = {
    success: 'text-success',
    pending: 'text-warn',
    failed: 'text-danger'
  }[status]

  return (
    <div className="flex items-center justify-between py-3 border-b border-border-subtle last:border-0">
      <div className="flex-1">
        <div className="flex items-center gap-2">
          <span className="text-text-primary font-medium">{type}</span>
          <span className={`text-sm ${statusColor} capitalize`}>{status}</span>
        </div>
        <div className="text-text-muted text-sm">{amount}</div>
      </div>
      <div className="text-right">
        <div className="text-text-muted text-sm">{timestamp}</div>
        <a
          href={`#/tx/${hash}`}
          className="text-brand-primary text-sm hover:text-brand-soft transition-colors inline-flex items-center gap-1"
        >
          {truncateAddress(hash, 4, 4)}
          <ExternalLink className="w-3 h-3" />
        </a>
      </div>
    </div>
  )
}

export function MyPositions() {
  const [isWalletConnected, setIsWalletConnected] = useState(false)
  const currentAccount = useCurrentAccount()
  const { data: userBalances, isLoading: balancesLoading, error: balancesError } = useUserBalances()
  const { data: spData, isLoading: spLoading, error: spError } = useSpUser()
  const { data: transactionsData, isLoading: transactionsLoading, error: transactionsError } = useUserTransactions()

  // Check wallet connection (demo mode or real wallet)
  useEffect(() => {
    const checkWallet = () => {
      const isConnected = localStorage.getItem('demoWalletConnected') === 'true'
      setIsWalletConnected(isConnected)
    }
    
    checkWallet()
    window.addEventListener('storage', checkWallet)
    return () => window.removeEventListener('storage', checkWallet)
  }, [])

  const hasWallet = currentAccount || isWalletConnected
  const connected = !!currentAccount

  if (!hasWallet) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-center">
          <div className="text-text-muted text-lg mb-2">Connect Wallet</div>
          <div className="text-text-muted text-sm">
            Connect your wallet to view positions and transaction history
          </div>
        </div>
      </div>
    )
  }

  // Use live balances from API only when real wallet is connected (keep raw strings)
  const balances = (currentAccount && userBalances) ? {
    fToken: userBalances.balances.f,
    xToken: userBalances.balances.x,
    Sui: userBalances.balances.r,
  } : null

  // Helper function to format transaction timestamp
  const formatTimestamp = (timestamp: number) => {
    const now = Date.now() / 1000
    const diff = now - timestamp
    if (diff < 3600) return `${Math.floor(diff / 60)} minutes ago`
    if (diff < 86400) return `${Math.floor(diff / 3600)} hours ago`
    return `${Math.floor(diff / 86400)} days ago`
  }

  // Convert API transaction data to component format
  const recentTxs = transactionsData?.items.map(tx => ({
    hash: tx.hash,
    type: tx.type,
    amount: `${tx.amount} ${tx.token}`,
    timestamp: formatTimestamp(tx.timestamp),
    status: tx.status as 'success' | 'pending' | 'failed',
  })) || []

  return (
    <div className="overflow-hidden">
      {/* Balances */}
      <div className="p-4 border-b border-border-subtle">
        <h3 className="text-text-primary font-semibold mb-3">
          Token Balances
          {currentAccount && balancesLoading && (
            <span className="text-text-muted text-sm font-normal ml-2">(Loading live data...)</span>
          )}
        </h3>
        {currentAccount && balancesLoading ? (
          <div className="grid grid-cols-3 gap-4">
            <div className="text-center">
              <div className="text-text-muted text-sm">fToken</div>
              <div className="text-text-primary text-lg font-bold">Loading...</div>
            </div>
            <div className="text-center">
              <div className="text-text-muted text-sm">xToken</div>
              <div className="text-text-primary text-lg font-bold">Loading...</div>
            </div>
            <div className="text-center">
              <div className="text-text-muted text-sm">Sui</div>
              <div className="text-text-primary text-lg font-bold">Loading...</div>
            </div>
          </div>
        ) : currentAccount && balancesError ? (
          <div className="text-center text-text-muted py-4">
            Failed to load balances
          </div>
        ) : balances ? (
          <div className="grid grid-cols-3 gap-4">
            <div className="text-center">
              <div className="text-text-muted text-sm">fToken</div>
              <div className="text-text-primary text-lg font-bold">
                {formatTokenAmount(balances.fToken, DECIMALS)}
              </div>
            </div>
            <div className="text-center">
              <div className="text-text-muted text-sm">xToken</div>
              <div className="text-text-primary text-lg font-bold">
                {formatTokenAmount(balances.xToken, DECIMALS)}
              </div>
            </div>
            <div className="text-center">
              <div className="text-text-muted text-sm">Sui</div>
              <div className="text-text-primary text-lg font-bold">
                {formatTokenAmount(balances.Sui, DECIMALS)}
              </div>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-3 gap-4">
            <div className="text-center">
              <div className="text-text-muted text-sm">fToken</div>
              <div className="text-text-primary text-lg font-bold">-</div>
            </div>
            <div className="text-center">
              <div className="text-text-muted text-sm">xToken</div>
              <div className="text-text-primary text-lg font-bold">-</div>
            </div>
            <div className="text-center">
              <div className="text-text-muted text-sm">Sui</div>
              <div className="text-text-primary text-lg font-bold">-</div>
            </div>
          </div>
        )}
      </div>

      {/* Stability Pool */}
      <div className="p-4 border-b border-border-subtle">
        <h3 className="text-text-primary font-semibold mb-3">Stability Pool</h3>
        {!connected ? (
          <div className="bg-bg-card2/50 rounded-xl p-4">
            <div className="grid grid-cols-2 gap-4 mb-3">
              <div>
                <div className="text-text-muted text-sm">Staked (fToken)</div>
                <div className="text-text-primary text-lg font-bold">—</div>
              </div>
              <div>
                <div className="text-text-muted text-sm">Claimable (Sui)</div>
                <div className="text-success text-lg font-bold">—</div>
              </div>
            </div>
            <div className="text-text-muted text-sm">
              Index gain: —
            </div>
          </div>
        ) : spLoading ? (
          <div className="bg-bg-card2/50 rounded-xl p-4 text-center text-text-muted">
            Loading...
          </div>
        ) : spError ? (
          <div className="bg-bg-card2/50 rounded-xl p-4 text-center text-text-muted">
            Failed to load stability pool data
          </div>
        ) : spData ? (
          <div className="bg-bg-card2/50 rounded-xl p-4">
            <div className="grid grid-cols-2 gap-4 mb-3">
              <div>
                <div className="text-text-muted text-sm">Staked (fToken)</div>
                <div className="text-text-primary text-lg font-bold">
                  {formatNumber(parseFloat(spData.stakeF))}
                </div>
              </div>
              <div>
                <div className="text-text-muted text-sm">Claimable (Sui)</div>
                <div className="text-success text-lg font-bold">
                  {formatNumber(parseFloat(spData.claimableR))}
                </div>
              </div>
            </div>
            <div className="text-text-muted text-sm">
              Index gain: {((parseFloat(spData.currentIndex) / parseFloat(spData.indexAtJoin) - 1) * 100).toFixed(3)}%
            </div>
          </div>
        ) : (
          <div className="bg-bg-card2/50 rounded-xl p-4 text-center text-text-muted">
            No stability pool data
          </div>
        )}
      </div>

      {/* Recent Transactions - only when connected */}
      {connected && (
        <div className="p-4">
          <h3 className="text-text-primary font-semibold mb-3">Recent Transactions</h3>
          {transactionsLoading ? (
            <div className="text-center text-text-muted py-4">
              Loading transactions...
            </div>
          ) : transactionsError ? (
            <div className="text-center text-text-muted py-4">
              Failed to load transactions
            </div>
          ) : recentTxs.length > 0 ? (
            <div className="space-y-0">
              {recentTxs.map((tx) => (
                <TransactionRow key={tx.hash} {...tx} />
              ))}
            </div>
          ) : (
            <div className="text-center text-text-muted py-4">
              No recent transactions
            </div>
          )}
        </div>
      )}
    </div>
  )
}