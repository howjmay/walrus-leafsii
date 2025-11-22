package initializer

import (
	"context"
	"fmt"
	"strings"

	"github.com/fardream/go-bcs/bcs"
	"github.com/leafsii/leafsii-backend/internal/movebuild"
	"github.com/leafsii/leafsii-backend/internal/prices/binance"
	"github.com/pattonkan/sui-go/sui"
	"github.com/pattonkan/sui-go/sui/suiptb"
	"github.com/pattonkan/sui-go/suiclient"
	"github.com/pattonkan/sui-go/suisigner"
	"github.com/pattonkan/sui-go/utils"
)

type Result struct {
	LeafsiiPackageId *sui.PackageId
	ProtocolId       *sui.ObjectId
	PoolId           *sui.ObjectId
	AdminCapId       *sui.ObjectId
	FtokenPackageId  *sui.PackageId
	XtokenPackageId  *sui.PackageId
	Provider         *binance.Provider
}

func Initialize(ctx context.Context, client *suiclient.ClientImpl, signer *suisigner.Signer, corePath string, currentSuiPrice uint64, provider *binance.Provider) (Result, error) {
	var result Result
	result.Provider = provider

	_, leafsiiPackageId, err := client.BuildAndPublishContract(ctx, signer, corePath, 50*suiclient.DefaultGasBudget, &suiclient.SuiTransactionBlockResponseOptions{ShowEffects: true, ShowObjectChanges: true})
	if err != nil {
		return result, fmt.Errorf("failed to build and publish walrus-leafsii contract: %w", err)
	}
	result.LeafsiiPackageId = leafsiiPackageId

	protocolId, poolId, adminCapId, ftokenPackageId, xtokenPackageId, err := initProtocolAndPool(ctx, client, signer, leafsiiPackageId, currentSuiPrice)
	if err != nil {
		return result, fmt.Errorf("failed to initialize protocol and pool: %w", err)
	}

	result.ProtocolId = protocolId
	result.PoolId = poolId
	result.AdminCapId = adminCapId
	result.FtokenPackageId = ftokenPackageId
	result.XtokenPackageId = xtokenPackageId

	return result, nil
}

func initPool(
	ctx context.Context,
	client *suiclient.ClientImpl,
	signer *suisigner.Signer,
	packageId *sui.PackageId,
	ftokenPackageId *sui.PackageId,
	currentSuiPrice uint64,
) (*sui.ObjectId, *sui.ObjectId, error) {
	coinPage, err := client.GetCoins(ctx, &suiclient.GetCoinsRequest{Owner: signer.Address})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get coins: %w", err)
	}

	ptb := suiptb.NewTransactionDataTransactionBuilder()

	stabilityPoolAdminCapArg := ptb.Command(suiptb.Command{
		MoveCall: &suiptb.ProgrammableMoveCall{
			Package:  packageId,
			Module:   "stability_pool",
			Function: "create_stability_pool",
			TypeArguments: []sui.TypeTag{
				{Struct: &sui.StructTag{
					Address: ftokenPackageId,
					Module:  "ftoken",
					Name:    "FTOKEN",
				}},
			},
			Arguments: []suiptb.Argument{},
		}},
	)

	ptb.Command(suiptb.Command{TransferObjects: &suiptb.ProgrammableTransferObjects{
		Objects: []suiptb.Argument{stabilityPoolAdminCapArg},
		Address: ptb.MustPure(signer.Address),
	}})

	pt := ptb.Finish()

	tx := suiptb.NewTransactionData(
		signer.Address,
		pt,
		[]*sui.ObjectRef{coinPage.Data[1].Ref()},
		suiclient.DefaultGasBudget,
		suiclient.DefaultGasPrice,
	)

	txBytes, err := bcs.Marshal(tx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal transaction: %w", err)
	}

	txnResponse, err := client.SignAndExecuteTransaction(
		ctx,
		signer,
		txBytes,
		&suiclient.SuiTransactionBlockResponseOptions{
			ShowEffects:       true,
			ShowObjectChanges: true,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign and execute transaction: %w", err)
	}

	if !txnResponse.Effects.Data.IsSuccess() {
		return nil, nil, fmt.Errorf("transaction failed")
	}

	var poolId *sui.ObjectId
	var poolAdminCapId *sui.ObjectId
	for _, change := range txnResponse.ObjectChanges {
		fmt.Println("change.Data.String(): ", change.Data.String())
		if change.Data.Created != nil {
			resource, err := sui.NewResourceType(change.Data.Created.ObjectType)
			if err != nil {
				return nil, nil, fmt.Errorf("parse resource failed")
			}
			if resource.Contains(nil, "stability_pool", "StabilityPool") {
				poolId = &change.Data.Created.ObjectId
			}
			if resource.Contains(nil, "stability_pool", "StabilityPoolAdminCap") {
				poolAdminCapId = &change.Data.Created.ObjectId
			}
		}
	}
	if poolId == nil {
		return nil, nil, fmt.Errorf("pool ID not found in transaction response")
	}
	if poolAdminCapId == nil {
		return nil, nil, fmt.Errorf("pool admin cap ID not found in transaction response")
	}

	return poolId, poolAdminCapId, nil
}

