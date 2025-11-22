package onchain

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/fardream/go-bcs/bcs"
	"github.com/pattonkan/sui-go/sui"
	"github.com/pattonkan/sui-go/sui/suiptb"
	"github.com/pattonkan/sui-go/suiclient"
	"github.com/pattonkan/sui-go/suisigner"
	"github.com/pattonkan/sui-go/suisigner/suicrypto"
	"github.com/pattonkan/sui-go/utils/unit"
	"github.com/shopspring/decimal"
)

// TxBuildMode defines the transaction building mode
type TxBuildMode string

const (
	TxBuildModeExecution  TxBuildMode = "execution"
	TxBuildModeDevInspect TxBuildMode = "devinspect"
)

// MintTxRequest contains parameters for building mint transactions
type MintTxRequest struct {
	OutTokenType string
	Amount       decimal.Decimal
	UserAddress  *sui.Address
	Mode         TxBuildMode
}

// RedeemTxRequest contains parameters for building redeem transactions
type RedeemTxRequest struct {
	InTokenType string
	Amount      decimal.Decimal
	UserAddress *sui.Address
	Mode        TxBuildMode
}

type UpdateOracleTxRequest struct {
	SingerAddress *sui.Address
	NewPrice      uint64
	Mode          TxBuildMode
}

// TransactionBuilderInterface defines the interface for building transactions
type TransactionBuilderInterface interface {
	BuildMintTransaction(ctx context.Context, req MintTxRequest) (*UnsignedTransaction, error)
	BuildRedeemTransaction(ctx context.Context, req RedeemTxRequest) (*UnsignedTransaction, error)
	BuildUpdateOracleTransaction(ctx context.Context, req UpdateOracleTxRequest) (*UnsignedTransaction, error)
}

// TransactionSubmitterInterface defines the interface for submitting signed transactions
type TransactionSubmitterInterface interface {
	SubmitSignedTransaction(ctx context.Context, txBytes, signature string) (*TransactionResult, error)
}

type TransactionResult struct {
	TransactionDigest string
	Status            string
}

type TransactionBuilder struct {
	client          *suiclient.ClientImpl
	packageId       *sui.PackageId
	protocolId      *sui.ObjectId
	poolId          *sui.ObjectId
	adminCapId      *sui.ObjectId
	ftokenPackageId *sui.PackageId
	xtokenPackageId *sui.PackageId
	rpcURL          string
	network         string
}

func NewTransactionBuilder(
	rpcURL, network string,
	packageId *sui.PackageId,
	protocolId, poolId, adminCapId *sui.ObjectId,
	ftokenPackageId, xtokenPackageId *sui.PackageId,
) *TransactionBuilder {
	client := suiclient.NewClient(rpcURL)
	return &TransactionBuilder{
		client:          client,
		packageId:       packageId,
		protocolId:      protocolId,
		poolId:          poolId,
		adminCapId:      adminCapId,
		ftokenPackageId: ftokenPackageId,
		xtokenPackageId: xtokenPackageId,
		rpcURL:          rpcURL,
		network:         network,
	}
}

// NewTransactionBuilderWithClient creates a new TransactionBuilder with injectable client for testing
func NewTransactionBuilderWithClient(
	client *suiclient.ClientImpl,
	rpcURL, network string,
	packageId *sui.PackageId,
	protocolId, poolId *sui.ObjectId,
	ftokenPackageId, xtokenPackageId *sui.PackageId,
) *TransactionBuilder {
	return &TransactionBuilder{
		client:          client,
		packageId:       packageId,
		protocolId:      protocolId,
		poolId:          poolId,
		ftokenPackageId: ftokenPackageId,
		xtokenPackageId: xtokenPackageId,
		rpcURL:          rpcURL,
		network:         network,
	}
}

type UnsignedTransaction struct {
	TransactionBlockBytes []byte
	GasEstimate           uint64
	Metadata              map[string]string
}

