# Cross-Chain Collateral Implementation

## Overview

This document describes the implementation of cross-chain collateral support for the FXN-style protocol, enabling base tokens (e.g., ETH on Ethereum) to be used as collateral via Walrus for cross-chain state publication. The implementation follows the architectural proposal with:

- **Minimal vaults on other chains** (Ethereum) that hold and stake base tokens
- **All minting and risk logic on Sui**
- **Walrus for cross-chain state publication** with client proofs
- **Self-custody withdrawals** via signed vouchers

## Architecture

### Components

1. **Other Chains (Ethereum)**: Vault contracts that accept deposits, stake tokens, emit events
2. **Sui**: Core protocol for minting fETH/xETH tokens, risk calculations, liquidations
3. **Walrus**: Data availability layer carrying checkpoints with merkle roots and proofs
4. **Offchain Monitors**: Index vault events, compute state, publish to Walrus, fulfill withdrawals

### Key Features

- **Client Proofs**: zk or SPV proofs verify vault state on Sui (no trusted signers for deposits)
- **Self-Withdrawals**: Users burn tokens on Sui, get vouchers, redeem directly on Ethereum
- **Conservative Parameters**: ETH has 65% LTV, 72% maintenance, 6% liquidation penalty
- **Walrus Integration**: All cross-chain state flows through Walrus with cryptographic proofs

## Backend Implementation

### File Structure

```
backend/
├── internal/
│   ├── crosschain/
│   │   ├── types.go          # Data structures for checkpoints, vouchers, params
│   │   └── service.go        # Cross-chain state management service
│   ├── api/
│   │   ├── crosschain_types.go     # API request/response types
│   │   ├── crosschain_handlers.go  # HTTP handlers
│   │   └── routes.go              # Routes (updated)
│   └── markets/
│       ├── types.go           # Market IDs (added crosschain-eth)
│       └── service.go         # Market service (added ETH market)
└── cmd/
    └── api/
        └── main.go            # Initialization (added crosschain service)
```

### Data Models

#### WalrusCheckpoint
```go
type WalrusCheckpoint struct {
    UpdateID     uint64          // Monotonic update counter
    ChainID      ChainID         // e.g., "ethereum"
    Vault        string          // Vault contract address
    Asset        string          // "ETH"
    BlockNumber  uint64          // Finalized block
    TotalShares  decimal.Decimal // Total vault shares
    Index        decimal.Decimal // Assets per share (for staking yield)
    BalancesRoot string          // Merkle root of user balances
    ProofBlob    []byte          // zk or SPV proof
    WalrusBlobID string          // Walrus storage ID
    Status       CheckpointStatus // pending/verified/rejected
}
```

#### WithdrawalVoucher
```go
type WithdrawalVoucher struct {
    VoucherID     string
    SuiOwner      string
    ChainID       ChainID
    Asset         string
    Shares        decimal.Decimal
    Nonce         uint64
    Expiry        time.Time
    UserSignature string          // User signs voucher
    ProofBlob     []byte           // zk proof of Sui state or notary sigs
    Status        VoucherStatus    // pending/spent/settled
    TxHash        string           // Fulfillment tx on target chain
}
```

#### CollateralParams
```go
type CollateralParams struct {
    ChainID              ChainID
    Asset                string
    LTV                  decimal.Decimal // 0.65 for ETH
    MaintenanceThreshold decimal.Decimal // 0.72 for ETH
    LiquidationPenalty   decimal.Decimal // 0.06 for ETH
    OracleHaircut        decimal.Decimal // 0.02 base haircut
    StalenessHardCap     time.Duration   // 60 minutes
    MintRateLimit        decimal.Decimal
    WithdrawRateLimit    decimal.Decimal
    Active               bool
}
```

### API Endpoints

All endpoints are under `/v1/crosschain`:

#### Checkpoints
- `GET /checkpoint?chainId=ethereum&asset=ETH` - Get latest verified checkpoint
- `POST /checkpoint` - Submit new checkpoint (monitor only)
  ```json
  {
    "chainId": "ethereum",
    "asset": "ETH",
    "vault": "0x...",
    "blockNumber": 12345678,
    "blockHash": "0x...",
    "totalShares": "1000.5",
    "index": "1.02",
    "balancesRoot": "0x...",
    "proofType": "zk",
    "proofBlob": "base64...",
    "walrusBlobId": "bafy..."
  }
  ```

