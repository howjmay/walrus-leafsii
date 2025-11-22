# Leafsii Protocol Trading Interface

A PancakeSwap-style trading interface built for f(x) protocol v1 on Sui blockchain. This interface provides a comprehensive trading experience with advanced features for minting, redeeming, swapping, and staking operations.

## ✨ Features

### Core Trading
- **Swap**: Token-to-token exchanges with auto-slippage and price impact warnings
- **Mint/Redeem**: f(x) protocol specific operations for minting fToken (β=0) and xToken
- **Staking**: Stability Pool operations with deferred reward claims
- **Limit Orders**: Advanced order types with customizable expiry (feature flagged)
- **TWAP Orders**: Time-weighted average price execution for large trades (feature flagged)

### Analytics & Monitoring
- **Real-time Charts**: Price charts with multiple timeframes using lightweight-charts
- **Protocol Metrics**: Live CR, peg deviation, TVL, and reward APR tracking
- **Position Dashboard**: Complete view of balances, staking positions, and transaction history
- **Health Monitoring**: Protocol health indicators and oracle status

### Advanced Features
- **Deep Linking**: URL state persistence for sharing configurations
- **Responsive Design**: Mobile-first design that works on all screen sizes
- **Dark Theme**: Custom Swirl-style dark theme with purple accent colors
- **Accessibility**: Full keyboard navigation and screen reader support

## 🚀 Quick Start

### Prerequisites
- Node.js 18+ (Node 20 recommended to match CI)
- npm or yarn
- A Sui wallet (Sui Wallet, Suiet, etc.)

### Installation

```bash
# Clone the repository
git clone <repository-url>
cd frontend

# Install dependencies
npm install

# Start development server (demo mode)
npm run dev

# The app will be available at http://localhost:5173
```

### Demo Mode

The application includes a demo mode that showcases all features without requiring wallet connection or blockchain interaction:

- **Mock Data**: All protocol metrics, prices, and user data are simulated
- **Full UI**: Complete interface including all trading tabs and analytics
- **Responsive**: Test mobile and desktop layouts
- **Interactive**: All buttons and forms are functional with mock responses

Demo mode is perfect for:
- Development and testing
- UI/UX review and feedback
- Showcasing features to stakeholders
- Integration testing with frontend components

### Using Demo Mode

1. **Start the Application**: Run `npm run dev` and navigate to `http://localhost:5173`
2. **Connect Demo Wallet**: In demo pages (e.g., `DemoTrade`), use the "Connect Demo Wallet" button inside any Simple* panel (e.g., Simple Swap/Stake) — no browser extension needed. Alternatively, connect a real wallet via the header button.
3. **Explore Features**: All trading tabs (Swap, Mint/Redeem, Stake, Limit, TWAP) are fully functional
4. **Test Transactions**: Execute mock transactions with realistic feedback and notifications
5. **View Analytics**: Charts, protocol metrics, and position data work with live mock data

The demo wallet connection persists across browser sessions and enables all wallet-dependent features like trading, staking, and viewing positions.

### Environment Setup

The app automatically detects and connects to Sui networks:
- **Testnet**: Default network for development and testing
- **Mainnet**: Production network

No additional environment variables are required for basic functionality.

### Wallet Setup

1. Install a compatible Sui wallet extension
2. Create or import a wallet
3. Switch to the desired network (testnet recommended for development)
4. Connect your wallet using the "Connect Wallet" button in the header

## 🏗️ Architecture

### Tech Stack
- **Frontend**: React 18 + TypeScript + Vite
- **Styling**: Tailwind CSS with custom design tokens
- **State Management**: Zustand for local state, TanStack Query for server state
- **Blockchain**: Sui TypeScript SDK + dApp Kit for wallet integration
- **Charts**: lightweight-charts for price/metrics visualization
- **Routing**: React Router with URL state persistence

### Project Structure

```
src/
├── components/          # Reusable UI components
│   ├── Header/         # Navigation and wallet connection
│   ├── Charts/         # Price charts and analytics panels
│   ├── TradeBox/       # Trading interface components
│   ├── Widgets/        # Protocol health and status widgets
│   └── ui/             # Base UI components (buttons, inputs, etc.)
├── hooks/              # Custom React hooks for data fetching
├── lib/                # Utility functions and helpers
├── pages/              # Page components and routing
├── sdk/                # Sui SDK wrapper and protocol client
└── theme/              # Design tokens and styling
```

### Key Components

- **TradePage**: Main trading interface with dual-panel layout
- **Header**: Network selection, wallet connection, and settings
- **TradePanel**: Tabbed interface for different trading operations
- **ChartPanel**: Analytics dashboard with price and protocol metrics
- **ProtocolHealth**: Real-time monitoring of system health indicators

### Demo vs Production Components

This frontend maintains two parallel sets of components to support both a no‑wallet demo flow and the production, wallet/API‑backed flow.

- Simple components (demo/offline):
  - Examples: `SimpleHeader`, `SimpleTradePanel`, `SimpleSwapTab`, `SimpleMintRedeemTab`, `SimpleStakeTab`, `SimpleLimitTab`, `SimpleTWAPTab`, `SimpleMyPositions`.
  - Behavior: Work without a real wallet. These components can enable a “demo wallet” by setting `localStorage['demoWalletConnected'] = 'true'`. They show realistic mock interactions and placeholders when not connected to a real wallet. When a real wallet is connected, some Simple* components will also show live data where available.
  - Data: Use mock clients/hooks where applicable (see `src/sdk/simple-client.ts` and `src/hooks/useSimpleProtocol.ts`).

