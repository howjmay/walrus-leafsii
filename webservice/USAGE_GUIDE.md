# Cross-Chain Collateral Usage Guide

## Quick Start

This guide walks you through using the cross-chain collateral system with ETH on Ethereum as collateral for minting fETH/xETH tokens on Sui.

## Architecture Overview

```
┌──────────────┐     ┌─────────────┐     ┌──────────────┐
│   Ethereum   │────>│   Walrus    │<────│     Sui      │
│    Vault     │     │ Checkpoints │     │   Protocol   │
└──────────────┘     └─────────────┘     └──────────────┘
       ↑                    ↑                     │
       │                    │                     │
       │              ┌─────────────┐            │
       │              │   Monitor   │            │
       │              │   Service   │            │
       │              └─────────────┘            │
       │                                         │
       └─────────── Vouchers (Withdrawal) ───────┘
```

## Prerequisites

1. **Backend API** running on `localhost:8080`
2. **Monitor service** running (generates checkpoints)
3. **Frontend** running on `localhost:5173`
4. **Sui wallet** connected (for minting)
5. **Ethereum wallet** (for deposits/withdrawals)

## Step-by-Step Usage

### 1. Start the Backend API

```bash
cd backend
go run cmd/api/main.go
```

Expected output:
```
INFO  Starting FX Protocol API server
INFO  Database initialized
INFO  Cache connection established
INFO  Cross-chain service initialized  vaults=1
INFO  API server starting  addr=:8080
```

### 2. Start the Monitor Service

The monitor simulates vault events and publishes checkpoints every 2 minutes:

```bash
cd backend
go run cmd/monitor/main.go
```

Expected output:
```
INFO  Starting Cross-Chain Monitor
INFO  Monitor started  vaults=1
INFO  Starting indexer  chain=ethereum asset=ETH
INFO  Checkpoint submitted  chain=ethereum asset=ETH update_id=1 block=100
```

### 3. Verify API Endpoints

Test that the cross-chain endpoints are working:

```bash
# Check markets (should show crosschain-eth)
curl http://localhost:8080/v1/markets | jq '.[] | select(.id=="crosschain-eth")'

# Check latest checkpoint
curl 'http://localhost:8080/v1/crosschain/checkpoint?chainId=ethereum&asset=ETH' | jq

# Check vault info
curl 'http://localhost:8080/v1/crosschain/vault?chainId=ethereum&asset=ETH' | jq

# Check collateral params
curl 'http://localhost:8080/v1/crosschain/params?chainId=ethereum&asset=ETH' | jq
```

Expected checkpoint response:
```json
{
  "checkpoint": {
    "updateId": 1,
    "chainId": "ethereum",
    "asset": "ETH",
    "blockNumber": 100,
    "totalShares": "0.5",
    "index": "1.0001",
    "balancesRoot": "0x...",
    "proofType": "zk",
    "walrusBlobId": "bafy...",
    "status": "verified"
  }
}
```

### 4. Frontend Integration

#### A. Add Components to Your Trade Page

Update `frontend/src/pages/Trade.tsx`:

```tsx
import {
  CrossChainBalance,
  CrossChainDeposit,
  VoucherManager,
  WalrusCheckpointInfo
} from '@/components/CrossChain'

// In your component
function Trade() {
  const [selectedMarket, setSelectedMarket] = useState<Market | null>(null)
  const userAddress = useCurrentAccount()?.address

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {/* Left Column */}
      <div className="space-y-6">
        {/* Market Selector */}
        <MarketSelector
          onSelect={setSelectedMarket}
        />

        {/* Cross-Chain Deposit (only for cross-chain markets) */}
        {selectedMarket?.collateralType === 'crosschain' && userAddress && (
          <CrossChainDeposit
            market={selectedMarket}
            userAddress={userAddress}
          />
        )}

        {/* Checkpoint Info */}
        {selectedMarket?.collateralType === 'crosschain' && (
          <WalrusCheckpointInfo
            chainId="ethereum"
            asset="ETH"
          />
        )}
      </div>

      {/* Right Column */}
      <div className="space-y-6">
        {/* Cross-Chain Balance */}
        {userAddress && (
          <CrossChainBalance
            suiOwner={userAddress}
            chainId="ethereum"
            asset="ETH"
            showEmpty
          />
        )}

        {/* Regular Mint/Redeem */}
        <MintRedeemTab market={selectedMarket} />

        {/* Voucher Manager */}
        {userAddress && (
          <VoucherManager
            suiOwner={userAddress}
            chainId="ethereum"
            asset="ETH"
          />
        )}
      </div>
    </div>
  )
}
```

