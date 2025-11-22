# Leafsii Protocol Monorepo

[![CI](https://github.com/leafsii-dev/webservice/actions/workflows/ci.yml/badge.svg)](https://github.com/leafsii-dev/webservice/actions/workflows/ci.yml)

A complete f(x) protocol v1 implementation on Sui blockchain, featuring a type-safe Go backend and React TypeScript frontend. The backend is non-custodial (never holds private keys) and provides fast read APIs, quote/preview endpoints, event indexing, and WebSocket/SSE for live updates to the React frontend.

## Features

- **Ultra-fast read & preview APIs** with server-side caching and TTL'd quotes
- **Event indexer** for on-chain state (mints/redeems/stake/unstake/claim/rebalance)
- **Live updates** via WebSocket and Server-Sent Events (SSE)
- **Non-custodial** - never handles private keys
- **Type-safe** with strict decimal arithmetic using shopspring/decimal
- **Production-ready** with metrics, logging, and health checks

## Repository Structure

This is a monorepo containing both backend and frontend components:

```
webservice/
├── backend/          # Go backend service
├── frontend/         # React TypeScript frontend
├── .github/          # GitHub Actions CI/CD
└── README.md         # This file
```

### Backend Features

- **Protocol State**: CR, reserves, supply tracking with real-time updates
- **Stability Pool**: Index-based reward accrual system
- **Quote Engine**: TTL-based mint/redeem quotes with slippage protection
- **Event Indexer**: Processes blockchain events and updates database
- **WebSocket Hub**: Real-time updates for frontend

### Frontend Features

- **PancakeSwap-style Trading Interface**: Swap, mint/redeem, staking operations
- **Real-time Charts**: Price charts with multiple timeframes
- **Protocol Metrics**: Live CR, peg deviation, TVL, and reward APR tracking
- **Mobile-first Design**: Responsive design with custom dark theme
- **Wallet Integration**: Sui wallet support with demo mode

### Tech Stack

**Backend:**
- **Language**: Go 1.23
- **Router**: Chi v5
- **Database**: PostgreSQL (primary), Redis (cache + pub/sub)
- **Blockchain**: Sui via pattonkan/sui-go SDK
- **Metrics**: OpenTelemetry + Prometheus
- **Logging**: Zap (structured logging)

**Frontend:**
- **Framework**: React 18 + TypeScript + Vite
- **Styling**: Tailwind CSS with custom design tokens
- **State Management**: Zustand for local state, TanStack Query for server state
- **Blockchain**: Sui TypeScript SDK + dApp Kit for wallet integration
- **Charts**: lightweight-charts for price/metrics visualization

## API Endpoints

### Protocol & Health
- `GET /v1/protocol/state` - Current protocol state (CR, reserves, supplies)
- `GET /v1/protocol/health` - System health status

### Quotes & Previews  
- `GET /v1/quotes/mint?amountR=100` - Get mint quote for Sui amount
- `GET /v1/quotes/redeemF?amountF=100` - Get redeem quote for fToken amount

### Stability Pool
- `GET /v1/sp/index` - Current SP index, TVL, and APR
- `GET /v1/sp/user/{address}` - User's SP position and claimable rewards

### User Portfolio
- `GET /v1/users/{address}/positions` - User balances and positions

### Live Updates
- `GET /v1/stream` - Server-Sent Events stream
- `GET /v1/ws` - WebSocket connection for real-time updates

### Operations
- `GET /healthz` - Health check
- `GET /metrics` - Prometheus metrics

## Getting Started

### Prerequisites

**Backend:**
- Go 1.23+
- PostgreSQL 15+
- Redis 7+
- Docker & Docker Compose (for local development)

**Frontend:**
- Node.js 18+ (Node 20 recommended)
- npm or yarn

### Local Development

#### Backend Setup

1. **Clone and navigate to backend**:
   ```bash
   git clone <repo-url>
   cd backend
   go mod download
   ```

2. **Start dependencies**:
   ```bash
   docker-compose up postgres redis -d
   ```

3. **Run migrations**:
   ```bash
   go run cmd/migrate/main.go up
   ```

4. **Start services**:
   ```bash
   # API server
   go run cmd/api/main.go

   # Event indexer (in another terminal)
   go run cmd/indexer/main.go
   ```

#### Frontend Setup

1. **Navigate to frontend**:
   ```bash
   cd frontend
   ```

2. **Install dependencies**:
   ```bash
   npm install
   ```

3. **Start development server**:
   ```bash
   npm run dev
   ```

The frontend will be available at `http://localhost:5173` with demo mode enabled by default.

### Environment Variables

**Backend (`backend/.env`):**
```bash
# Environment
LFS_ENV=dev|staging|prod
LFS_HTTP_ADDR=:8080
LFS_PUBLIC_ORIGIN=https://app.fx.xyz

# Sui Configuration (defaults to localnet)
LFS_SUI_RPC_URL=http://localhost:9000
LFS_SUI_WS_URL=wss://localhost:9000
LFS_NETWORK=localnet|testnet|mainnet

# Object IDs are now loaded from init.json:
# - leafsii_package_id (replaces LFS_SUI_OBJECTS_CORE)
# - pool_id (replaces LFS_SUI_OBJECTS_SP)

# Database & Cache
LFS_POSTGRES_DSN=postgres://user:pass@host:5432/fx?sslmode=disable
LFS_REDIS_ADDR=127.0.0.1:6379

# Oracles
LFS_PRICE_ORACLE_URLS=https://api.coingecko.com/api/v3/simple/price
LFS_ORACLE_MAX_AGE=60s

# Security
LFS_RATE_LIMIT_RPM=120
LFS_CORS_ALLOWED_ORIGINS=https://app.fx.xyz
```

**Frontend (`frontend/.env`):**
```bash
# Optional - defaults work for development
VITE_DEFAULT_NETWORK=testnet
VITE_ANALYTICS_ID=your-analytics-id
```

### Docker Deployment

```bash
# Full stack
docker-compose up -d

# Individual services
docker-compose up api          # API server only  
docker-compose up indexer      # Indexer only
```

## Testing

### Backend Testing

```bash
cd backend

# Unit tests
go test ./...

# With coverage and race detection
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Integration tests
make test-integration

# Run via Makefile
make test
```

### Frontend Testing

```bash
cd frontend

# Type checking
npm run typecheck

# Linting
npm run lint

# Build test
npm run build
```

### Full Stack Testing

The CI pipeline runs both backend and frontend tests automatically. You can also test the full stack locally:

```bash
# Backend
cd backend && make ci-test

# Frontend
cd frontend && npm run typecheck && npm run lint && npm run build
```

## Monitoring

### Metrics
- **HTTP requests**: Duration, status codes, throughput
- **Cache performance**: Hit/miss ratios  
- **Oracle data**: Age, staleness tracking
- **Indexer lag**: Blockchain sync status
- **WebSocket connections**: Active connection count

### Health Checks
- `/healthz` - Basic liveness check
- `/readyz` - Readiness check (DB, Redis, etc.)
- Protocol health monitoring for CR violations, oracle staleness

### Logs
Structured JSON logs with:
- Request tracing
- Error context
- Performance metrics
- Security events

## Protocol Calculations

### Collateral Ratio
```
CR = reserves_r / supply_f
```
Where `supply_f` represents the liability (fTokens at 1.0 peg).

### Stability Pool Index
```  
index_new = index_prev + payout_r / total_stake_f
```

Rewards are calculated as:
```
claimable = stake_f * (current_index - index_at_join)
```

### Rebalancing
When `|CR - CR_target| > tolerance`:
- Calculate `f_burn` and `payout_r` amounts
- Update SP index with new rewards
- Maintain target collateral ratio

## Security

- **Rate limiting**: 120 RPM per IP by default
- **CORS**: Configurable allowed origins  
- **Input validation**: All API inputs sanitized
- **No private keys**: Backend never handles wallet private keys
- **Audit logs**: All critical operations logged

## Performance

- **Sub-50ms p95** for cached protocol state queries
- **2-3 second cache TTL** for protocol data
- **30-second quote TTL** with validation
- **Real-time updates** via WebSocket/SSE
- **Database connection pooling** and prepared statements

## Deployment

### Staging
```bash
# Update staging
git push origin develop
# CI/CD automatically deploys to staging environment
```

### Production  
```bash
# Create release
git tag v1.0.0
git push origin v1.0.0
# CI/CD builds and deploys production images
```

### Environment Configuration

Production deployments require:
- Sui mainnet RPC endpoints
- Production database credentials  
- Redis cluster for high availability
- Load balancer configuration
- SSL/TLS certificates

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)  
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Code Style

- Run `golangci-lint run` before committing
- Follow Go best practices and project conventions
- Add tests for new functionality
- Update documentation for API changes

## License

This project is licensed under the MIT License - see the LICENSE file for details.
