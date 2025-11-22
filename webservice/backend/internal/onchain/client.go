package onchain

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/fardream/go-bcs/bcs"
	"github.com/leafsii/leafsii-backend/internal/prices/binance"
	"github.com/pattonkan/sui-go/sui"
	"github.com/pattonkan/sui-go/sui/suiptb"
	"github.com/pattonkan/sui-go/suiclient"
	"github.com/shopspring/decimal"
)

type ChainReader interface {
	ProtocolState(ctx context.Context) (*ProtocolState, error)
	SPIndex(ctx context.Context) (SPIndex, error)
	UserPositions(ctx context.Context, addr *sui.Address) (*UserPositions, error)
	EventsSince(ctx context.Context, fromCheckpoint uint64) ([]Event, uint64, error)
	PreviewMint(ctx context.Context, amountR decimal.Decimal) (PreviewMint, error)
	PreviewRedeemF(ctx context.Context, amountF decimal.Decimal) (PreviewRedeem, error)
	GetLatestCheckpoint(ctx context.Context) (uint64, error)
	GetOraclePrice(ctx context.Context, symbol string) (decimal.Decimal, time.Time, error)
	GetAllBalances(ctx context.Context, addr *sui.Address) (*Balances, error)
}

type Client struct {
	rpcURL           string
	client           *suiclient.ClientImpl
	wsURL            string
	objectsCore      string
	objectsSP        string
	network          string
	protocolId       *sui.ObjectId
	poolId           *sui.ObjectId
	ftokenPackageId  *sui.PackageId
	xtokenPackageId  *sui.PackageId
	leafsiiPackageId *sui.PackageId
	ftokenCoinType   sui.ObjectType
	xtokenCoinType   sui.ObjectType
	provider         *binance.Provider
}

type ClientOptions struct {
	ProtocolId       *sui.ObjectId
	PoolId           *sui.ObjectId
	FtokenPackageId  *sui.PackageId
	XtokenPackageId  *sui.PackageId
	LeafsiiPackageId *sui.PackageId
	Provider         *binance.Provider
}

func NewClient(rpcURL, wsURL, objectsCore, objectsSP, network string) *Client {
	return NewClientWithOptions(rpcURL, wsURL, objectsCore, objectsSP, network, ClientOptions{})
}

func NewClientWithOptions(rpcURL, wsURL, objectsCore, objectsSP, network string, opts ClientOptions) *Client {
	client := suiclient.NewClient(rpcURL)

	var ftokenCoinType, xtokenCoinType sui.ObjectType
	if opts.FtokenPackageId != nil {
		ftokenCoinType = fmt.Sprintf("%s::ftoken::FTOKEN", opts.FtokenPackageId.String())
	}
	if opts.XtokenPackageId != nil {
		xtokenCoinType = fmt.Sprintf("%s::xtoken::XTOKEN", opts.XtokenPackageId.String())
	}

	return &Client{
		rpcURL:           rpcURL,
		wsURL:            wsURL,
		client:           client,
		objectsCore:      objectsCore,
		objectsSP:        objectsSP,
		network:          network,
		protocolId:       opts.ProtocolId,
		poolId:           opts.PoolId,
		ftokenPackageId:  opts.FtokenPackageId,
		xtokenPackageId:  opts.XtokenPackageId,
		leafsiiPackageId: opts.LeafsiiPackageId,
		ftokenCoinType:   ftokenCoinType,
		xtokenCoinType:   xtokenCoinType,
		provider:         opts.Provider,
	}
}

// TODO: Implement with actual Sui SDK calls
func (c *Client) ProtocolState(ctx context.Context) (*ProtocolState, error) {
	protocolGetObject, err := c.client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: c.protocolId,
		Options: &suiclient.SuiObjectDataOptions{
			ShowContent: true,
			ShowBcs:     true,
			ShowOwner:   true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get protocol object: %w", err)
	}

	var moveProtocol MoveObjectProtocol
	_, err = bcs.Unmarshal(protocolGetObject.Data.Bcs.Data.MoveObject.BcsBytes, &moveProtocol)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to MoveObjectProtocol: %w", err)
	}

	ftokenSupply, err := c.getSupplyOnChain(ctx, "ftoken", protocolGetObject.Data.RefSharedObject())
	if err != nil {
		return nil, fmt.Errorf("failed to get ftoken supply: %w", err)
	}
	xtokenSupply, err := c.getSupplyOnChain(ctx, "xtoken", protocolGetObject.Data.RefSharedObject())
	if err != nil {
		return nil, fmt.Errorf("failed to get xtoken supply: %w", err)
	}

	// Placeholder implementation - would use Sui RPC to fetch protocol state
	reserveNetVal := float64(moveProtocol.ReserveTokenBalance.Value) * float64(moveProtocol.LastReservePrice)
	ftokenNetVal := float64(ftokenSupply) * float64(moveProtocol.Pf)

	return &ProtocolState{
		CR:           decimal.NewFromFloat(reserveNetVal / ftokenNetVal),
		ReservesR:    decimal.NewFromBigInt(new(big.Int).SetUint64(moveProtocol.ReserveTokenBalance.Value), 0),
		SupplyF:      decimal.NewFromBigInt(new(big.Int).SetUint64(ftokenSupply), 0),
		SupplyX:      decimal.NewFromBigInt(new(big.Int).SetUint64(xtokenSupply), 0),
		Pf:           moveProtocol.Pf,
		Px:           moveProtocol.Px,
		P:            moveProtocol.LastReservePrice,
		Mode:         "normal",
		OracleAgeSec: 30,
		AsOf:         time.Now(),
	}, nil
}

