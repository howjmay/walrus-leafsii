// Simplified mock client for demonstration
export interface ProtocolData {
  currentCR: number
  targetCR: number
  reserves: number
  fTokenSupply: number
  xTokenSupply: number
  oraclePrice: number
  lastUpdate: number
  systemMode: 'normal' | 'rebalance' | 'emergency'
}

export interface UserData {
  address: string
  fTokenBalance: number
  xTokenBalance: number
  SuiBalance: number
  stakedAmount: number
  claimableRewards: number
  indexAtJoin: number
}

export class SimpleProtocolClient {
  // Mock data methods
  async getProtocolData(): Promise<ProtocolData> {
    return {
      currentCR: 1.45,
      targetCR: 1.50,
      reserves: 12500000,
      fTokenSupply: 8500000,
      xTokenSupply: 1200000,
      oraclePrice: 0.9985,
      lastUpdate: Date.now() - 23000,
      systemMode: 'normal'
    }
  }

  async getUserData(address: string): Promise<UserData> {
    return {
      address,
      fTokenBalance: 1250.45,
      xTokenBalance: 89.12,
      SuiBalance: 2100.88,
      stakedAmount: 850.0,
      claimableRewards: 12.45,
      indexAtJoin: 1.0234
    }
  }

  async previewMint(amountR: number): Promise<{ outputAmount: number; fee: number; priceImpact: number }> {
    const outputAmount = amountR * 0.995 // 0.5% fee
    const fee = amountR * 0.005
    return { outputAmount, fee, priceImpact: 0.02 }
  }

  async previewRedeem(amountF: number): Promise<{ outputAmount: number; fee: number; priceImpact: number }> {
    const outputAmount = amountF * 0.992 // 0.8% fee
    const fee = amountF * 0.008
    return { outputAmount, fee, priceImpact: 0.03 }
  }
}