#### B. Add to Portfolio/Positions Page

Show cross-chain balances alongside regular positions:

```tsx
import { CrossChainBalanceList } from '@/components/CrossChain'

function Portfolio() {
  const userAddress = useCurrentAccount()?.address

  return (
    <div className="space-y-6">
      <h2>Your Positions</h2>

      {/* Native Sui positions */}
      <NativePositions address={userAddress} />

      {/* Cross-chain collateral */}
      {userAddress && (
        <CrossChainBalanceList suiOwner={userAddress} />
      )}
    </div>
  )
}
```

### 5. User Flow: Depositing ETH

#### Step 1: Select Cross-Chain Market

In the UI, select "Ethereum Cross-Chain Vault" from the markets dropdown.

#### Step 2: View Deposit Instructions

The `CrossChainDeposit` component will show:
- Vault address on Ethereum
- Your Sui address (to include in memo)
- Amount input

#### Step 3: Send ETH on Ethereum

Using your Ethereum wallet (MetaMask, etc.):

```
To: 0x0000... (vault address)
Amount: 1.0 ETH
Data/Memo: 0xYourSuiAddress
```

#### Step 4: Wait for Checkpoint

- Monitor processes the deposit event (~2-5 min)
- Checkpoint is submitted to Walrus and verified
- Your balance appears in the `CrossChainBalance` component

#### Step 5: Mint Tokens on Sui

Once your balance shows:
1. Go to Mint/Redeem tab
2. Select fETH or xETH
3. Enter amount
4. Sign transaction on Sui

Your ETH collateral on Ethereum is now backing fETH/xETH on Sui!

### 6. User Flow: Withdrawing ETH

#### Step 1: Burn Tokens (if needed)

If you have minted tokens, redeem them first to free up collateral.

#### Step 2: Create Withdrawal Voucher

In the `VoucherManager` component:
1. Click "Create Voucher"
2. Enter shares to withdraw
3. Voucher is created with 7-day expiry

#### Step 3: Download Voucher

Click the download button to save voucher JSON:
```json
{
  "voucherId": "voucher_...",
  "suiOwner": "0x...",
  "chainId": "ethereum",
  "asset": "ETH",
  "shares": "0.5",
  "expiry": "...",
  ...
}
```

#### Step 4: Redeem on Ethereum

Using the vault redemption interface:
1. Upload voucher JSON
2. Sign with your Ethereum wallet
3. Receive ETH to your Ethereum address

### 7. Monitoring Checkpoints

The `WalrusCheckpointInfo` component shows:

- **Update ID**: Monotonic checkpoint counter
- **Block Number**: Latest indexed Ethereum block
- **Index**: Current yield multiplier (>1.0 means earning)
- **Age**: Time since last checkpoint
- **Status**: Verified/Pending

**Warning Signs:**
- ⚠️ **Stale** (>10 min old): New mints may be restricted
- ⏳ **Pending**: Checkpoint not yet verified

### 8. Understanding Collateral Parameters

Query the parameters:

```bash
curl 'http://localhost:8080/v1/crosschain/params?chainId=ethereum&asset=ETH' | jq
```

```json
{
  "params": {
    "chainId": "ethereum",
    "asset": "ETH",
    "ltv": "0.65",                    // Max 65% loan-to-value
    "maintenanceThreshold": "0.72",   // Liquidation at 72%
    "liquidationPenalty": "0.06",     // 6% penalty
    "oracleHaircut": "0.02",          // 2% price haircut
    "stalenessHardCap": "3600000000000", // 60 min max age
    "active": true
  }
}
```

**What this means:**
- You can mint up to 65% of your collateral value
- If your collateral ratio drops below 72%, you get liquidated
- Liquidators receive a 6% bonus
- Collateral price has a 2% safety haircut

## Troubleshooting

### No Checkpoint Available

**Symptom**: `WalrusCheckpointInfo` shows "No checkpoint available"

**Solution**:
1. Make sure monitor service is running
2. Wait 2-5 minutes for first checkpoint
3. Check monitor logs for errors

### Checkpoint is Stale

**Symptom**: Checkpoint age > 10 minutes, orange warning

