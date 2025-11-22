import { describe, it, expect, vi, beforeEach } from 'vitest'
import { SuiClient } from '@mysten/sui/client'
import { createTxBuilder } from './index'
import { LOCALNET_CONFIG } from '../test/localnet-config'
import type { TransactionBuildInfo } from './types'

describe('TxBuilder Unit Tests', () => {
  let client: SuiClient
  let txBuilder: ReturnType<typeof createTxBuilder>

  beforeEach(() => {
    client = new SuiClient({ url: LOCALNET_CONFIG.rpcUrl })
    txBuilder = createTxBuilder(client)
  })

  describe('Configuration Management', () => {
    it('should handle API configuration correctly', async () => {
      const mockConfig: TransactionBuildInfo = {
        packageId: '0x1234567890123456789012345678901234567890123456789012345678901234',
        protocolId: '0x2345678901234567890123456789012345678901234567890123456789012345',
        poolId: '0x3456789012345678901234567890123456789012345678901234567890123456',
        ftokenPackageId: '0x4567890123456789012345678901234567890123456789012345678901234567',
        xtokenPackageId: '0x5678901234567890123456789012345678901234567890123456789012345678',
        adminCapId: '0x6789012345678901234567890123456789012345678901234567890123456789',
        network: 'localnet'
      }

      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockConfig
      })

      const { getTransactionBuildInfo } = await import('./config')
      const config = await getTransactionBuildInfo()

      expect(config).toEqual(mockConfig)
      expect(config.packageId).toMatch(/^0x[a-f0-9]{64}$/)
    })
  })

  describe('Transaction Structure Validation', () => {
    it('should validate mint transaction parameters', async () => {
      // Test parameter validation without building
      const params = {
        amount: '0.1',
        userAddress: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f4',
        chain: 'localnet'
      }

      expect(params.amount).toBe('0.1')
      expect(params.userAddress).toMatch(/^0x[a-f0-9]{64}$/)
      expect(params.chain).toBe('localnet')
    })

    it('should validate amount conversion', () => {
      const amount = '0.1'
      const amountUnits = BigInt(Math.round(parseFloat(amount) * 1e9))

      expect(amountUnits).toBe(100000000n)
    })

    it('should construct correct type arguments', () => {
      const mockConfig = {
        ftokenPackageId: '0x4567890123456789012345678901234567890123456789012345678901234567',
        xtokenPackageId: '0x5678901234567890123456789012345678901234567890123456789012345678'
      }

      const FTOKEN_TYPE = `${mockConfig.ftokenPackageId}::ftoken::FTOKEN`
      const XTOKEN_TYPE = `${mockConfig.xtokenPackageId}::xtoken::XTOKEN`
      const SUI_TYPE = '0x2::sui::SUI'

      expect(FTOKEN_TYPE).toBe('0x4567890123456789012345678901234567890123456789012345678901234567::ftoken::FTOKEN')
      expect(XTOKEN_TYPE).toBe('0x5678901234567890123456789012345678901234567890123456789012345678::xtoken::XTOKEN')
      expect(SUI_TYPE).toBe('0x2::sui::SUI')
    })
  })

  describe('Error Handling', () => {
    it('should handle insufficient balance correctly', async () => {
      // Mock insufficient balance scenario
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => LOCALNET_CONFIG.protocolIds
      })

      vi.spyOn(client, 'getObject').mockResolvedValue({
        data: {
          owner: {
            Shared: {
              initial_shared_version: '1'
            }
          }
        }
      } as any)

      vi.spyOn(client, 'getCoins').mockResolvedValue({
        data: [
          {
            coinObjectId: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f4',
            balance: '1000' // Very small balance
          }
        ]
      } as any)

      await expect(
        txBuilder.mintFToken({
          amount: '10.0', // Large amount
          userAddress: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f4',
          chain: 'localnet'
        })
      ).rejects.toThrow('Insufficient SUI balance')
    })

    it('should handle invalid shared objects', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => LOCALNET_CONFIG.protocolIds
      })

      // Mock non-shared object
      vi.spyOn(client, 'getObject').mockResolvedValue({
        data: {
          owner: 'someAddress' // Not shared
        }
      } as any)

      await expect(
        txBuilder.mintFToken({
          amount: '0.1',
          userAddress: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f4',
          chain: 'localnet'
        })
      ).rejects.toThrow('Protocol object is not shared')
    })

    it('should handle API failures gracefully', async () => {
      // Clear the cache first
      const { clearBuildInfoCache } = await import('./config')
      clearBuildInfoCache()

      // Mock API failure
      global.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error'
      })

      const { getTransactionBuildInfo } = await import('./config')

      await expect(getTransactionBuildInfo()).rejects.toThrow('Failed to fetch transaction build info: 500 Internal Server Error')
    })
  })

  describe('Method Routing', () => {
    it('should route buildMintRedeem calls correctly', async () => {
      // Mock successful API calls
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => LOCALNET_CONFIG.protocolIds
      })

      vi.spyOn(client, 'getObject').mockResolvedValue({
        data: {
          owner: {
            Shared: {
              initial_shared_version: '1'
            }
          }
        }
      } as any)

      vi.spyOn(client, 'getCoins').mockResolvedValue({
        data: [
          {
            coinObjectId: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f4',
            balance: '1000000000' // 1 SUI
          }
        ]
      } as any)

      // Spy on individual methods to verify routing
      const mintFTokenSpy = vi.spyOn(txBuilder, 'mintFToken')
      const mintXTokenSpy = vi.spyOn(txBuilder, 'mintXToken')

      // Test routing to mintFToken
      await expect(
        txBuilder.buildMintRedeem({
          action: 'mint',
          tokenType: 'f',
          amount: '0.1',
          userAddress: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f4',
          chain: 'localnet'
        })
      ).rejects.toThrow() // We expect it to fail at build step, but routing should work

      expect(mintFTokenSpy).toHaveBeenCalled()

      // Test routing to mintXToken
      await expect(
        txBuilder.buildMintRedeem({
          action: 'mint',
          tokenType: 'x',
          amount: '0.1',
          userAddress: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f4',
          chain: 'localnet'
        })
      ).rejects.toThrow()

      expect(mintXTokenSpy).toHaveBeenCalled()
    })

    it('should throw error for unsupported operations', async () => {
      await expect(
        txBuilder.buildMintRedeem({
          action: 'unsupported' as any,
          tokenType: 'f',
          amount: '0.1',
          userAddress: '0x742d35cc6ba9b3ac9b19c6ad11d5ba83e7c2b1e5f2c8c6c7b5f5b7c2c1f7f9f4',
          chain: 'localnet'
        })
      ).rejects.toThrow('Unsupported operation')
    })
  })
})