func initProtocolAndPool(
	ctx context.Context,
	client *suiclient.ClientImpl,
	signer *suisigner.Signer,
	packageId *sui.PackageId,
	currentSuiPrice uint64,
) (*sui.ObjectId, *sui.ObjectId, *sui.ObjectId, *sui.PackageId, *sui.PackageId, error) {
	ftokenPackageId, ftokenTreasuryCap, err := buildDeployToken(ctx, client, signer, "ftoken")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to deploy ftoken: %w", err)
	}

	xtokenPackageId, xtokenTreasuryCap, err := buildDeployToken(ctx, client, signer, "xtoken")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to deploy xtoken: %w", err)
	}

	ftokenGetObjectRes, err := client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: ftokenTreasuryCap,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to get ftoken object: %w", err)
	}

	xtokenGetObjectRes, err := client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: xtokenTreasuryCap,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to get xtoken object: %w", err)
	}

	// TreasuryCaps are now owned objects
	ftokenTreasuryRef := ftokenGetObjectRes.Data.Ref()
	xtokenTreasuryRef := xtokenGetObjectRes.Data.Ref()

	poolId, poolAdminCapId, err := initPool(ctx, client, signer, packageId, ftokenPackageId, currentSuiPrice)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to init pool: %w", err)
	}

	poolGetObjectRes, err := client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: poolId,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to get pool object: %w", err)
	}
	poolRef := poolGetObjectRes.Data.RefSharedObject()

	poolAdminCapGetObjectRes, err := client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: poolAdminCapId,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to get poolAdminCap object: %w", err)
	}
	poolAdminCapRef := poolAdminCapGetObjectRes.Data.Ref()

	coinPage, err := client.GetCoins(ctx, &suiclient.GetCoinsRequest{Owner: signer.Address})
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to get coins: %w", err)
	}

	ptb := suiptb.NewTransactionDataTransactionBuilder()

	// Split SUI for reserve from a coin (not the gas coin)
	reserveAmount := uint64(1_000_000_000) // 1 SUI
	splitCoinArg := ptb.Command(suiptb.Command{
		SplitCoins: &suiptb.ProgrammableSplitCoins{
			Coin:    ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: coinPage.Data[0].Ref()}),
			Amounts: []suiptb.Argument{ptb.MustPure(reserveAmount)},
		},
	})

	ptb.Command(suiptb.Command{
		MoveCall: &suiptb.ProgrammableMoveCall{
			Package:  packageId,
			Module:   "leafsii",
			Function: "init_protocol",
			TypeArguments: []sui.TypeTag{
				{Struct: &sui.StructTag{
					Address: ftokenPackageId,
					Module:  "ftoken",
					Name:    "FTOKEN",
				}},
				{Struct: &sui.StructTag{
					Address: xtokenPackageId,
					Module:  "xtoken",
					Name:    "XTOKEN",
				}},
			},
			Arguments: []suiptb.Argument{
				ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: ftokenTreasuryRef}),
				ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: xtokenTreasuryRef}),
				ptb.MustPure(currentSuiPrice),
				splitCoinArg,
				ptb.MustObj(suiptb.ObjectArg{SharedObject: &suiptb.SharedObjectArg{
					Id:                   poolRef.ObjectId,
					InitialSharedVersion: poolRef.Version,
					Mutable:              true,
				}}),
				ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: poolAdminCapRef}),
				ptb.MustObj(suiptb.ObjectArg{SharedObject: &suiptb.SharedObjectArg{
					Id:                   sui.SuiObjectIdClock,
					InitialSharedVersion: sui.SuiClockObjectSharedVersion,
					Mutable:              false,
				}}),
			},
		}},
	)

	ptb.TransferArg(signer.Address, suiptb.Argument{NestedResult: &suiptb.NestedResult{Cmd: 1, Result: 0}})
	ptb.TransferArg(signer.Address, suiptb.Argument{NestedResult: &suiptb.NestedResult{Cmd: 1, Result: 1}})
	ptb.TransferArg(signer.Address, suiptb.Argument{NestedResult: &suiptb.NestedResult{Cmd: 1, Result: 2}})

	pt := ptb.Finish()

	tx := suiptb.NewTransactionData(
		signer.Address,
		pt,
		[]*sui.ObjectRef{coinPage.Data[1].Ref()},
		suiclient.DefaultGasBudget,
		suiclient.DefaultGasPrice,
	)

	txBytes, err := bcs.Marshal(tx)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to marshal transaction: %w", err)
	}

	txnResponse, err := client.SignAndExecuteTransaction(
		ctx,
		signer,
		txBytes,
		&suiclient.SuiTransactionBlockResponseOptions{
			ShowEffects:       true,
			ShowObjectChanges: true,
		},
	)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to sign and execute transaction: %w", err)
	}

	if !txnResponse.Effects.Data.IsSuccess() {
		return nil, nil, nil, nil, nil, fmt.Errorf("transaction failed")
	}

	var protocolId *sui.ObjectId
	var adminCapId *sui.ObjectId
	for _, change := range txnResponse.ObjectChanges {
		if change.Data.Created != nil {
			if strings.Contains(string(change.Data.Created.ObjectType), "leafsii::Protocol<") {
				protocolId = &change.Data.Created.ObjectId
			}
		}
		if change.Data.Created != nil {
			if strings.Contains(string(change.Data.Created.ObjectType), "leafsii::AdminCap") {
				adminCapId = &change.Data.Created.ObjectId
			}
		}
	}
	if protocolId == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("protocol ID not found in transaction response")
	}
	if adminCapId == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("admin ID not found in transaction response")
	}

	return protocolId, poolId, adminCapId, ftokenPackageId, xtokenPackageId, nil
}