func (c *Client) getSupplyOnChain(ctx context.Context, tokenName string, protocolRef *sui.ObjectRef) (uint64, error) {
	var funcName string
	if strings.ToLower(tokenName) == "ftoken" {
		funcName = "get_total_stable_supply"
	} else if strings.ToLower(tokenName) == "xtoken" {
		funcName = "get_total_leverage_supply"
	}
	ptb := suiptb.NewTransactionDataTransactionBuilder()

	ptb.Command(suiptb.Command{
		MoveCall: &suiptb.ProgrammableMoveCall{
			Package:  c.LeafsiiPackageId(),
			Module:   "leafsii",
			Function: funcName,
			TypeArguments: []sui.TypeTag{
				{Struct: &sui.StructTag{
					Address: c.ftokenPackageId,
					Module:  "ftoken",
					Name:    "FTOKEN",
				}},
				{Struct: &sui.StructTag{
					Address: c.xtokenPackageId,
					Module:  "xtoken",
					Name:    "XTOKEN",
				}},
				{Struct: &sui.StructTag{
					Address: sui.MustAddressFromHex("0x2"),
					Module:  "sui",
					Name:    "SUI",
				}},
			},
			Arguments: []suiptb.Argument{
				ptb.MustObj(suiptb.ObjectArg{SharedObject: &suiptb.SharedObjectArg{
					Id:                   protocolRef.ObjectId,
					InitialSharedVersion: protocolRef.Version,
					Mutable:              true,
				}}),
			},
		}},
	)

	pt := ptb.Finish()

	tx := suiptb.NewTransactionData(
		sui.MustAddressFromHex("0x0"),
		pt,
		[]*sui.ObjectRef{},
		suiclient.DefaultGasBudget,
		suiclient.DefaultGasPrice,
	)
	txBytes, err := bcs.Marshal(tx.V1.Kind)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal transaction: %w", err)
	}
	res, err := c.client.DevInspectTransactionBlock(ctx, &suiclient.DevInspectTransactionBlockRequest{
		SenderAddress: sui.MustAddressFromHex("0x0"),
		TxKindBytes:   txBytes,
	})
	if err != nil || res.Error != "" {
		return 0, fmt.Errorf("failed to run DevInspectTransactionBlock, response.Error: %s: %w", res.Error, err)
	}

	var balance uint64
	_, err = bcs.Unmarshal(res.Results[0].ReturnValues[0].Data, &balance)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal balance from response: %w", err)
	}

	return balance, nil
}

func (c *Client) SPIndex(ctx context.Context) (SPIndex, error) {
	// TODO: Implement with actual Sui SDK calls
	return SPIndex{
		IndexValue:    decimal.NewFromFloat(1.05),
		TVLF:          decimal.NewFromInt(500000),
		TotalRewardsR: decimal.NewFromInt(25000),
		AsOf:          time.Now(),
	}, nil
}

func (c *Client) UserPositions(ctx context.Context, addr *sui.Address) (*UserPositions, error) {
	// TODO: Implement with actual Sui SDK calls to fetch user's balances and SP position
	ret := &UserPositions{
		Address:     addr,
		BalanceF:    decimal.NewFromInt(1000),
		BalanceX:    decimal.NewFromInt(0),
		BalanceR:    decimal.NewFromInt(500),
		StakeF:      decimal.NewFromInt(200),
		IndexAtJoin: decimal.NewFromFloat(1.02),
		ClaimableR:  decimal.NewFromInt(10),
		UpdatedAt:   time.Now(),
	}
	balances, err := c.client.GetAllBalances(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to get all balance: %w", err)
	}
	for _, bal := range balances {
		if strings.Contains(bal.CoinType, "ftoken::FTOKEN") {
			ret.BalanceF = decimal.NewFromBigInt(bal.TotalBalance.Int, 0)
		}
		if strings.Contains(bal.CoinType, "xtoken::XTOKEN") {
			ret.BalanceX = decimal.NewFromBigInt(bal.TotalBalance.Int, 0)
		}
		if strings.Contains(bal.CoinType, "sui::SUI") {
			ret.BalanceR = decimal.NewFromBigInt(bal.TotalBalance.Int, 0)
		}
	}
	return ret, nil
}

