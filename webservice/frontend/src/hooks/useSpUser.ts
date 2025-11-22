import { useQuery } from '@tanstack/react-query'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { apiUrl } from '@/utils/api'

interface SpUserResponse {
  address: string
  stakeF: string
  claimableR: string
  indexAtJoin: string
  currentIndex: string
  enteredAt: number
  updatedAt: number
}

export function useSpUser() {
  const currentAccount = useCurrentAccount()

  return useQuery({
    queryKey: ['spUser', currentAccount?.address],
    queryFn: async (): Promise<SpUserResponse> => {
      if (!currentAccount?.address) {
        throw new Error('No wallet connected')
      }
      
      const response = await fetch(apiUrl(`/v1/sp/user/${currentAccount.address}`))
      if (!response.ok) {
        throw new Error(`Failed to fetch stability pool data: ${response.statusText}`)
      }
      
      return response.json()
    },
    enabled: !!currentAccount?.address,
    refetchInterval: 30000,
    staleTime: 15000,
  })
}