**Solution**:
1. Check if monitor service is still running
2. Check monitor logs for errors
3. Restart monitor service if needed

### Balance Not Showing

**Symptom**: Deposited ETH but balance shows zero

**Solution**:
1. Wait for next checkpoint (2-5 min)
2. Check that Sui address was included in deposit memo
3. Verify deposit on Etherscan
4. Check monitor logs for deposit event

### Cannot Mint Tokens

**Symptom**: Mint button disabled or error

**Possible Causes**:
1. Insufficient collateral balance
2. Checkpoint too stale (>60 min)
3. Market is paused
4. LTV limit reached

**Solution**:
1. Check cross-chain balance
2. Check checkpoint age
3. Try again in 2-5 minutes after fresh checkpoint

## Development vs Production

### Current Implementation (MVP)

- ✅ Backend API with all endpoints
- ✅ Monitor service with mock event indexing
- ✅ Frontend components
- ✅ Checkpoint generation and submission
- ⚠️ **Mock proof generation** (not real zk/SPV)
- ⚠️ **Simulated deposits** (no real Ethereum integration)
- ⚠️ **Auto-verified checkpoints** (no on-chain proof verification)

### Production Requirements

1. **Real Ethereum Integration**
   - Connect to Ethereum RPC node
   - Index actual vault events
   - Query real vault state

2. **Proof System**
   - Generate real zk-SNARK proofs or SPV bundles
   - Verify proofs on Sui
   - Bond monitors with slashing

3. **Walrus Integration**
   - Publish checkpoints to Walrus storage
   - Retrieve Walrus certificates
   - Verify Walrus blob availability

4. **Vault Contracts**
   - Deploy EVM vault contract on Ethereum
   - Implement deposit/withdraw/strategy logic
   - Add voucher redemption functionality

5. **Security**
   - Audit all contracts
   - Add rate limits and circuit breakers
   - Implement monitor bonding and slashing

## API Reference

### GET /v1/crosschain/checkpoint

Get latest verified checkpoint.

**Query Params:**
- `chainId`: ethereum
- `asset`: ETH

**Response:**
```json
{
  "checkpoint": { ...checkpoint object... }
}
```

### POST /v1/crosschain/checkpoint

Submit new checkpoint (monitor only).

**Body:**
```json
{
  "chainId": "ethereum",
  "asset": "ETH",
  "vault": "0x...",
  "blockNumber": 12345,
  "blockHash": "0x...",
  "totalShares": "10.5",
  "index": "1.02",
  "balancesRoot": "0x...",
  "proofType": "zk",
  "proofBlob": "base64...",
  "walrusBlobId": "bafy..."
}
```

### GET /v1/crosschain/balance

Get user's cross-chain balance.

**Query Params:**
- `suiOwner`: 0x...
- `chainId`: ethereum
- `asset`: ETH

**Response:**
```json
{
  "balance": {
    "suiOwner": "0x...",
    "chainId": "ethereum",
    "asset": "ETH",
    "shares": "10.5",
    "index": "1.02",
    "value": "10.71",
    "collateralUsd": "30528.3"
  }
}
```

### POST /v1/crosschain/voucher

Create withdrawal voucher.

**Body:**
```json
{
  "suiOwner": "0x...",
  "chainId": "ethereum",
  "asset": "ETH",
  "shares": "5.0",
  "expiry": 1737360000
}
```

**Response:**
```json
{
  "voucherId": "voucher_...",
  "status": "pending"
}
```

### GET /v1/crosschain/vouchers

List user's vouchers.

**Query Params:**
- `suiOwner`: 0x...

**Response:**
```json
{
  "vouchers": [ ...array of vouchers... ]
}
```

## Next Steps

1. **Test the Full Flow**
   - Start backend and monitor
   - Open frontend
   - Select cross-chain market
   - Create mock deposit (via monitor logs)
   - Verify balance appears
   - Create voucher

2. **Integrate with Your UI**
   - Add components to your pages
   - Style to match your design
   - Add loading states and error handling

3. **Prepare for Production**
   - Set up Ethereum RPC node
   - Deploy vault contracts
   - Implement real proof generation
   - Integrate Walrus SDK
   - Security audit

## Support

- **Documentation**: See `CROSSCHAIN_IMPLEMENTATION.md`
- **Issues**: Check backend logs and browser console
- **Questions**: Review the architecture and data flow

---

**Status**: MVP Complete ✅
**Last Updated**: January 20, 2025