- Production components (real wallet/API):
  - Examples: `Header`, `TradePanel`, `SwapTab`, `MintRedeemTab`, `StakeTab`, `LimitTab`, `TWAPTab`, `MyPositions`.
  - Behavior: Assume a Mysten wallet via `@mysten/dapp-kit` and gate wallet‑dependent data on `useCurrentAccount()`.
  - Data: Use hooks that hit backend endpoints (e.g., `useUserBalances`, `useSpUser`, `useUserTransactions`).

- Pages:
  - `src/pages/DemoTrade.tsx` uses Simple* components end‑to‑end for a full demo experience.
  - `src/pages/Trade.tsx` is the main page for production. It currently includes `SimpleMyPositions` (transitional) alongside non‑prefixed components; switch to `MyPositions` when removing demo dependencies from this page.

- Feature flags: `src/lib/featureFlags.ts` toggles optional tabs (e.g., Limit, TWAP) commonly enabled during demo.

- Guidance on keeping/removing both:
  - Keep both sets while a no‑wallet demo path is needed for reviews, showcases, or offline development.
  - When retiring demo mode, replace remaining `Simple*` usages in production pages with the non‑prefixed components, and remove `DemoTrade.tsx` and unused Simple* components.
  - Optional consolidation: Introduce a `VITE_DEMO_MODE` flag and switch only the data layer (mock vs API) to reduce duplicate UI.

## 🎨 Design System

### Colors
- **Background**: Deep navy (#0B0F1A) with subtle gradients
- **Cards**: Semi-transparent surfaces with backdrop blur
- **Primary**: Purple (#8B5CF6) with soft glow effects
- **Status**: Success (green), Warning (orange), Danger (red)

### Typography
- **Font**: Inter for clean, modern readability
- **Scale**: 12px (xs) to 32px (display) with consistent line heights
- **Weight**: Regular (400) to Bold (700) for information hierarchy

### Spacing & Layout
- **Grid**: CSS Grid with responsive breakpoints
- **Spacing**: 4px base unit with harmonious scale
- **Radius**: Rounded corners from 8px to 20px
- **Shadows**: Subtle depth with colored borders

## 🔧 Configuration

### Feature Flags

Control experimental features in `src/lib/featureFlags.ts`:

```typescript
export const featureFlags = {
  xtoken: false,    // xToken minting/redeeming
  limit: false,     // Limit orders
  twap: false,      // TWAP orders
  telemetry: false, // Analytics tracking
}
```

### Contract Addresses

Update contract addresses in `src/sdk/types.ts` for different networks:

```typescript
export const CONTRACTS: Record<'mainnet' | 'testnet', ContractConfig> = {
  testnet: {
    packageId: '0x1111...',
    protocolObjectId: '0x2222...',
    // ... other contract IDs
  }
}
```

## 📱 Responsive Design

The interface adapts to different screen sizes:

- **Desktop (1024px+)**: Full dual-panel layout with sidebar
- **Tablet (768-1023px)**: Stacked layout with compact charts
- **Mobile (360-767px)**: Single column with collapsible sections

Key responsive features:
- Touch-friendly button sizes (minimum 44px)
- Readable text at all zoom levels
- Optimized input fields for mobile keyboards
- Gesture-friendly navigation

## 🧪 Testing

### Development Testing
```bash
npm run typecheck  # TypeScript validation
npm run lint       # ESLint code quality checks
```

### Testing Checklist
- [ ] Wallet connection/disconnection
- [ ] Network switching
- [ ] All trading operations (swap, mint, stake)
- [ ] Quote refreshing and staleness warnings
- [ ] URL state persistence
- [ ] Mobile responsiveness
- [ ] Keyboard navigation
- [ ] Error handling and user feedback

## 🚀 Deployment

### Build for Production
```bash
npm run build
npm run preview  # Test production build locally
```

### Deployment Targets
- **Vercel**: Recommended for automatic deployments
- **Netlify**: Alternative with form handling capabilities
- **Static Hosting**: Any CDN supporting SPA routing

### Environment Variables
- `VITE_DEFAULT_NETWORK`: Set to 'mainnet' for production
- `VITE_ANALYTICS_ID`: Optional analytics tracking ID

## 🔒 Security Considerations

### Smart Contract Integration
- All transactions are built locally and signed by user wallet
- No private keys or sensitive data stored in the application
- Read-only RPC calls for data fetching
- Transaction previews calculated off-chain for safety

### User Safety Features
- Slippage protection with user-configurable limits
- Price impact warnings for large trades
- Stale quote detection and refresh prompts
- Collateral ratio breach prevention
- Oracle staleness monitoring

## 🎯 Performance Optimization

### Loading Performance
- Code splitting with dynamic imports
- Lazy loading of chart components
- Optimized bundle size with tree shaking
- Image optimization and compression

### Runtime Performance
- React.memo for expensive components
- Debounced input handling
- Efficient re-render patterns
- Background data refreshing

### Caching Strategy
- 30s cache for real-time data (prices, quotes)
- 5min cache for protocol state
- 1h cache for static data (token metadata)

## 📋 Deployment Checklist

Before deploying to production:

- [ ] Update contract addresses for mainnet
- [ ] Enable mainnet as default network
- [ ] Configure analytics and monitoring
- [ ] Test all features on mainnet
- [ ] Run Lighthouse audit (target: >90 scores)
- [ ] Verify wallet compatibility
- [ ] Test responsive design on real devices
- [ ] Enable error reporting and logging

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests and lint checks
5. Submit a pull request

## 📄 License

This project is licensed under the MIT License.