func (tb *TransactionBuilder) BuildMintTransaction(ctx context.Context, req MintTxRequest) (*UnsignedTransaction, error) {
	protocolGetObject, err := tb.client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: tb.protocolId,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get protocol object: %w", err)
	}
	protocolRef := protocolGetObject.Data.RefSharedObject()

	poolGetObject, err := tb.client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: tb.poolId,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get pool object: %w", err)
	}
	poolRef := poolGetObject.Data.RefSharedObject()

	coinPages, err := tb.client.GetCoins(ctx, &suiclient.GetCoinsRequest{Owner: req.UserAddress})
	if err != nil {
		return nil, fmt.Errorf("failed to get coin object: %w", err)
	}
	coins := suiclient.Coins(coinPages.Data)

	// Convert amount to the appropriate unit (assuming 9 decimal places for Sui tokens)
	amountMist := req.Amount.Mul(decimal.New(1, unit.SuiDecimal)).BigInt().Uint64()

	ptb := suiptb.NewTransactionDataTransactionBuilder()

	if coins.TotalBalance().Uint64() < req.Amount.BigInt().Uint64() {
		return nil, fmt.Errorf("not enough balance")
	}

	var splitTargetCoinArg suiptb.Argument
	var mergeCoinsArgs []suiptb.Argument
	var bal uint64
	for i, coin := range coins {
		if bal > amountMist {
			break
		}
		bal += coin.Balance.Uint64()

		if i == 0 {
			splitTargetCoinArg = ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: coin.Ref()})
		} else {
			mergeCoinsArgs = append(mergeCoinsArgs, ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: coin.Ref()}))
		}
	}

	var splitCoinArg suiptb.Argument
	if len(mergeCoinsArgs) < 1 {
		splitCoinArg = ptb.Command(suiptb.Command{
			SplitCoins: &suiptb.ProgrammableSplitCoins{
				Coin:    splitTargetCoinArg,
				Amounts: []suiptb.Argument{ptb.MustPure(amountMist)},
			},
		})
	} else {
		ptb.Command(suiptb.Command{
			MergeCoins: &suiptb.ProgrammableMergeCoins{
				Destination: splitTargetCoinArg,
				Sources:     mergeCoinsArgs,
			},
		})
		splitCoinArg = ptb.Command(suiptb.Command{
			SplitCoins: &suiptb.ProgrammableSplitCoins{
				Coin:    splitTargetCoinArg,
				Amounts: []suiptb.Argument{ptb.MustPure(amountMist)},
			},
		})
	}

	var mintedArg suiptb.Argument
	switch req.OutTokenType {
	case "ftoken":
		mintedArg = ptb.Command(suiptb.Command{
			MoveCall: &suiptb.ProgrammableMoveCall{
				Package:  tb.packageId,
				Module:   "leafsii",
				Function: "mint_f",
				TypeArguments: []sui.TypeTag{
					{Struct: &sui.StructTag{
						Address: tb.ftokenPackageId,
						Module:  "ftoken",
						Name:    "FTOKEN",
					}},
					{Struct: &sui.StructTag{
						Address: tb.xtokenPackageId,
						Module:  "xtoken",
						Name:    "XTOKEN",
					}},
					{Struct: &sui.StructTag{
						Address: sui.MustObjectIdFromHex("0x2"),
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
					ptb.MustObj(suiptb.ObjectArg{SharedObject: &suiptb.SharedObjectArg{
						Id:                   poolRef.ObjectId,
						InitialSharedVersion: poolRef.Version,
						Mutable:              true,
					}}),
					splitCoinArg,
				},
			},
		})

	case "xtoken":
		// Build mint XToken transaction
		mintedArg = ptb.Command(suiptb.Command{
			MoveCall: &suiptb.ProgrammableMoveCall{
				Package:  tb.packageId,
				Module:   "leafsii",
				Function: "mint_x",
				TypeArguments: []sui.TypeTag{
					{Struct: &sui.StructTag{
						Address: tb.ftokenPackageId,
						Module:  "ftoken",
						Name:    "FTOKEN",
					}},
					{Struct: &sui.StructTag{
						Address: tb.xtokenPackageId,
						Module:  "xtoken",
						Name:    "XTOKEN",
					}},
					{Struct: &sui.StructTag{
						Address: sui.MustObjectIdFromHex("0x2"),
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
					ptb.MustObj(suiptb.ObjectArg{SharedObject: &suiptb.SharedObjectArg{
						Id:                   poolRef.ObjectId,
						InitialSharedVersion: poolRef.Version,
						Mutable:              true,
					}}),
					splitCoinArg,
				},
			},
		})

	default:
		return nil, fmt.Errorf("unsupported token type: %s", req.OutTokenType)
	}

	ptb.Command(suiptb.Command{
		TransferObjects: &suiptb.ProgrammableTransferObjects{
			Objects: []suiptb.Argument{mintedArg},
			Address: ptb.MustPure(req.UserAddress),
		},
	})

	pt := ptb.Finish()

	tx := suiptb.NewTransactionData(
		req.UserAddress,
		pt,
		[]*sui.ObjectRef{coins.CoinRefs()[len(coins)-1]},
		suiclient.DefaultGasBudget,
		suiclient.DefaultGasPrice,
	)

	var txBytes []byte
	if req.Mode == TxBuildModeDevInspect {
		txBytes, err = bcs.Marshal(tx.V1.Kind)
	} else {
		txBytes, err = bcs.Marshal(tx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transaction: %w", err)
	}

	return &UnsignedTransaction{
		TransactionBlockBytes: txBytes,
		GasEstimate:           suiclient.DefaultGasBudget,
		Metadata: map[string]string{
			"action":    "mint",
			"tokenType": req.OutTokenType,
			"amount":    req.Amount.String(),
			"network":   tb.network,
			"mode":      string(req.Mode),
		},
	}, nil
}

func (tb *TransactionBuilder) BuildRedeemTransaction(ctx context.Context, req RedeemTxRequest) (*UnsignedTransaction, error) {
	protocolGetObject, err := tb.client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: tb.protocolId,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get protocol object: %w", err)
	}
	protocolRef := protocolGetObject.Data.RefSharedObject()

	poolGetObject, err := tb.client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: tb.poolId,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get pool object: %w", err)
	}
	poolRef := poolGetObject.Data.RefSharedObject()

	coinType := ""
	switch req.InTokenType {
	case "ftoken":
		coinType = fmt.Sprintf("%s::ftoken::FTOKEN", tb.ftokenPackageId)
	case "xtoken":
		coinType = fmt.Sprintf("%s::xtoken::XTOKEN", tb.xtokenPackageId)
	default:
		return nil, fmt.Errorf("unsupported token type: %s", req.InTokenType)
	}
	coinPages, err := tb.client.GetCoins(ctx, &suiclient.GetCoinsRequest{Owner: req.UserAddress, CoinType: &coinType})
	if err != nil {
		return nil, fmt.Errorf("failed to get coin object: %w", err)
	}
	coins := suiclient.Coins(coinPages.Data)

	// Convert amount to the appropriate unit
	intTokenMetadata, err := tb.client.GetCoinMetadata(ctx, coinType)
	if err != nil {
		return nil, fmt.Errorf("failed to get input token coin_metadata: %w", err)
	}
	amountMist := req.Amount.Mul(decimal.New(1, int32(intTokenMetadata.Decimals))).BigInt().Uint64()

	ptb := suiptb.NewTransactionDataTransactionBuilder()

	if coins.TotalBalance().Uint64() < req.Amount.BigInt().Uint64() {
		return nil, fmt.Errorf("not enough balance")
	}

	var splitTargetInCoinArg suiptb.Argument
	var mergeInCoinsArgs []suiptb.Argument
	var bal uint64
	for i, coin := range coins {
		if bal > amountMist {
			break
		}
		bal += coin.Balance.Uint64()

		if i == 0 {
			splitTargetInCoinArg = ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: coin.Ref()})
		} else {
			mergeInCoinsArgs = append(mergeInCoinsArgs, ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: coin.Ref()}))
		}
	}

	var splitCoinArg suiptb.Argument
	if len(mergeInCoinsArgs) < 1 {
		splitCoinArg = ptb.Command(suiptb.Command{
			SplitCoins: &suiptb.ProgrammableSplitCoins{
				Coin:    splitTargetInCoinArg,
				Amounts: []suiptb.Argument{ptb.MustPure(amountMist)},
			},
		})
	} else {
		ptb.Command(suiptb.Command{
			MergeCoins: &suiptb.ProgrammableMergeCoins{
				Destination: splitTargetInCoinArg,
				Sources:     mergeInCoinsArgs,
			},
		})
		splitCoinArg = ptb.Command(suiptb.Command{
			SplitCoins: &suiptb.ProgrammableSplitCoins{
				Coin:    splitTargetInCoinArg,
				Amounts: []suiptb.Argument{ptb.MustPure(amountMist)},
			},
		})
	}

	var redeemedArg suiptb.Argument
	switch req.InTokenType {
	case "ftoken":
		redeemedArg = ptb.Command(suiptb.Command{
			MoveCall: &suiptb.ProgrammableMoveCall{
				Package:  tb.packageId,
				Module:   "leafsii",
				Function: "redeem_f",
				TypeArguments: []sui.TypeTag{
					{Struct: &sui.StructTag{
						Address: tb.ftokenPackageId,
						Module:  "ftoken",
						Name:    "FTOKEN",
					}},
					{Struct: &sui.StructTag{
						Address: tb.xtokenPackageId,
						Module:  "xtoken",
						Name:    "XTOKEN",
					}},
					{Struct: &sui.StructTag{
						Address: sui.MustObjectIdFromHex("0x2"),
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
					ptb.MustObj(suiptb.ObjectArg{SharedObject: &suiptb.SharedObjectArg{
						Id:                   poolRef.ObjectId,
						InitialSharedVersion: poolRef.Version,
						Mutable:              true,
					}}),
					splitCoinArg,
				},
			},
		})

	case "xtoken":
		redeemedArg = ptb.Command(suiptb.Command{
			MoveCall: &suiptb.ProgrammableMoveCall{
				Package:  tb.packageId,
				Module:   "leafsii",
				Function: "redeem_x",
				TypeArguments: []sui.TypeTag{
					{Struct: &sui.StructTag{
						Address: tb.ftokenPackageId,
						Module:  "ftoken",
						Name:    "FTOKEN",
					}},
					{Struct: &sui.StructTag{
						Address: tb.xtokenPackageId,
						Module:  "xtoken",
						Name:    "XTOKEN",
					}},
					{Struct: &sui.StructTag{
						Address: sui.MustObjectIdFromHex("0x2"),
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
					ptb.MustObj(suiptb.ObjectArg{SharedObject: &suiptb.SharedObjectArg{
						Id:                   poolRef.ObjectId,
						InitialSharedVersion: poolRef.Version,
						Mutable:              true,
					}}),
					splitCoinArg,
				},
			},
		})

	default:
		return nil, fmt.Errorf("unsupported token type: %s", req.InTokenType)
	}

	ptb.Command(suiptb.Command{
		TransferObjects: &suiptb.ProgrammableTransferObjects{
			Objects: []suiptb.Argument{redeemedArg},
			Address: ptb.MustPure(req.UserAddress),
		},
	})

	pt := ptb.Finish()

	gasGetCoins, err := tb.client.GetCoins(ctx, &suiclient.GetCoinsRequest{Owner: req.UserAddress})
	if err != nil {
		return nil, fmt.Errorf("failed to get gas coin: %w", err)
	}
	tx := suiptb.NewTransactionData(
		req.UserAddress,
		pt,
		[]*sui.ObjectRef{gasGetCoins.Data[0].Ref()},
		suiclient.DefaultGasBudget,
		suiclient.DefaultGasPrice,
	)

	var txBytes []byte
	if req.Mode == TxBuildModeDevInspect {
		txBytes, err = bcs.Marshal(tx.V1.Kind)
	} else {
		txBytes, err = bcs.Marshal(tx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transaction: %w", err)
	}

	return &UnsignedTransaction{
		TransactionBlockBytes: txBytes,
		GasEstimate:           suiclient.DefaultGasBudget,
		Metadata: map[string]string{
			"action":    "redeem",
			"tokenType": req.InTokenType,
			"amount":    req.Amount.String(),
			"network":   tb.network,
			"mode":      string(req.Mode),
		},
	}, nil
}

// BuildUpdateOracleTransaction builds an unsigned transaction for oracle updates
//
//	curl -X POST http://localhost:8080/v1/oracle/update/build \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "mode": "execution",
//	    "price": 4467890
//	  }'
func (tb *TransactionBuilder) BuildUpdateOracleTransaction(ctx context.Context, req UpdateOracleTxRequest) (*UnsignedTransaction, error) {
	protocolGetObject, err := tb.client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: tb.protocolId,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get protocol object: %w", err)
	}
	protocolRef := protocolGetObject.Data.RefSharedObject()

	signer := suisigner.NewSignerByIndex(suisigner.TEST_SEED, suicrypto.KeySchemeFlagDefault, 0)

	adminCapGetObjectRes, err := tb.client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: tb.adminCapId, // FIXME pass AdminCap
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get coin object: %w", err)
	}
	adminCapRef := adminCapGetObjectRes.Data.Ref()

	coinPages, err := tb.client.GetCoins(ctx, &suiclient.GetCoinsRequest{Owner: signer.Address})
	if err != nil {
		return nil, fmt.Errorf("failed to get coin object: %w", err)
	}
	coins := suiclient.Coins(coinPages.Data)

	ptb := suiptb.NewTransactionDataTransactionBuilder()

	clockArg := ptb.MustObj(suiptb.ObjectArg{SharedObject: &suiptb.SharedObjectArg{
		Id:                   sui.SuiObjectIdClock,
		InitialSharedVersion: sui.SuiClockObjectSharedVersion,
		Mutable:              false,
	}})
	oracleArg := ptb.Command(suiptb.Command{
		MoveCall: &suiptb.ProgrammableMoveCall{
			Package:  tb.packageId,
			Module:   "oracle",
			Function: "create_mock_oracle",
			TypeArguments: []sui.TypeTag{
				{Struct: &sui.StructTag{
					Address: sui.MustObjectIdFromHex("0x2"),
					Module:  "sui",
					Name:    "SUI",
				}},
			},
			Arguments: []suiptb.Argument{
				ptb.MustForceSeparatePure(req.NewPrice),
				clockArg,
			},
		},
	})
	ptb.Command(suiptb.Command{
		MoveCall: &suiptb.ProgrammableMoveCall{
			Package:  tb.packageId,
			Module:   "leafsii",
			Function: "update_from_oracle",
			TypeArguments: []sui.TypeTag{
				{Struct: &sui.StructTag{
					Address: tb.ftokenPackageId,
					Module:  "ftoken",
					Name:    "FTOKEN",
				}},
				{Struct: &sui.StructTag{
					Address: tb.xtokenPackageId,
					Module:  "xtoken",
					Name:    "XTOKEN",
				}},
				{Struct: &sui.StructTag{
					Address: sui.MustObjectIdFromHex("0x2"),
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
				oracleArg,
				clockArg,
				ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: adminCapRef}),
			},
		},
	})

	ptb.Command(suiptb.Command{
		TransferObjects: &suiptb.ProgrammableTransferObjects{
			Objects: []suiptb.Argument{oracleArg},
			Address: ptb.MustPure(signer.Address),
		},
	})

	pt := ptb.Finish()

	tx := suiptb.NewTransactionData(
		signer.Address,
		pt,
		[]*sui.ObjectRef{coins.CoinRefs()[len(coins)-1]},
		suiclient.DefaultGasBudget,
		suiclient.DefaultGasPrice,
	)

	var txBytes []byte
	if req.Mode == TxBuildModeDevInspect {
		txBytes, err = bcs.Marshal(tx.V1.Kind)
	} else {
		txBytes, err = bcs.Marshal(tx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transaction: %w", err)
	}

	res, err := tb.client.SignAndExecuteTransaction(ctx, signer, txBytes, &suiclient.SuiTransactionBlockResponseOptions{
		ShowInput:          true,
		ShowRawInput:       true,
		ShowEffects:        true,
		ShowEvents:         true,
		ShowObjectChanges:  true,
		ShowBalanceChanges: true,
		ShowRawEffects:     true,
	})
	if err != nil || !res.Effects.Data.IsSuccess() {
		return nil, fmt.Errorf("ExecuteTransactionBlock failed or not success: %w", err)
	}

	return &UnsignedTransaction{
		TransactionBlockBytes: txBytes,
		GasEstimate:           suiclient.DefaultGasBudget,
		Metadata: map[string]string{
			"action": "update_oracle",
			"mode":   string(req.Mode),
		},
	}, nil
}

// SubmitSignedTransaction submits a signed transaction to the Sui network
func (tb *TransactionBuilder) SubmitSignedTransaction(
	ctx context.Context,
	rawTxBytes, rawSignature string,
) (*TransactionResult, error) {
	txBytes, err := base64.StdEncoding.DecodeString(rawTxBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 encoded transaction bytes: %w", err)
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(rawSignature)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 encoded signature: %w", err)
	}

	sig := &suisigner.Signature{Ed25519SuiSignature: &suisigner.Ed25519SuiSignature{}}
	copy(sig.Ed25519SuiSignature.Signature[:], signatureBytes)
	response, err := tb.client.ExecuteTransactionBlock(ctx, &suiclient.ExecuteTransactionBlockRequest{
		TxDataBytes: txBytes,
		Signatures:  []*suisigner.Signature{sig},
		Options:     &suiclient.SuiTransactionBlockResponseOptions{ShowEffects: true},
		RequestType: suiclient.TxnRequestTypeWaitForLocalExecution,
	})
	if err != nil || !response.Effects.Data.IsSuccess() {
		return nil, fmt.Errorf("ExecuteTransactionBlock failed or not success: %w", err)
	}

	return &TransactionResult{
		TransactionDigest: "placeholder_digest_" + rawTxBytes[:8],
		Status:            "success",
	}, nil
}
