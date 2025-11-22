import { apiUrlForNetwork } from '../utils/api'
import type { TransactionBuildInfo } from './types'

// Cache for build info to avoid repeated API calls
const cachedBuildInfo: Record<string, TransactionBuildInfo | null> = {}
const buildInfoPromise: Record<string, Promise<TransactionBuildInfo> | null> = {}

const hasBridgeConfig = (info?: TransactionBuildInfo | null) =>
  Boolean(
    info &&
      info.ftokenTreasuryCapId &&
      info.ftokenAuthorityId &&
      info.xtokenTreasuryCapId &&
      info.xtokenAuthorityId
  )

const normalizeBuildInfo = (raw: any): TransactionBuildInfo => {
  // Accept both camelCase and PascalCase in case backend/clients serialize differently.
  const pick = (keys: string[]) => {
    for (const k of keys) {
      const val = raw?.[k]
      if (typeof val === 'string' && val.trim()) return val.trim()
    }
    return ''
  }

  return {
    packageId: pick(['packageId', 'PackageId']),
    protocolId: pick(['protocolId', 'ProtocolId']),
    poolId: pick(['poolId', 'PoolId']),
    ftokenPackageId: pick(['ftokenPackageId', 'FtokenPackageId']),
    xtokenPackageId: pick(['xtokenPackageId', 'XtokenPackageId']),
    adminCapId: pick(['adminCapId', 'AdminCapId']),
    ftokenTreasuryCapId: pick(['ftokenTreasuryCapId', 'FtokenTreasuryCapId']),
    xtokenTreasuryCapId: pick(['xtokenTreasuryCapId', 'XtokenTreasuryCapId']),
    ftokenAuthorityId: pick(['ftokenAuthorityId', 'FtokenAuthorityId']),
    xtokenAuthorityId: pick(['xtokenAuthorityId', 'XtokenAuthorityId']),
    network: pick(['network', 'Network']) || 'localnet',
    rpcUrl: pick(['rpcUrl', 'RpcUrl']),
    wsUrl: pick(['wsUrl', 'WsUrl']),
    evmRpcUrl: pick(['evmRpcUrl', 'EvmRpcUrl']),
    evmChainId: pick(['evmChainId', 'EvmChainId']),
  }
}

export async function getTransactionBuildInfo(network?: string | null): Promise<TransactionBuildInfo> {
  const key = network || 'default'

  // Return cached result if available (and complete)
  if (cachedBuildInfo[key] && hasBridgeConfig(cachedBuildInfo[key])) {
    return cachedBuildInfo[key] as TransactionBuildInfo
  }

  // Return existing promise if already fetching
  if (buildInfoPromise[key]) {
    return buildInfoPromise[key] as Promise<TransactionBuildInfo>
  }

  // Create new fetch promise
  buildInfoPromise[key] = fetchBuildInfo(network)

  try {
    const result = await (buildInfoPromise[key] as Promise<TransactionBuildInfo>)
    cachedBuildInfo[key] = result
    return result
  } finally {
    // Clear the promise after completion (success or failure)
    buildInfoPromise[key] = null
  }
}

async function fetchBuildInfo(network?: string | null): Promise<TransactionBuildInfo> {
  const response = await fetch(apiUrlForNetwork('/v1/protocol/build-info', network), {
    method: 'GET',
    headers: {
      'Content-Type': 'application/json',
    },
  })

  if (!response.ok) {
    throw new Error(`Failed to fetch transaction build info: ${response.status} ${response.statusText}`)
  }

  const data = await response.json()
  return normalizeBuildInfo(data)
}

// Clear cache function for testing or when config changes
export function clearBuildInfoCache(network?: string | null): void {
  if (network) {
    const key = network || 'default'
    cachedBuildInfo[key] = null
    buildInfoPromise[key] = null
    return
  }
  Object.keys(cachedBuildInfo).forEach((k) => delete cachedBuildInfo[k])
  Object.keys(buildInfoPromise).forEach((k) => delete buildInfoPromise[k])
}