func buildDeployToken(ctx context.Context, client *suiclient.ClientImpl, signer *suisigner.Signer, tokenName string) (*sui.PackageId, *sui.ObjectId, error) {
	contractPath := fmt.Sprintf("/webservice/backend/cmd/initializer/contract/%s/", tokenName)
	modules, err := movebuild.Build(ctx, utils.GetGitRoot()+contractPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build %s contract: %w", tokenName, err)
	}

	txnBytes, err := client.Publish(
		ctx,
		&suiclient.PublishRequest{
			Sender:          signer.Address,
			CompiledModules: modules.Modules,
			Dependencies:    modules.Dependencies,
			GasBudget:       sui.NewBigInt(10 * suiclient.DefaultGasBudget),
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to publish %s contract: %w", tokenName, err)
	}

	txnResponse, err := client.SignAndExecuteTransaction(
		ctx, signer, txnBytes.TxBytes, &suiclient.SuiTransactionBlockResponseOptions{
			ShowEffects:       true,
			ShowObjectChanges: true,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign and execute %s publish transaction: %w", tokenName, err)
	}

	if !txnResponse.Effects.Data.IsSuccess() {
		return nil, nil, fmt.Errorf("%s publish transaction failed", tokenName)
	}

	packageId, err := txnResponse.GetPublishedPackageId()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get published package ID for %s: %w", tokenName, err)
	}

	treasuryCap, _, err := txnResponse.GetCreatedObjectInfo("coin", "TreasuryCap")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get treasury cap for %s: %w", tokenName, err)
	}

	return packageId, treasuryCap, nil
}