#### Balances
- `GET /balance?suiOwner=0x...&chainId=ethereum&asset=ETH` - Get user's cross-chain balance
  ```json
  {
    "balance": {
      "suiOwner": "0x...",
      "chainId": "ethereum",
      "asset": "ETH",
      "shares": "10.5",
      "index": "1.02",
      "value": "10.71",
      "collateralUsd": "30528.3",
      "lastCheckpointId": 42,
      "updatedAt": "2025-01-20T10:30:00Z"
    }
  }
  ```

#### Vouchers
- `GET /voucher?voucherId=voucher_123` - Get specific voucher
- `GET /vouchers?suiOwner=0x...` - List user's vouchers
- `POST /voucher` - Create withdrawal voucher
  ```json
  {
    "suiOwner": "0x...",
    "chainId": "ethereum",
    "asset": "ETH",
    "shares": "5.0",
    "expiry": 1737360000
  }
  ```

#### Configuration
- `GET /params?chainId=ethereum&asset=ETH` - Get collateral parameters
- `GET /vault?chainId=ethereum&asset=ETH` - Get vault info

### Markets

New market added: `crosschain-eth`

```json
{
  "id": "crosschain-eth",
  "label": "Ethereum Cross-Chain Vault",
  "pairSymbol": "fETH/xETH",
  "stableSymbol": "fETH",
  "leverageSymbol": "xETH",
  "collateralSymbol": "ETH",
  "collateralType": "crosschain",
  "collateralHighlights": [
    "Native ETH staked on Ethereum mainnet",
    "Verified via Walrus + zk light client proofs",
    "Self-custody withdrawals with signed vouchers",
    "Conservative 65% LTV, 6% liquidation penalty"
  ],
  "px": 2850000000,
  "cr": "1.38",
  "targetCr": "1.38",
  "reserves": "8500000",
  "supplyStable": "6159420.29",
  "supplyLeverage": "2340579.71",
  "mode": "crosschain",
  "feedUrl": "https://walrus.xyz/api/feeds/eth-vault",
  "proofCid": "bafyEthereumVaultProof",
  "snapshotUrl": "https://walrus.storage/eth/latest.json"
}
```

## Frontend Implementation Guide

### 1. Market Selection

Update the market selector to show the new cross-chain markets:

```tsx
// In Markets or CollateralView component
const markets = await fetch('/v1/markets').then(r => r.json())

// Filter by collateral type
const crossChainMarkets = markets.filter(m => m.collateralType === 'crosschain')
const nativeMarkets = markets.filter(m => m.collateralType === 'native')
```

### 2. Cross-Chain Deposit Flow

Create a new component `CrossChainDeposit.tsx`:

```tsx
interface CrossChainDepositProps {
  market: Market  // ETH market
  userAddress: string
}

export function CrossChainDeposit({ market, userAddress }: CrossChainDepositProps) {
  const [amount, setAmount] = useState('')
  const [depositAddress, setDepositAddress] = useState('')

  // Get vault info
  const vault = await fetch(`/v1/crosschain/vault?chainId=ethereum&asset=ETH`)
    .then(r => r.json())

  // Show deposit instructions
  return (
    <div>
      <h3>Deposit ETH to Ethereum Vault</h3>
      <p>Send ETH to: <code>{vault.vaultAddress}</code></p>
      <p>Include your Sui address in the memo: <code>{userAddress}</code></p>
      <Input value={amount} onChange={e => setAmount(e.target.value)} />
      <Button onClick={handleDeposit}>Generate Deposit Intent</Button>

      {/* Show pending balance updates */}
      <PendingDeposits suiOwner={userAddress} chainId="ethereum" asset="ETH" />
    </div>
  )
}
```

### 3. Balance Display

Show cross-chain balances in the portfolio:

```tsx
export function CrossChainBalance({ suiOwner }: { suiOwner: string }) {
  const [balance, setBalance] = useState<CrossChainBalance | null>(null)

  useEffect(() => {
    fetch(`/v1/crosschain/balance?suiOwner=${suiOwner}&chainId=ethereum&asset=ETH`)
      .then(r => r.json())
      .then(data => setBalance(data.balance))
  }, [suiOwner])

  if (!balance || balance.shares.isZero()) return null

  return (
    <div className="border rounded p-4">
      <h4>ETH on Ethereum</h4>
      <div>Shares: {balance.shares.toString()}</div>
      <div>Value: {balance.value.toString()} ETH</div>
      <div>Collateral: ${balance.collateralUsd.toString()}</div>
      <div className="text-sm text-gray-500">
        Last updated: {new Date(balance.updatedAt).toLocaleString()}
      </div>
    </div>
  )
}
```

