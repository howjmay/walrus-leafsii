import { useQuery } from '@tanstack/react-query'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { apiUrl } from '@/utils/api'

interface UserBalancesResponse {
  address: string
  balances: {
    f: string
    x: string
    r: string
  }
  updatedAt: number
}

export function useUserBalances() {
  const currentAccount = useCurrentAccount()

  return useQuery({
    queryKey: ['userBalances', currentAccount?.address],
    queryFn: async (): Promise<UserBalancesResponse> => {
      if (!currentAccount?.address) {
        throw new Error('No wallet connected')
      }
      
      const response = await fetch(apiUrl(`/v1/users/${currentAccount.address}/balances`))
      if (!response.ok) {
        throw new Error(`Failed to fetch balances: ${response.statusText}`)
      }
      
      return response.json()
    },
    enabled: !!currentAccount?.address,
    refetchInterval: 30000, // Refresh every 30 seconds
    staleTime: 15000,
  })
}