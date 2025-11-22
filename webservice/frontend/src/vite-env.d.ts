/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_NETWORK?: 'mainnet' | 'testnet' | 'localnet'
  readonly VITE_API_BASE?: string
  readonly VITE_MAINNET_API_BASE?: string
  readonly VITE_TESTNET_API_BASE?: string
  readonly VITE_LOCALNET_API_BASE?: string
  readonly VITE_SHOW_HEALTH_BADGE?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
