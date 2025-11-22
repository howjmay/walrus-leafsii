export interface WalrusCheckpoint {
  updateId: number
  chainId: string
  asset: string
  vault?: string
  blockNumber: number
  blockHash?: string
  totalShares: string
  index: string
  balancesRoot: string
  proofType?: string
  walrusBlobId?: string
  status: string
  timestamp?: number
}

export interface CrossChainBalance {
  suiOwner: string
  chainId: string
  asset: string
  shares: string
  index: string
  value: string
  collateralUsd: string
  lastCheckpointId?: number
  updatedAt?: number
}

export interface VaultInfo {
  chainId: string
  asset: string
  vaultAddress: string
  depositMemoFormat?: string
  feedUrl?: string
  proofCid?: string
  snapshotUrl?: string
}

export interface CollateralParams {
  chainId: string
  asset: string
  ltv: string
  maintenanceThreshold: string
  liquidationPenalty: string
  oracleHaircut: string
  stalenessHardCap: number
  mintRateLimit: string
  withdrawRateLimit: string
  active: boolean
}

export interface WithdrawalVoucher {
  voucherId: string
  suiOwner: string
  chainId: string
  asset: string
  shares: string
  nonce: number
  expiry: number
  status: string
  txHash?: string
  createdAt: number
}

export interface BridgeReceipt {
  receiptId: string
  txHash: string
  suiOwner: string
  chainId: string
  asset: string
  minted: string
  createdAt: number
  suiTxDigests?: string[]
}

export interface RedeemReceipt {
  receiptId: string
  suiTxDigest: string
  suiOwner: string
  ethRecipient: string
  chainId: string
  asset: string
  token: string
  burned: string
  payoutEth: string
  walrusUpdateId?: number
  walrusBlobId?: string
  payoutTxHash?: string
  createdAt: number
}
