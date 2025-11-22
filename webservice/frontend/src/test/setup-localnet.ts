/**
 * Helper script to set up localnet testing environment
 * This script helps you:
 * 1. Get real addresses from localnet
 * 2. Fund test addresses
 * 3. Validate protocol deployment
 */

import { SuiClient } from '@mysten/sui/client'
import { Ed25519Keypair } from '@mysten/sui/keypairs/ed25519'
import { getFaucetHost, requestSuiFromFaucetV0 } from '@mysten/sui/faucet'

const LOCALNET_RPC = 'http://127.0.0.1:9000'
// const FAUCET_URL = 'http://127.0.0.1:9123/gas' // Unused but may be needed for manual faucet calls

interface LocalnetSetup {
  client: SuiClient
  testKeypair: Ed25519Keypair
  testAddress: string
}

export async function setupLocalnetTesting(): Promise<LocalnetSetup> {
  console.log('🔧 Setting up localnet testing environment...')

  // Initialize client
  const client = new SuiClient({ url: LOCALNET_RPC })

  // Generate test keypair
  const testKeypair = new Ed25519Keypair()
  const testAddress = testKeypair.toSuiAddress()

  console.log(`👤 Test address: ${testAddress}`)

  try {
    // Request SUI from faucet
    console.log('💰 Requesting SUI from faucet...')
    await requestSuiFromFaucetV0({
      host: getFaucetHost('localnet'),
      recipient: testAddress
    })

    // Verify balance
    const balance = await client.getBalance({ owner: testAddress })
    console.log(`✅ Balance: ${parseInt(balance.totalBalance) / 1e9} SUI`)

    return { client, testKeypair, testAddress }
  } catch (error) {
    console.error('❌ Failed to setup localnet testing:', error)
    throw error
  }
}

export async function validateProtocolDeployment(client: SuiClient, packageId: string) {
  console.log(`🔍 Validating protocol deployment at ${packageId}...`)

  try {
    const packageInfo = await client.getObject({
      id: packageId,
      options: { showType: true, showContent: true }
    })

    if (!packageInfo.data) {
      throw new Error('Protocol package not found')
    }

    console.log('✅ Protocol package found')
    return true
  } catch (error) {
    console.error('❌ Protocol validation failed:', error)
    return false
  }
}

export async function getProtocolConfigFromBackend(): Promise<any> {
  console.log('📡 Fetching protocol config from backend...')

  try {
    const response = await fetch('http://localhost:8080/v1/protocol/build-info')
    if (!response.ok) {
      throw new Error(`Backend API error: ${response.status}`)
    }

    const config = await response.json()
    console.log('✅ Protocol config fetched:', config)
    return config
  } catch (error) {
    console.error('❌ Failed to fetch protocol config:', error)
    throw error
  }
}

// CLI utility - run this with: npx tsx src/test/setup-localnet.ts
if (import.meta.url === `file://${process.argv[1]}`) {
  (async () => {
    try {
      console.log('🚀 Starting localnet setup...')

      // Setup testing environment
      const setup = await setupLocalnetTesting()

      // Try to get protocol config
      try {
        const config = await getProtocolConfigFromBackend()
        await validateProtocolDeployment(setup.client, config.packageId)
      } catch (error) {
        console.log('⚠️  Backend API not available or protocol not deployed')
        console.log('💡 Make sure to:')
        console.log('   1. Deploy your protocol to localnet')
        console.log('   2. Start the backend API server')
        console.log('   3. Update LOCALNET_CONFIG with real addresses')
      }

      console.log('\n📋 Update your test config with these values:')
      console.log('export const LOCALNET_CONFIG = {')
      console.log(`  rpcUrl: '${LOCALNET_RPC}',`)
      console.log('  testAddresses: {')
      console.log(`    client: '${setup.testAddress}',`)
      console.log('    admin: \'<YOUR_ADMIN_ADDRESS>\'')
      console.log('  },')
      console.log('  // ... rest of config')
      console.log('}')

    } catch (error) {
      console.error('💥 Setup failed:', error)
      process.exit(1)
    }
  })()
}