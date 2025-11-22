package markets

// Service exposes a small in-memory catalog of markets.
type Service struct {
	markets []Market
}

func NewService() *Service {
	return &Service{
		markets: []Market{
			{
				ID:                   "crosschain-eth",
				Label:                "Ethereum Cross-Chain Vault",
				PairSymbol:           "fETH/xETH",
				StableSymbol:         "fETH",
				LeverageSymbol:       "xETH",
				CollateralSymbol:     "ETH",
				CollateralType:       "crosschain",
				CollateralHighlights: []string{"Native ETH staked on Ethereum mainnet", "Verified via Walrus + zk light client proofs", "Self-custody withdrawals with signed vouchers", "Conservative 65% LTV, 6% liquidation penalty"},
				Px:                   2850000000,
				CR:                   "1.38",
				TargetCR:             "1.38",
				Reserves:             "8500000",
				SupplyStable:         "6159420.29",
				SupplyLeverage:       "2340579.71",
				Mode:                 "crosschain",
				FeedURL:              "https://walrus.xyz/api/feeds/eth-vault",
				ProofCID:             "bafyEthereumVaultProof",
				SnapshotURL:          "https://walrus.storage/eth/latest.json",
				ChainID:              "ethereum",
				Asset:                "ETH",
			},
		},
	}
}

func (s *Service) List() []Market {
	return s.markets
}

func (s *Service) Get(id string) (Market, bool) {
	for _, m := range s.markets {
		if m.ID == id {
			return m, true
		}
	}
	return Market{}, false
}
