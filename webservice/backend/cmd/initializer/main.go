package main

import (
	"context"
	"fmt"
	"time"

	"github.com/leafsii/leafsii-backend/cmd/initializer/pkg"
	"github.com/leafsii/leafsii-backend/internal/initializer"
	"github.com/leafsii/leafsii-backend/internal/prices/binance"
	"github.com/pattonkan/sui-go/sui"
	"github.com/pattonkan/sui-go/suiclient"
	"github.com/pattonkan/sui-go/suiclient/conn"
	"github.com/pattonkan/sui-go/suisigner"
	"github.com/pattonkan/sui-go/suisigner/suicrypto"
	"github.com/pattonkan/sui-go/utils"
	"go.uber.org/zap"
)

const (
	initConfigPath = "/webservice/backend/cmd/initializer/init.json"
)

func main() {
	initConfig, err := pkg.ReadConfig(utils.GetGitRoot() + initConfigPath)
	if err != nil {
		panic(err)
	}

	suiClient, signer := suiclient.NewClient(conn.LocalnetEndpointUrl).WithSignerAndFund(suisigner.TEST_SEED, suicrypto.KeySchemeFlagDefault, 0)
	fmt.Println("signer: ", signer.Address)
	time.Sleep(100 * time.Millisecond)

	err = suiclient.RequestFundFromFaucet(initConfig.BrowserWalletAddr, conn.LocalnetFaucetUrl)
	if err != nil {
		panic(err)
	}

	// Resolve git root and set corePath
	corePath := utils.GetGitRoot() + "/walrus-leafsii/"

	// Fetch live SUI price with timeout and fallback
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var currentSuiPrice uint64
	logger := zap.NewNop().Sugar()
	provider := binance.NewProvider(logger)
	priceDecimal, err := provider.GetLatestPrice(ctx, "SUIUSDT")
	if err != nil {
		fmt.Printf("Warning: Failed to fetch live SUI price, using fallback $1.00: %v\n", err)
		currentSuiPrice = binance.BinanceScale // scale stands for 1 which is $1.00 too
	} else {
		currentSuiPrice = priceDecimal.BigInt().Uint64()
		fmt.Printf("Using live SUI price: $%.6f (scaled: %d)\n", float64(currentSuiPrice)/float64(binance.BinanceScale), currentSuiPrice)
	}

	// Call initializer.Initialize to get typed IDs
	result, err := initializer.Initialize(context.Background(), suiClient, signer, corePath, currentSuiPrice, provider)
	if err != nil {
		panic(err)
	}

	fmt.Println("leafsiiPackageId: ", result.LeafsiiPackageId)
	fmt.Println("protocolId: ", result.ProtocolId)
	fmt.Println("poolId: ", result.PoolId)
	fmt.Println("adminCapId: ", result.AdminCapId)
	fmt.Println("ftokenPackageId: ", result.FtokenPackageId)
	fmt.Println("xtokenPackageId: ", result.XtokenPackageId)

	// Convert returned values into a struct matching pkg.InitConfig
	initConfig.ProtocolId = (*sui.Address)(result.ProtocolId)
	initConfig.PoolId = (*sui.Address)(result.PoolId)
	initConfig.AdminCapId = result.AdminCapId
	initConfig.FtokenPackageId = (*sui.Address)(result.FtokenPackageId)
	initConfig.XtokenPackageId = (*sui.Address)(result.XtokenPackageId)
	initConfig.LeafsiiPackageId = result.LeafsiiPackageId

	// Marshal to JSON and write to init.json
	err = pkg.WriteConfig(utils.GetGitRoot()+initConfigPath, initConfig)
	if err != nil {
		fmt.Printf("Error writing config: %v\n", err)
		panic(err)
	}
	fmt.Printf("Configuration written to %s\n", utils.GetGitRoot()+initConfigPath)
}
