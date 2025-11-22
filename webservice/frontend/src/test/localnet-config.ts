// Localnet configuration for testing
export const LOCALNET_CONFIG = {
  // Sui localnet RPC endpoint
  rpcUrl: 'http://127.0.0.1:9000',

  // Test addresses - these are standard localnet addresses
  // You should replace these with actual addresses from your localnet setup
  testAddresses: {
    // Client address that has SUI coins for testing
    client: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f4',

    // Protocol admin address
    admin: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f5'
  },

  // Test private keys (for localnet only!)
  testKeys: {
    // Client private key - NEVER use in production!
    client: 'suiprivkey1qp8pxqh0p5x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x',

    // Admin private key - NEVER use in production!
    admin: 'suiprivkey1qp8pxqh0p5x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7x7y'
  },

  // Expected protocol configuration on localnet
  // These should match your deployed contracts
  protocolIds: {
    packageId: '0x0000000000000000000000000000000000000000000000000000000000000001',
    protocolId: '0x0000000000000000000000000000000000000000000000000000000000000002',
    poolId: '0x0000000000000000000000000000000000000000000000000000000000000003',
    ftokenPackageId: '0x0000000000000000000000000000000000000000000000000000000000000004',
    xtokenPackageId: '0x0000000000000000000000000000000000000000000000000000000000000005',
    adminCapId: '0x0000000000000000000000000000000000000000000000000000000000000006',
    network: 'localnet'
  },

  // Test amounts
  testAmounts: {
    smallMint: '0.1',     // 0.1 SUI
    largeMint: '10.0',    // 10.0 SUI
    smallRedeem: '0.05',  // 0.05 tokens
    largeRedeem: '5.0'    // 5.0 tokens
  }
}

// Helper to validate localnet is running
export async function validateLocalnet(rpcUrl: string): Promise<boolean> {
  try {
    const response = await fetch(rpcUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        jsonrpc: '2.0',
        id: 1,
        method: 'sui_getLatestSuiSystemState',
        params: []
      })
    })
    return response.ok
  } catch {
    return false
  }
}