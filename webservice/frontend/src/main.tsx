import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WalletProvider, SuiClientProvider } from '@mysten/dapp-kit'
import { getFullnodeUrl } from '@mysten/sui/client'
import { Toaster } from 'sonner'
import App from './App.tsx'
import './index.css'
import '@mysten/dapp-kit/dist/index.css'
import { getApiBase, setRuntimeApiBase } from './utils/api'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30 * 1000,
      refetchOnWindowFocus: false,
    },
  },
})

type SupportedNetwork = 'mainnet' | 'testnet' | 'localnet'

const isSupportedNetwork = (n: string | null | undefined): n is SupportedNetwork =>
  n === 'mainnet' || n === 'testnet' || n === 'localnet'

type NetworkInfo = {
  network: SupportedNetwork
  rpcUrl?: string
  evmRpcUrl?: string
  evmChainId?: string
}

async function detectNetwork(): Promise<NetworkInfo> {
  const apiBase = getApiBase()
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 3000)

  try {
    const res = await fetch(`${apiBase}/v1/protocol/build-info`, { signal: controller.signal })
    clearTimeout(timeout)
    if (res.ok) {
      const data = (await res.json()) as { network?: string; rpcUrl?: string; evmRpcUrl?: string; evmChainId?: string }
      if (isSupportedNetwork(data?.network)) {
        return { network: data.network, rpcUrl: data.rpcUrl, evmRpcUrl: data.evmRpcUrl, evmChainId: data.evmChainId }
      }
    }
  } catch {
    clearTimeout(timeout)
  }

  const envNet = import.meta.env.VITE_NETWORK
  if (isSupportedNetwork(envNet)) return { network: envNet }

  return { network: 'localnet', rpcUrl: undefined, evmRpcUrl: undefined, evmChainId: undefined }
}

async function start() {
  const { network: defaultNetwork, rpcUrl } = await detectNetwork()
  // Pin the API base at runtime so subsequent apiUrl() calls never fall back to Vite origin.
  setRuntimeApiBase(getApiBase())

  const networks = {
    mainnet: { url: rpcUrl && defaultNetwork === 'mainnet' ? rpcUrl : getFullnodeUrl('mainnet') },
    testnet: { url: rpcUrl && defaultNetwork === 'testnet' ? rpcUrl : getFullnodeUrl('testnet') },
    localnet: { url: rpcUrl && defaultNetwork === 'localnet' ? rpcUrl : 'http://localhost:9000' },
  }

  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <QueryClientProvider client={queryClient}>
        <SuiClientProvider networks={networks} defaultNetwork={defaultNetwork}>
          <WalletProvider>
            <BrowserRouter>
              <App />
              <Toaster
                theme="dark"
                position="bottom-right"
                toastOptions={{
                  style: {
                    background: '#121826',
                    border: '1px solid #1F2937',
                    color: '#E5E7EB',
                  },
                }}
              />
            </BrowserRouter>
          </WalletProvider>
        </SuiClientProvider>
      </QueryClientProvider>
    </React.StrictMode>,
  )
}

start()
