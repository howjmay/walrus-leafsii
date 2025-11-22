package markets

// Market describes an available trading or collateral pair.
type Market struct {
	ID                   string   `json:"id"`
	Label                string   `json:"label"`
	PairSymbol           string   `json:"pairSymbol"`
	StableSymbol         string   `json:"stableSymbol"`
	LeverageSymbol       string   `json:"leverageSymbol"`
	CollateralSymbol     string   `json:"collateralSymbol"`
	CollateralType       string   `json:"collateralType"`
	CollateralHighlights []string `json:"collateralHighlights"`
	Px                   int64    `json:"px"`
	CR                   string   `json:"cr"`
	TargetCR             string   `json:"targetCr"`
	Reserves             string   `json:"reserves"`
	SupplyStable         string   `json:"supplyStable"`
	SupplyLeverage       string   `json:"supplyLeverage"`
	Mode                 string   `json:"mode"`
	FeedURL              string   `json:"feedUrl,omitempty"`
	ProofCID             string   `json:"proofCid,omitempty"`
	SnapshotURL          string   `json:"snapshotUrl,omitempty"`
	ChainID              string   `json:"chainId,omitempty"`
	Asset                string   `json:"asset,omitempty"`
}
