import { Transaction, TransactionArgument } from '@mysten/sui/transactions'
import { toBase64 } from '@mysten/sui/utils'
import type { SuiClient } from '@mysten/sui/client'
import type { BridgeRedeemParams, BuildMintRedeemParams, BuildResult, OperationParams, TransactionBuildInfo } from './types'
import { clearBuildInfoCache, getTransactionBuildInfo } from './config'

export interface TxBuilder {
  buildMintRedeem(params: BuildMintRedeemParams): Promise<BuildResult>
  bridgeRedeem(params: BridgeRedeemParams): Promise<BuildResult>
  mintFToken(params: OperationParams): Promise<BuildResult>
  redeemFToken(params: OperationParams): Promise<BuildResult>
  mintXToken(params: OperationParams): Promise<BuildResult>
  redeemXToken(params: OperationParams): Promise<BuildResult>
}

interface SharedObjectInfo {
  protocolArg: TransactionArgument
  poolArg: TransactionArgument
}

interface TypeArguments {
  FTOKEN_TYPE: string
  XTOKEN_TYPE: string
  SUI_TYPE: string
}

export function createTxBuilder(client: SuiClient, network?: string | null): TxBuilder {
  // Helper function to resolve shared objects and create references
  async function resolveSharedObjects(config: TransactionBuildInfo, tx: Transaction): Promise<SharedObjectInfo> {
    const [protocolObj, poolObj] = await Promise.all([
      client.getObject({
        id: config.protocolId,
        options: { showOwner: true }
      }),
      client.getObject({
        id: config.poolId,
        options: { showOwner: true }
      })
    ])

    if (!protocolObj.data?.owner || typeof protocolObj.data.owner === 'string' || !('Shared' in protocolObj.data.owner)) {
      throw new Error('Protocol object is not shared')
    }
    if (!poolObj.data?.owner || typeof poolObj.data.owner === 'string' || !('Shared' in poolObj.data.owner)) {
      throw new Error('Pool object is not shared')
    }

    const protocolArg = tx.sharedObjectRef({
      objectId: config.protocolId,
      initialSharedVersion: protocolObj.data.owner.Shared.initial_shared_version,
      mutable: true
    })
    const poolArg = tx.sharedObjectRef({
      objectId: config.poolId,
      initialSharedVersion: poolObj.data.owner.Shared.initial_shared_version,
      mutable: true
    })

    return { protocolArg, poolArg }
  }

  // Helper function to get type arguments
  function getTypeArguments(config: TransactionBuildInfo): TypeArguments {
    return {
      FTOKEN_TYPE: `${config.ftokenPackageId}::ftoken::FTOKEN`,
      XTOKEN_TYPE: `${config.xtokenPackageId}::xtoken::XTOKEN`,
      SUI_TYPE: '0x2::sui::SUI'
    }
  }

  // Helper function for SUI coin handling (used by mint operations)
  async function handleSuiCoins(tx: Transaction, userAddress: string, amountUnits: bigint): Promise<TransactionArgument> {
    const coinsRes = await client.getCoins({ owner: userAddress })
    const coins = coinsRes.data

    const totalBalance = coins.reduce((sum, coin) => sum + BigInt(coin.balance), 0n)
    if (totalBalance < amountUnits) {
      throw new Error('Insufficient SUI balance')
    }

    let targetCoin = coins[0]
    let coinArg = tx.object(targetCoin.coinObjectId)

    if (BigInt(targetCoin.balance) < amountUnits) {
      const additionalCoins = []
      let accumulatedBalance = BigInt(targetCoin.balance)

      for (let i = 1; i < coins.length && accumulatedBalance < amountUnits; i++) {
        additionalCoins.push(tx.object(coins[i].coinObjectId))
        accumulatedBalance += BigInt(coins[i].balance)
      }

      if (additionalCoins.length > 0) {
        tx.mergeCoins(coinArg, additionalCoins)
      }
    }

    const [splitCoin] = tx.splitCoins(coinArg, [tx.pure.u64(amountUnits)])
    return splitCoin
  }

  // Helper function for token coin handling (used by redeem operations)
  async function handleTokenCoins(tx: Transaction, userAddress: string, coinType: string, amountUnits: bigint, tokenName: string): Promise<TransactionArgument> {
    const coinsRes = await client.getCoins({ owner: userAddress, coinType })
    const coins = coinsRes.data

    const totalBalance = coins.reduce((sum, coin) => sum + BigInt(coin.balance), 0n)
    if (totalBalance < amountUnits) {
      throw new Error(`Insufficient ${tokenName} balance`)
    }

    let targetCoin = coins[0]
    let coinArg = tx.object(targetCoin.coinObjectId)

    if (BigInt(targetCoin.balance) < amountUnits) {
      const additionalCoins = []
      let accumulatedBalance = BigInt(targetCoin.balance)

      for (let i = 1; i < coins.length && accumulatedBalance < amountUnits; i++) {
        additionalCoins.push(tx.object(coins[i].coinObjectId))
        accumulatedBalance += BigInt(coins[i].balance)
      }

      if (additionalCoins.length > 0) {
        tx.mergeCoins(coinArg, additionalCoins)
      }
    }

    const [splitCoin] = tx.splitCoins(coinArg, [tx.pure.u64(amountUnits)])
    return splitCoin
  }

  // Helper for shared object references (authority, etc.)
  async function getSharedObjectArg(tx: Transaction, objectId: string, label: string): Promise<TransactionArgument> {
    const obj = await client.getObject({ id: objectId, options: { showOwner: true } })
    const owner = obj.data?.owner
    if (!owner || typeof owner === 'string' || !('Shared' in owner)) {
      throw new Error(`${label} object is not shared`)
    }
    return tx.sharedObjectRef({
      objectId,
      initialSharedVersion: owner.Shared.initial_shared_version,
      mutable: true
    })
  }

  function normalizeEthRecipient(recipient: string): Uint8Array {
    const normalized = recipient.trim().toLowerCase()
    if (!/^0x[a-f0-9]{40}$/.test(normalized)) {
      throw new Error('Recipient address must be a 0x-prefixed 20-byte hex string')
    }
    const hex = normalized.replace(/^0x/, '')
    const bytes = new Uint8Array(hex.length / 2)
    for (let i = 0; i < hex.length; i += 2) {
      bytes[i / 2] = parseInt(hex.slice(i, i + 2), 16)
    }
    return bytes
  }

  return {
    async buildMintRedeem(params: BuildMintRedeemParams): Promise<BuildResult> {
      const { action, tokenType, amount, userAddress, chain } = params

      // Route to specific operation method
      const operationParams = { amount, userAddress, chain }

      if (action === 'mint' && tokenType === 'f') {
        return this.mintFToken(operationParams)
      } else if (action === 'mint' && tokenType === 'x') {
        return this.mintXToken(operationParams)
      } else if (action === 'redeem' && tokenType === 'f') {
        return this.redeemFToken(operationParams)
      } else if (action === 'redeem' && tokenType === 'x') {
        return this.redeemXToken(operationParams)
      }

      throw new Error(`Unsupported operation: ${action} ${tokenType}`)
    },

    async bridgeRedeem(params: BridgeRedeemParams): Promise<BuildResult> {
      const { tokenType, amount, userAddress, ethRecipient } = params

      let config = await getTransactionBuildInfo(network)
      const missingBridgeFields =
        !config.ftokenTreasuryCapId ||
        !config.ftokenAuthorityId ||
        !config.xtokenTreasuryCapId ||
        !config.xtokenAuthorityId
      if (missingBridgeFields) {
        // Clear stale cache and retry once to pick up freshly-injected backend env vars.
        clearBuildInfoCache(network)
        config = await getTransactionBuildInfo(network)
      }

      const missing: string[] = []
      if (!config.ftokenTreasuryCapId) missing.push('ftokenTreasuryCapId')
      if (!config.ftokenAuthorityId) missing.push('ftokenAuthorityId')
      if (!config.xtokenTreasuryCapId) missing.push('xtokenTreasuryCapId')
      if (!config.xtokenAuthorityId) missing.push('xtokenAuthorityId')

      const tx = new Transaction()
      const typeArgs = getTypeArguments(config)

      const coinType = tokenType === 'f' ? typeArgs.FTOKEN_TYPE : typeArgs.XTOKEN_TYPE
      const treasuryCapId = tokenType === 'f' ? config.ftokenTreasuryCapId : config.xtokenTreasuryCapId
      const authorityId = tokenType === 'f' ? config.ftokenAuthorityId : config.xtokenAuthorityId

      if (!treasuryCapId || !authorityId) {
        const details = missing.length ? `Missing: ${missing.join(', ')}` : 'Bridge redeem not configured'
        throw new Error(
          `Bridge redeem is not configured for this network (${details}). Ask the operator to set LFS_SUI_FTOKEN_TREASURY_CAP / LFS_SUI_FTOKEN_AUTHORITY / LFS_SUI_XTOKEN_TREASURY_CAP / LFS_SUI_XTOKEN_AUTHORITY on the backend.`
        )
      }

      const metadata = await client.getCoinMetadata({ coinType })
      const decimals = metadata?.decimals ?? 9
      const amountUnits = BigInt(Math.round(parseFloat(amount) * (10 ** decimals)))

      const [splitCoin, authorityArg] = await Promise.all([
        handleTokenCoins(tx, userAddress, coinType, amountUnits, tokenType === 'f' ? 'FTOKEN' : 'XTOKEN'),
        getSharedObjectArg(tx, authorityId, 'Authority')
      ])

      const target = tokenType === 'f'
        ? `${config.ftokenPackageId}::ftoken::bridge_redeem`
        : `${config.xtokenPackageId}::xtoken::bridge_redeem`

      tx.moveCall({
        target,
        arguments: [
          tx.object(treasuryCapId),
          authorityArg,
          splitCoin,
          tx.pure(normalizeEthRecipient(ethRecipient))
        ]
      })

      tx.setGasBudget(120000000)
      tx.setSender(userAddress)

      const bytes = await tx.build({ client })
      const transactionBlockBytes = toBase64(bytes)

      return { transactionBlockBytes }
    },

    async mintFToken(params: OperationParams): Promise<BuildResult> {
      const { amount, userAddress } = params

      const config = await getTransactionBuildInfo(network)
      const tx = new Transaction()
      const amountUnits = BigInt(Math.round(parseFloat(amount) * 1e9))

      // Use helper functions to reduce duplication
      const [{ protocolArg, poolArg }, splitCoin, typeArgs] = await Promise.all([
        resolveSharedObjects(config, tx),
        handleSuiCoins(tx, userAddress, amountUnits),
        Promise.resolve(getTypeArguments(config))
      ])

      // Call mint_f function
      const [mintedToken] = tx.moveCall({
        target: `${config.packageId}::leafsii::mint_f`,
        typeArguments: [typeArgs.FTOKEN_TYPE, typeArgs.XTOKEN_TYPE, typeArgs.SUI_TYPE],
        arguments: [protocolArg, poolArg, splitCoin]
      })

      // Transfer minted tokens to user
      tx.transferObjects([mintedToken], tx.pure.address(userAddress))

      tx.setGasBudget(100000000)
      tx.setSender(userAddress)

      const bytes = await tx.build({ client })
      const transactionBlockBytes = toBase64(bytes)

      return {
        transactionBlockBytes,
        quoteId: undefined
      }
    },

    async redeemFToken(params: OperationParams): Promise<BuildResult> {
      const { amount, userAddress } = params

      const config = await getTransactionBuildInfo(network)
      const tx = new Transaction()
      const typeArgs = getTypeArguments(config)

      // Get coin metadata for FTOKEN to determine decimals
      const ftokenMetadata = await client.getCoinMetadata({ coinType: typeArgs.FTOKEN_TYPE })
      const decimals = ftokenMetadata?.decimals ?? 9
      const amountUnits = BigInt(Math.round(parseFloat(amount) * (10 ** decimals)))

      // Use helper functions to reduce duplication
      const [{ protocolArg, poolArg }, splitCoin] = await Promise.all([
        resolveSharedObjects(config, tx),
        handleTokenCoins(tx, userAddress, typeArgs.FTOKEN_TYPE, amountUnits, 'FTOKEN')
      ])

      // Call redeem_f function
      const [redeemedToken] = tx.moveCall({
        target: `${config.packageId}::leafsii::redeem_f`,
        typeArguments: [typeArgs.FTOKEN_TYPE, typeArgs.XTOKEN_TYPE, typeArgs.SUI_TYPE],
        arguments: [protocolArg, poolArg, splitCoin]
      })

      // Transfer redeemed tokens to user
      tx.transferObjects([redeemedToken], tx.pure.address(userAddress))

      tx.setGasBudget(100000000)
      tx.setSender(userAddress)

      const bytes = await tx.build({ client })
      const transactionBlockBytes = toBase64(bytes)

      return {
        transactionBlockBytes,
        quoteId: undefined
      }
    },

    async mintXToken(params: OperationParams): Promise<BuildResult> {
      const { amount, userAddress } = params

      const config = await getTransactionBuildInfo()
      const tx = new Transaction()
      const amountUnits = BigInt(Math.round(parseFloat(amount) * 1e9))

      // Use helper functions to reduce duplication
      const [{ protocolArg, poolArg }, splitCoin, typeArgs] = await Promise.all([
        resolveSharedObjects(config, tx),
        handleSuiCoins(tx, userAddress, amountUnits),
        Promise.resolve(getTypeArguments(config))
      ])

      // Call mint_x function
      const [mintedToken] = tx.moveCall({
        target: `${config.packageId}::leafsii::mint_x`,
        typeArguments: [typeArgs.FTOKEN_TYPE, typeArgs.XTOKEN_TYPE, typeArgs.SUI_TYPE],
        arguments: [protocolArg, poolArg, splitCoin]
      })

      // Transfer minted tokens to user
      tx.transferObjects([mintedToken], tx.pure.address(userAddress))

      tx.setGasBudget(100000000)
      tx.setSender(userAddress)

      const bytes = await tx.build({ client })
      const transactionBlockBytes = toBase64(bytes)

      return {
        transactionBlockBytes,
        quoteId: undefined
      }
    },

    async redeemXToken(params: OperationParams): Promise<BuildResult> {
      const { amount, userAddress } = params

      const config = await getTransactionBuildInfo()
      const tx = new Transaction()
      const typeArgs = getTypeArguments(config)

      // Get coin metadata for XTOKEN to determine decimals
      const xtokenMetadata = await client.getCoinMetadata({ coinType: typeArgs.XTOKEN_TYPE })
      const decimals = xtokenMetadata?.decimals ?? 9
      const amountUnits = BigInt(Math.round(parseFloat(amount) * (10 ** decimals)))

      // Use helper functions to reduce duplication
      const [{ protocolArg, poolArg }, splitCoin] = await Promise.all([
        resolveSharedObjects(config, tx),
        handleTokenCoins(tx, userAddress, typeArgs.XTOKEN_TYPE, amountUnits, 'XTOKEN')
      ])

      // Call redeem_x function
      const [redeemedToken] = tx.moveCall({
        target: `${config.packageId}::leafsii::redeem_x`,
        typeArguments: [typeArgs.FTOKEN_TYPE, typeArgs.XTOKEN_TYPE, typeArgs.SUI_TYPE],
        arguments: [protocolArg, poolArg, splitCoin]
      })

      // Transfer redeemed tokens to user
      tx.transferObjects([redeemedToken], tx.pure.address(userAddress))

      tx.setGasBudget(100000000)
      tx.setSender(userAddress)

      const bytes = await tx.build({ client })
      const transactionBlockBytes = toBase64(bytes)

      return {
        transactionBlockBytes,
        quoteId: undefined
      }
    }
  }
}

export type { TxAction, TokenType, BuildMintRedeemParams, BuildResult, OperationParams, TransactionBuildInfo } from './types'
export type { BridgeRedeemParams } from './types'
export { getTransactionBuildInfo, clearBuildInfoCache } from './config'
