export type Network = 'mainnet' | 'testnet' | 'localnet'
export type Tab = 'swap' | 'mint' | 'stake' | 'limit' | 'twap'

export interface URLState {
  network?: Network
  tab?: Tab
  from?: string
  to?: string
  amount?: string
  slippage?: string
}

export function parseURLState(searchParams: URLSearchParams): URLState {
  return {
    network: (searchParams.get('network') as Network) || 'localnet',
    tab: (searchParams.get('tab') as Tab) || 'mint',
    from: searchParams.get('from') || undefined,
    to: searchParams.get('to') || undefined,
    amount: searchParams.get('amount') || undefined,
    slippage: searchParams.get('slippage') || undefined,
  }
}

export function buildURL(state: URLState): string {
  const params = new URLSearchParams()
  
  if (state.network) params.set('network', state.network)
  if (state.tab) params.set('tab', state.tab)
  if (state.from) params.set('from', state.from)
  if (state.to) params.set('to', state.to)
  if (state.amount) params.set('amount', state.amount)
  if (state.slippage) params.set('slippage', state.slippage)
  
  return params.toString() ? `?${params.toString()}` : ''
}