### 4. Mint/Redeem with Cross-Chain Collateral

Update `MintRedeemTab.tsx` to handle cross-chain markets:

```tsx
// When user selects crosschain-eth market
const handleMint = async () => {
  // Check if user has cross-chain balance
  const balance = await fetch(`/v1/crosschain/balance?suiOwner=${userAddress}&chainId=ethereum&asset=ETH`)
    .then(r => r.json())

  if (balance.balance.collateralUsd.lt(requiredCollateral)) {
    toast.error('Insufficient cross-chain collateral')
    return
  }

  // Build transaction as normal
  const tx = await fetch('/v1/transactions/build', {
    method: 'POST',
    body: JSON.stringify({
      action: 'mint',
      tokenType: 'ftoken',
      amount: amount,
      marketId: 'crosschain-eth'
    })
  })

  // If market is cross-chain, tx will include Walrus metadata
  if (tx.metadata.walrusProofCid) {
    // Show Walrus details (similar to stock market flow)
    toast.info(`Checkpoint: ${tx.metadata.walrusProofCid}`)
  }
}
```

### 5. Withdrawal Voucher Flow

Create `VoucherManager.tsx`:

```tsx
export function VoucherManager({ suiOwner }: { suiOwner: string }) {
  const [vouchers, setVouchers] = useState<WithdrawalVoucher[]>([])

  // Fetch user's vouchers
  useEffect(() => {
    fetch(`/v1/crosschain/vouchers?suiOwner=${suiOwner}`)
      .then(r => r.json())
      .then(data => setVouchers(data.vouchers))
  }, [suiOwner])

  const createVoucher = async (shares: string) => {
    const expiry = Math.floor(Date.now() / 1000) + 86400 * 7 // 7 days

    const response = await fetch('/v1/crosschain/voucher', {
      method: 'POST',
      body: JSON.stringify({
        suiOwner,
        chainId: 'ethereum',
        asset: 'ETH',
        shares,
        expiry
      })
    })

    const { voucherId } = await response.json()
    toast.success(`Voucher created: ${voucherId}`)
    // Refresh list
  }

  return (
    <div>
      <h3>Withdrawal Vouchers</h3>
      <Button onClick={() => createVoucher('5.0')}>Create Voucher</Button>

      <div className="space-y-2 mt-4">
        {vouchers.map(voucher => (
          <VoucherCard key={voucher.voucherId} voucher={voucher} />
        ))}
      </div>
    </div>
  )
}

function VoucherCard({ voucher }: { voucher: WithdrawalVoucher }) {
  const handleRedeem = () => {
    // User redeems on Ethereum using the voucher
    window.open(`https://app.example.com/redeem/${voucher.voucherId}`, '_blank')
  }

  return (
    <div className="border rounded p-3">
      <div className="font-mono text-sm">{voucher.voucherId}</div>
      <div>{voucher.shares} shares ({voucher.asset})</div>
      <div className="text-sm">
        Status: <Badge>{voucher.status}</Badge>
      </div>
      {voucher.status === 'pending' && (
        <Button size="sm" onClick={handleRedeem}>
          Redeem on {voucher.chainId}
        </Button>
      )}
      {voucher.status === 'spent' && voucher.txHash && (
        <a
          href={`https://etherscan.io/tx/${voucher.txHash}`}
          target="_blank"
          className="text-blue-500 text-sm"
        >
          View Transaction →
        </a>
      )}
    </div>
  )
}
```

### 6. Walrus Checkpoint Display

Update the Walrus info box to show checkpoint details:

```tsx
export function WalrusCheckpointInfo({ chainId, asset }: { chainId: string, asset: string }) {
  const [checkpoint, setCheckpoint] = useState<WalrusCheckpoint | null>(null)

  useEffect(() => {
    fetch(`/v1/crosschain/checkpoint?chainId=${chainId}&asset=${asset}`)
      .then(r => r.json())
      .then(data => setCheckpoint(data.checkpoint))
  }, [chainId, asset])

  if (!checkpoint) return null

  const age = Date.now() - new Date(checkpoint.timestamp).getTime()
  const ageMinutes = Math.floor(age / 60000)

  return (
    <div className="bg-blue-50 border border-blue-200 rounded p-3 text-sm">
      <div className="font-semibold">Latest Checkpoint</div>
      <div className="space-y-1 mt-2">
        <div>Update ID: {checkpoint.updateId}</div>
        <div>Block: {checkpoint.blockNumber}</div>
        <div>Index: {checkpoint.index.toString()}</div>
        <div className="text-xs text-gray-600">
          Age: {ageMinutes} minutes
          {ageMinutes > 10 && <span className="text-orange-600 ml-2">⚠️ Stale</span>}
        </div>
        {checkpoint.walrusBlobId && (
          <a
            href={`https://walrus.xyz/blob/${checkpoint.walrusBlobId}`}
            target="_blank"
            className="text-blue-600 flex items-center gap-1"
          >
            View on Walrus <ExternalLink className="w-3 h-3" />
          </a>
        )}
      </div>
    </div>
  )
}
```

## Testing

### Manual Testing

1. **Start the backend**:
   ```bash
   cd backend
   go run cmd/api/main.go
   ```

2. **Test market endpoint**:
   ```bash
   curl http://localhost:8080/v1/markets | jq
   # Should show crosschain-eth market
   ```

3. **Test checkpoint endpoint**:
   ```bash
   curl 'http://localhost:8080/v1/crosschain/checkpoint?chainId=ethereum&asset=ETH' | jq
   # Should return error (no checkpoint yet) or checkpoint if populated
   ```

4. **Test balance endpoint**:
   ```bash
   curl 'http://localhost:8080/v1/crosschain/balance?suiOwner=0x123&chainId=ethereum&asset=ETH' | jq
   # Should return zero balance
   ```

5. **Test voucher creation**:
   ```bash
   curl -X POST http://localhost:8080/v1/crosschain/voucher \
     -H 'Content-Type: application/json' \
     -d '{
       "suiOwner": "0x123",
       "chainId": "ethereum",
       "asset": "ETH",
       "shares": "5.0",
       "expiry": 1737360000
     }' | jq
   ```

6. **Test params endpoint**:
   ```bash
   curl 'http://localhost:8080/v1/crosschain/params?chainId=ethereum&asset=ETH' | jq
   # Should return ETH collateral params (65% LTV, etc.)
   ```

## Next Steps

### Phase 1: MVP (Current)
- ✅ Backend data structures and API endpoints
- ✅ Market integration for ETH
- ✅ In-memory state management
- ⏳ Frontend UI for cross-chain deposits
- ⏳ Frontend voucher management

### Phase 2: Monitor & Proof System
- [ ] Offchain monitor service to index Ethereum vault events
- [ ] Merkle tree generation for user balances
- [ ] ZK or SPV proof generation
- [ ] Automated checkpoint submission to Walrus
- [ ] Proof verification on Sui

### Phase 3: Vault Contracts
- [ ] Ethereum vault contract (deposit, withdraw, strategy)
- [ ] Vault events (Deposit, Withdraw, Rebase)
- [ ] Integration with staking strategies (native, Lido, etc.)
- [ ] Deterministic vault addresses per chain/asset

### Phase 4: Production Readiness
- [ ] Database persistence (replace in-memory storage)
- [ ] Real price oracles integration
- [ ] Rate limiting and circuit breakers
- [ ] Slashing for monitor misbehavior
- [ ] Comprehensive testing (unit, integration, e2e)
- [ ] Security audit

## Security Considerations

1. **Client Proofs**: Currently auto-verified in MVP. Production must verify zk/SPV proofs on Sui
2. **Voucher Replay**: Nonce and voucher ID tracking prevents replay attacks
3. **Expiry**: Vouchers expire to limit attack surface
4. **Rate Limits**: Per-epoch mint/withdraw caps protect against rapid drainage
5. **Staleness**: Checkpoints older than 60min trigger safety measures (no mints, haircuts)
6. **Monitor Bonding**: Monitors should be bonded on Sui with slashing for provable faults

## References

- Original proposal: See conversation history
- Walrus documentation: https://walrus.xyz/docs
- FXN protocol: https://leafsii.xyz
- Sui documentation: https://docs.sui.io

---

**Implementation Date**: January 20, 2025
**Status**: Backend Complete, Frontend Pending
**Next Milestone**: Frontend UI Integration