func (c *Client) EventsSince(ctx context.Context, fromCheckpoint uint64) ([]Event, uint64, error) {
	// TODO: Implement with actual Sui event queries
	return []Event{}, fromCheckpoint, nil
}

func (c *Client) PreviewMint(ctx context.Context, amountR decimal.Decimal) (PreviewMint, error) {
	// TODO: Either call on-chain view function or compute using calc functions
	fee := amountR.Mul(decimal.NewFromFloat(0.003)) // 0.3% fee example
	fOut := amountR.Sub(fee)

	return PreviewMint{
		FOut:   fOut,
		Fee:    fee,
		PostCR: decimal.NewFromFloat(1.48), // Computed based on new state
	}, nil
}

func (c *Client) PreviewRedeemF(ctx context.Context, amountF decimal.Decimal) (PreviewRedeem, error) {
	// TODO: Either call on-chain view function or compute using calc functions
	fee := amountF.Mul(decimal.NewFromFloat(0.005)) // 0.5% fee example
	rOut := amountF.Sub(fee)

	return PreviewRedeem{
		ROut:   rOut,
		Fee:    fee,
		PostCR: decimal.NewFromFloat(1.52), // Computed based on new state
	}, nil
}

func (c *Client) GetLatestCheckpoint(ctx context.Context) (uint64, error) {
	// TODO: Implement with actual Sui RPC call to get latest checkpoint
	return 12345, nil
}

func (c *Client) GetOraclePrice(ctx context.Context, symbol string) (decimal.Decimal, time.Time, error) {
	if c.provider == nil {
		return decimal.Zero, time.Time{}, fmt.Errorf("provider not configured")
	}

	switch symbol {
	case "FTOKEN":
		return decimal.NewFromFloat(1 * binance.BinanceScale), time.Now().Add(-30 * time.Second), nil
	case "SUIUSDT":
		price, err := c.provider.GetLatestPrice(ctx, symbol)
		if err != nil {
			return decimal.Zero, time.Time{}, fmt.Errorf("get latest price: %w", err)
		}
		return price, time.Now().UTC(), nil
	case "RTOKEN":
		// RTOKEN represents the underlying SUI token, so get SUIUSDT price
		price, err := c.provider.GetLatestPrice(ctx, "SUIUSDT")
		if err != nil {
			return decimal.Zero, time.Time{}, fmt.Errorf("get latest price: %w", err)
		}
		return price, time.Now().UTC(), nil
	default:
		return decimal.Zero, time.Time{}, fmt.Errorf("unknown symbol: %s", symbol)
	}
}

const SuiDecimal = 9

func (c *Client) GetAllBalances(ctx context.Context, addr *sui.Address) (*Balances, error) {
	var ret Balances
	balances, err := c.client.GetAllBalances(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to get all balance: %w", err)
	}
	for _, bal := range balances {
		if strings.Contains(bal.CoinType, "ftoken::FTOKEN") {
			ret.F = decimal.NewFromBigInt(bal.TotalBalance.Int, 0)
		}
		if strings.Contains(bal.CoinType, "xtoken::XTOKEN") {
			ret.X = decimal.NewFromBigInt(bal.TotalBalance.Int, 0)
		}
		if strings.Contains(bal.CoinType, "sui::SUI") {
			ret.R = decimal.NewFromBigInt(bal.TotalBalance.Int, 0)
		}
	}

	return &ret, nil
}

// Getter methods for the new fields
func (c *Client) ProtocolId() *sui.ObjectId {
	return c.protocolId
}

func (c *Client) PoolId() *sui.ObjectId {
	return c.poolId
}

func (c *Client) FtokenPackageId() *sui.PackageId {
	return c.ftokenPackageId
}

func (c *Client) XtokenPackageId() *sui.PackageId {
	return c.xtokenPackageId
}

func (c *Client) LeafsiiPackageId() *sui.PackageId {
	return c.leafsiiPackageId
}

// WebSocket subscription methods
func (c *Client) SubscribeToEvents(ctx context.Context, eventTypes []string, callback func(Event)) error {
	// TODO: Implement WebSocket subscription to Sui events
	// This would subscribe to Mint, Redeem, Stake, Unstake, Claim, Rebalance events
	// and call the callback function for each event received
	return fmt.Errorf("WebSocket subscription not implemented yet")
}
