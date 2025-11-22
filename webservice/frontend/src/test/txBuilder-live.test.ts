/**
 * Live integration test for txBuilder against actual localnet
 * This test requires:
 * 1. Sui localnet running on http://127.0.0.1:9000
 * 2. Backend API running on http://localhost:8080
 * 3. Deployed protocol contracts on localnet
 *
 * Run with: npm run test -- txBuilder-live.test.ts
 */

import { describe, it, expect, beforeAll } from 'vitest'
import { SuiClient } from '@mysten/sui/client'
import { createTxBuilder } from '../txBuilder/index'
import { LOCALNET_CONFIG, validateLocalnet } from './localnet-config'

// Set to true when you want to run actual live tests
const RUN_LIVE_TESTS = false

describe('TxBuilder Live Tests (Localnet)', () => {
  let client: SuiClient
  let txBuilder: ReturnType<typeof createTxBuilder>
  let isLocalnetAvailable = false
  let isBackendAvailable = false

  beforeAll(async () => {
    // Check if localnet is running
    isLocalnetAvailable = await validateLocalnet(LOCALNET_CONFIG.rpcUrl)
    console.log(`🔍 Localnet available: ${isLocalnetAvailable}`)

    // Check if backend API is available
    try {
      const response = await fetch('http://localhost:8080/v1/protocol/build-info')
      isBackendAvailable = response.ok
    } catch {
      isBackendAvailable = false
    }
    console.log(`🔍 Backend API available: ${isBackendAvailable}`)

    if (isLocalnetAvailable) {
      client = new SuiClient({ url: LOCALNET_CONFIG.rpcUrl })
      txBuilder = createTxBuilder(client)
    }
  })

  describe('Live Environment Checks', () => {
    it('should detect localnet availability', async () => {
      if (RUN_LIVE_TESTS) {
        expect(isLocalnetAvailable).toBe(true)
      } else {
        console.log('ℹ️  Live tests disabled. Set RUN_LIVE_TESTS=true to enable.')
      }
    })

    it('should detect backend API availability', async () => {
      if (RUN_LIVE_TESTS) {
        expect(isBackendAvailable).toBe(true)
      } else {
        console.log('ℹ️  Set RUN_LIVE_TESTS=true to check backend availability.')
      }
    })
  })

  describe('Live Transaction Building', () => {
    const testIf = (RUN_LIVE_TESTS && isLocalnetAvailable && isBackendAvailable) ? it : it.skip

    testIf('should build actual mintFToken transaction', async () => {
      const result = await txBuilder.mintFToken({
        amount: '0.1',
        userAddress: LOCALNET_CONFIG.testAddresses.client,
        chain: 'localnet'
      })

      expect(result.transactionBlockBytes).toBeTruthy()
      console.log(`✅ Built mintFToken transaction: ${result.transactionBlockBytes.slice(0, 32)}...`)
    })

    testIf('should build actual mintXToken transaction', async () => {
      const result = await txBuilder.mintXToken({
        amount: '0.1',
        userAddress: LOCALNET_CONFIG.testAddresses.client,
        chain: 'localnet'
      })

      expect(result.transactionBlockBytes).toBeTruthy()
      console.log(`✅ Built mintXToken transaction: ${result.transactionBlockBytes.slice(0, 32)}...`)
    })

    testIf('should fetch real protocol configuration', async () => {
      const { getTransactionBuildInfo } = await import('../txBuilder/config')
      const config = await getTransactionBuildInfo()

      expect(config.packageId).toBeTruthy()
      expect(config.protocolId).toBeTruthy()
      expect(config.poolId).toBeTruthy()
      expect(config.ftokenPackageId).toBeTruthy()
      expect(config.xtokenPackageId).toBeTruthy()

      console.log('📋 Protocol Configuration:', {
        packageId: config.packageId,
        protocolId: config.protocolId,
        poolId: config.poolId,
        network: config.network
      })
    })

    testIf('should query actual chain state', async () => {
      // Get system state to verify connection
      try {
        const response = await fetch(LOCALNET_CONFIG.rpcUrl, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            jsonrpc: '2.0',
            id: 1,
            method: 'sui_getLatestSuiSystemState',
            params: []
          })
        })

        expect(response.ok).toBe(true)
        const data = await response.json()
        expect(data.result).toBeTruthy()

        console.log(`⛓️  Connected to Sui at epoch: ${data.result.epoch}`)
      } catch (error) {
        console.error('Failed to query chain state:', error)
        throw error
      }
    })
  })

  describe('Live Error Scenarios', () => {
    const testIf = (RUN_LIVE_TESTS && isLocalnetAvailable && isBackendAvailable) ? it : it.skip

    testIf('should handle invalid user address', async () => {
      await expect(
        txBuilder.mintFToken({
          amount: '0.1',
          userAddress: '0xinvalid',
          chain: 'localnet'
        })
      ).rejects.toThrow()

      console.log('✅ Properly handles invalid addresses')
    })
  })
})

/**
 * Usage Instructions:
 *
 * 1. Start Sui Localnet:
 *    sui start --with-faucet --force-regenesis
 *
 * 2. Deploy your protocol contracts to localnet
 *
 * 3. Start your backend API server:
 *    go run cmd/api/main.go
 *
 * 4. Update LOCALNET_CONFIG with actual deployed contract addresses
 *
 * 5. Set RUN_LIVE_TESTS = true in this file
 *
 * 6. Run the test:
 *    npm run test -- txBuilder-live.test.ts
 */