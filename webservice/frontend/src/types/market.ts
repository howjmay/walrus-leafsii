export interface Market {
  id: string
  label: string
  pairSymbol: string
  stableSymbol: string
  leverageSymbol: string
  collateralSymbol: string
  collateralType: string
  collateralHighlights?: string[]
  px: number
  cr: string
  targetCr: string
  reserves: string
  supplyStable: string
  supplyLeverage: string
  mode: string
  feedUrl?: string
  proofCid?: string
  snapshotUrl?: string
  chainId?: string
  asset?: string
}
