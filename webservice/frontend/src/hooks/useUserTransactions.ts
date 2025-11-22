import { useQuery } from '@tanstack/react-query'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { apiUrl } from '@/utils/api'

interface TransactionItem {
  hash: string
  type: string
  amount: string
  token: string
  timestamp: number
  status: 'success' | 'pending' | 'failed'
}

interface UserTransactionsResponse {
  address: string
  items: TransactionItem[]
  nextCursor: string
  updatedAt: number
}

interface UseUserTransactionsOptions {
  limit?: number
  cursor?: string
}

export function useUserTransactions(options: UseUserTransactionsOptions = {}) {
  const currentAccount = useCurrentAccount()
  const { limit = 20, cursor = '' } = options

  return useQuery({
    queryKey: ['userTx', currentAccount?.address, limit, cursor],
    queryFn: async (): Promise<UserTransactionsResponse> => {
      if (!currentAccount?.address) {
        throw new Error('No wallet connected')
      }
      
      const params = new URLSearchParams()
      params.append('limit', limit.toString())
      if (cursor) {
        params.append('cursor', cursor)
      }
      
      const response = await fetch(apiUrl(`/v1/users/${currentAccount.address}/transactions?${params.toString()}`))
      if (!response.ok) {
        throw new Error(`Failed to fetch transactions: ${response.statusText}`)
      }
      
      return response.json()
    },
    enabled: !!currentAccount?.address,
    refetchInterval: 60000, // Refresh every minute
    staleTime: 30000,
  })
}