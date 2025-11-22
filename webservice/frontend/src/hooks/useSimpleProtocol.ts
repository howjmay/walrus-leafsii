import { useQuery } from '@tanstack/react-query'
import { useCurrentAccount } from '@mysten/dapp-kit'
import { SimpleProtocolClient } from '@/sdk/simple-client'

const client = new SimpleProtocolClient()

export function useProtocolData() {
  return useQuery({
    queryKey: ['protocolData'],
    queryFn: () => client.getProtocolData(),
    refetchInterval: 15000,
    staleTime: 10000,
  })
}

export function useUserData() {
  const currentAccount = useCurrentAccount()

  return useQuery({
    queryKey: ['userData', currentAccount?.address],
    queryFn: () => {
      if (!currentAccount?.address) {
        throw new Error('No wallet connected')
      }
      return client.getUserData(currentAccount.address)
    },
    enabled: !!currentAccount?.address,
    refetchInterval: 30000,
    staleTime: 15000,
  })
}

export function useMintPreview(amountR: string) {
  return useQuery({
    queryKey: ['mintPreview', amountR],
    queryFn: () => client.previewMint(Number(amountR)),
    enabled: !!amountR && !isNaN(Number(amountR)) && Number(amountR) > 0,
    staleTime: 30000,
  })
}

export function useRedeemPreview(amountF: string) {
  return useQuery({
    queryKey: ['redeemPreview', amountF],
    queryFn: () => client.previewRedeem(Number(amountF)),
    enabled: !!amountF && !isNaN(Number(amountF)) && Number(amountF) > 0,
    staleTime: 30000,
  })
}