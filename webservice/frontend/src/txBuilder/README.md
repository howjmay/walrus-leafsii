# TxBuilder

A TypeScript implementation of the Leafsii transaction builder with full PTB (Programmable Transaction Block) support.

## Features

The txBuilder provides the following operations:

- ✅ **mintFToken**: Mint FTOKEN from SUI
- ✅ **mintXToken**: Mint XTOKEN from SUI
- ✅ **redeemFToken**: Redeem FTOKEN to SUI
- ✅ **redeemXToken**: Redeem XTOKEN to SUI

## Structure

```
src/txBuilder/
├── index.ts                      # Main implementation
├── types.ts                      # Type definitions
├── config.ts                     # API configuration management
├── txBuilder.unit.test.ts        # Unit tests
└── txBuilder.integration.test.ts # Integration tests
```

## Usage

### Basic Example

```typescript
import { SuiClient } from '@mysten/sui/client'
import { createTxBuilder } from './index'

const client = new SuiClient({ url: 'https://fullnode.mainnet.sui.io' })
const txBuilder = createTxBuilder(client)

// Mint FTOKEN
const result = await txBuilder.mintFToken({
  amount: '0.1',
  userAddress: '0x742d35...',
  chain: 'mainnet'
})

// Returns base64 encoded transaction bytes ready for signing
console.log(result.transactionBlockBytes)
```

### Available Methods

```typescript
// Mint tokens from SUI
await txBuilder.mintFToken({ amount, userAddress, chain })
await txBuilder.mintXToken({ amount, userAddress, chain })

// Redeem tokens back to SUI
await txBuilder.redeemFToken({ amount, userAddress, chain })
await txBuilder.redeemXToken({ amount, userAddress, chain })

// Combined API
await txBuilder.buildMintRedeem({
  action: 'mint',
  tokenType: 'f',
  amount,
  userAddress,
  chain
})
```

## Testing

### Unit Tests

Run unit tests to validate logic without requiring live services:

```bash
npm run test -- txBuilder.unit.test.ts
```

### Integration Tests

Run integration tests that use real configuration and PTB construction:

```bash
npm run test -- txBuilder.integration.test.ts
```

### Test Prerequisites

For integration tests, ensure:

1. **Backend API** is running on `http://localhost:8080`
2. **Protocol contracts** are deployed and configured
3. **API endpoint** `/v1/protocol/build-info` returns valid configuration

## Implementation Features

### ✅ Correct PTB Construction

- **Shared Objects**: Resolves `initialSharedVersion` from chain state
- **Coin Handling**: Implements merge-then-split logic matching Go backend
- **Type Arguments**: Uses exact same order as Go: `[FTOKEN, XTOKEN, SUI]`
- **Amount Conversion**: SUI (9 decimals) for mint, dynamic decimals for redeem

### ✅ Dynamic Configuration

- All contract IDs fetched from backend API
- No hardcoded addresses
- Proper caching with cache invalidation

### ✅ Error Handling

- Balance validation
- Shared object validation
- API error handling
- Invalid parameter handling

## Key Features

- **Dynamic Configuration**: Contract addresses fetched from backend API
- **PTB Support**: Full Programmable Transaction Block implementation
- **Coin Management**: Automatic coin merging and splitting
- **Error Handling**: Comprehensive validation and error reporting
- **Type Safety**: Full TypeScript support with proper types

## API Reference

### Types

```typescript
interface OperationParams {
  amount: string      // Amount in human-readable format (e.g., "0.1")
  userAddress: string // Sui address
  chain: string      // Network identifier
}

interface BuildResult {
  transactionBlockBytes: string // Base64 encoded transaction
  quoteId?: string             // Optional quote identifier
}
```

### Error Handling

The txBuilder throws descriptive errors for common issues:
- `"Insufficient SUI balance"` - Not enough SUI for minting
- `"Insufficient [TOKEN] balance"` - Not enough tokens for redemption
- `"Protocol object is not shared"` - Contract deployment issue
- `"Failed to fetch transaction build info"` - Backend API unavailable

The txBuilder is production-ready and fully compatible with the Go backend implementation.