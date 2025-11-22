import { useQuery } from '@tanstack/react-query'
import { apiUrl } from '@/utils/api'

interface ProtocolMetrics {
  currentCR: string
  targetCR: string
  pegDeviation: string
  reservesR: string
  supplyF: string
  supplyX: string
  spTVL: string
  rewardAPR: string
  indexDelta: string
  asOf: number
}

export function useProtocolMetrics() {
  return useQuery({
    queryKey: ['protocolMetrics'],
    queryFn: async (): Promise<ProtocolMetrics> => {
      const response = await fetch(apiUrl('/v1/protocol/metrics'))
      if (!response.ok) {
        throw new Error('Failed to fetch protocol metrics')
      }
      return response.json()
    },
    refetchInterval: 15000,
    staleTime: 10000,
  })
}