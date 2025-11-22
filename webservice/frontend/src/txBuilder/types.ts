export type TxAction = 'mint' | 'redeem'

export type TokenType = 'f' | 'x'

export interface BuildMintRedeemParams {
  action: TxAction
  tokenType: TokenType
  amount: string
  userAddress: string
  chain: string
}

export interface OperationParams {
  amount: string
  userAddress: string
  chain: string
}

export interface BridgeRedeemParams {
  tokenType: TokenType
  amount: string
  userAddress: string
  ethRecipient: string
  chain: string
}

export interface BuildResult {
  transactionBlockBytes: string
  quoteId?: string
}

export interface TransactionBuildInfo {
  packageId: string
  protocolId: string
  poolId: string
  ftokenPackageId: string
  xtokenPackageId: string
  adminCapId: string
  ftokenTreasuryCapId?: string
  xtokenTreasuryCapId?: string
  ftokenAuthorityId?: string
  xtokenAuthorityId?: string
  network: string
  rpcUrl?: string
  wsUrl?: string
  evmRpcUrl?: string
  evmChainId?: string
}
