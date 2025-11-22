import { useQuery } from '@tanstack/react-query'
import { apiUrl } from '@/utils/api'

interface ProtocolState {
  cr: string
  cr_target: string
  reserves_r: string
  supply_f: string
  supply_x: string
  px: number
  peg_deviation: string
  oracle_age_s: number
  mode: string
  asOf: number
}

export function useProtocolState() {
  return useQuery({
    queryKey: ['protocolState'],
    queryFn: async (): Promise<ProtocolState> => {
      const response = await fetch(apiUrl('/v1/protocol/state'))
      if (!response.ok) {
        throw new Error('Failed to fetch protocol state')
      }
      return response.json()
    },
    refetchInterval: 15000,
    staleTime: 10000,
  })
}