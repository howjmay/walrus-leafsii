//go:build e2e

package onchain

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/leafsii/leafsii-backend/internal/initializer"
	"github.com/leafsii/leafsii-backend/internal/prices/binance"
	"github.com/pattonkan/sui-go/sui"
	"github.com/pattonkan/sui-go/suiclient"
	"github.com/pattonkan/sui-go/suiclient/conn"
	"github.com/pattonkan/sui-go/suisigner"
	"github.com/pattonkan/sui-go/suisigner/suicrypto"
	"github.com/pattonkan/sui-go/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	localnetClientTestTimeout = 30 * time.Second
	localnetClientRPCURL      = "http://localhost:9000"
	localnetClientWSURL       = "ws://localhost:9000"
	localnetRecentTimeWindow  = 2 * time.Minute
)

var (
	localnetClient   *Client
	localnetTestAddr *sui.Address
)

// TestClientProtocolState_Localnet tests ProtocolState method
func TestClientProtocolState_Localnet(t *testing.T) {
	client, _ := setupLocalnetClientTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), localnetClientTestTimeout)
	defer cancel()

	state, err := client.ProtocolState(ctx)
	require.NoError(t, err, "ProtocolState should not error")
	require.NotNil(t, state, "ProtocolState should return non-nil result")

	fmt.Println("state: ", state.String())

	// Assert expected hardcoded values
	assert.True(t, state.CR.Equal(decimal.NewFromFloat(2)),
		"CR should  equal 2, got %s", state.CR.String())
	assert.True(t, state.ReservesR.GreaterThan(decimal.Zero),
		"ReservesR should be positive, got %s", state.ReservesR.String())
	assert.Equal(t, "normal", state.Mode, "Mode should be 'normal'")
	assert.GreaterOrEqual(t, state.OracleAgeSec, int64(0),
		"OracleAgeSec should be >= 0")
	requireLocalnetRecent(t, state.AsOf)
}

// TestClientSPIndex_Localnet tests SPIndex method
func TestClientSPIndex_Localnet(t *testing.T) {
	client, _ := setupLocalnetClientTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), localnetClientTestTimeout)
	defer cancel()

	index, err := client.SPIndex(ctx)
	require.NoError(t, err, "SPIndex should not error")

	// Assert expected hardcoded values
	assert.True(t, index.IndexValue.Equal(decimal.NewFromFloat(1.05)),
		"IndexValue should equal 1.05, got %s", index.IndexValue.String())
	assert.True(t, index.TVLF.Equal(decimal.NewFromInt(500000)),
		"TVLF should equal 500000, got %s", index.TVLF.String())
	assert.True(t, index.TotalRewardsR.Equal(decimal.NewFromInt(25000)),
		"TotalRewardsR should equal 25000, got %s", index.TotalRewardsR.String())
	requireLocalnetRecent(t, index.AsOf)
}

// TestClientUserPositions_Localnet tests UserPositions method
func TestClientUserPositions_Localnet(t *testing.T) {
	client, testAddr := setupLocalnetClientTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), localnetClientTestTimeout)
	defer cancel()

	positions, err := client.UserPositions(ctx, testAddr)
	require.NoError(t, err, "UserPositions should not error")
	require.NotNil(t, positions, "UserPositions should return non-nil result")

	assert.Equal(t, testAddr, positions.Address, "Address should match input")
	// Only assert BalanceR > 0 as the test address is funded with SUI
	assert.True(t, positions.BalanceR.GreaterThan(decimal.Zero),
		"BalanceR should be positive (funded address), got %s", positions.BalanceR.String())
	// F and X tokens might not be minted to test address, so don't assert positive amounts
	requireLocalnetRecent(t, positions.UpdatedAt)
}

// TestClientEventsSince_Localnet tests EventsSince method
func TestClientEventsSince_Localnet(t *testing.T) {
	client, _ := setupLocalnetClientTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), localnetClientTestTimeout)
	defer cancel()

	// Test with checkpoint 0
	events, checkpoint, err := client.EventsSince(ctx, 0)
	require.NoError(t, err, "EventsSince should not error")
	assert.Empty(t, events, "Events should be empty slice (per current impl)")
	assert.Equal(t, uint64(0), checkpoint, "Checkpoint should be same as input")

	// Test with latest checkpoint
	latest, err := client.GetLatestCheckpoint(ctx)
	require.NoError(t, err)

	events, returnedCheckpoint, err := client.EventsSince(ctx, latest)
	require.NoError(t, err, "EventsSince should not error")
	assert.Empty(t, events, "Events should be empty slice (per current impl)")
	assert.Equal(t, latest, returnedCheckpoint, "Checkpoint should be same as input")
}

// TestClientPreviewMint_Localnet tests PreviewMint method
func TestClientPreviewMint_Localnet(t *testing.T) {
	client, _ := setupLocalnetClientTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), localnetClientTestTimeout)
	defer cancel()

	amountR := decimal.NewFromFloat(100.0)
	preview, err := client.PreviewMint(ctx, amountR)
	require.NoError(t, err, "PreviewMint should not error")

	// Assert expected calculated values based on current implementation
	expectedFee := decimal.NewFromFloat(0.3)   // 0.3% of 100.0
	expectedFOut := decimal.NewFromFloat(99.7) // 100.0 - 0.3
	expectedPostCR := decimal.NewFromFloat(1.48)

	assert.True(t, preview.Fee.Equal(expectedFee),
		"Fee should equal 0.3, got %s", preview.Fee.String())
	assert.True(t, preview.FOut.Equal(expectedFOut),
		"FOut should equal 99.7, got %s", preview.FOut.String())
	assert.True(t, preview.PostCR.Equal(expectedPostCR),
		"PostCR should equal 1.48, got %s", preview.PostCR.String())
}

// TestClientPreviewRedeemF_Localnet tests PreviewRedeemF method
func TestClientPreviewRedeemF_Localnet(t *testing.T) {
	client, _ := setupLocalnetClientTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), localnetClientTestTimeout)
	defer cancel()

	amountF := decimal.NewFromFloat(200.0)
	preview, err := client.PreviewRedeemF(ctx, amountF)
	require.NoError(t, err, "PreviewRedeemF should not error")

	// Assert expected calculated values based on current implementation
	expectedFee := decimal.NewFromFloat(1.0)    // 0.5% of 200.0
	expectedROut := decimal.NewFromFloat(199.0) // 200.0 - 1.0
	expectedPostCR := decimal.NewFromFloat(1.52)

	assert.True(t, preview.Fee.Equal(expectedFee),
		"Fee should equal 1.0, got %s", preview.Fee.String())
	assert.True(t, preview.ROut.Equal(expectedROut),
		"ROut should equal 199.0, got %s", preview.ROut.String())
	assert.True(t, preview.PostCR.Equal(expectedPostCR),
		"PostCR should equal 1.52, got %s", preview.PostCR.String())
}

// TestClientGetLatestCheckpoint_Localnet tests GetLatestCheckpoint method
func TestClientGetLatestCheckpoint_Localnet(t *testing.T) {
	client, _ := setupLocalnetClientTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), localnetClientTestTimeout)
	defer cancel()

	checkpoint, err := client.GetLatestCheckpoint(ctx)
	require.NoError(t, err, "GetLatestCheckpoint should not error")

	// Assert hardcoded return value per current implementation
	assert.Equal(t, uint64(12345), checkpoint,
		"GetLatestCheckpoint should return 12345 (per current impl)")
}

// TestClientGetOraclePrice_Localnet tests GetOraclePrice method
func TestClientGetOraclePrice_Localnet(t *testing.T) {
	client, _ := setupLocalnetClientTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), localnetClientTestTimeout)
	defer cancel()

	// Test with provider configured - should delegate to provider
	if client.provider != nil {
		// Test with a valid symbol like SUIUSDT
		price, timestamp, err := client.GetOraclePrice(ctx, "SUIUSDT")
		require.NoError(t, err, "GetOraclePrice with provider should not error")
		assert.True(t, price.GreaterThan(decimal.Zero),
			"Price should be positive, got %s", price.String())
		requireLocalnetRecent(t, timestamp)
	} else {
		// Test without provider - should return error
		price, timestamp, err := client.GetOraclePrice(ctx, "SUIUSDT")
		require.Error(t, err, "GetOraclePrice without provider should error")
		assert.Contains(t, err.Error(), "provider not configured")
		assert.True(t, price.Equal(decimal.Zero), "Price should be zero when provider not configured")
		assert.True(t, timestamp.IsZero(), "Timestamp should be zero when provider not configured")
	}
}

// TestClientGetAllBalances_Localnet tests GetAllBalances method
func TestClientGetAllBalances_Localnet(t *testing.T) {
	client, testAddr := setupLocalnetClientTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), localnetClientTestTimeout)
	defer cancel()

	balances, err := client.GetAllBalances(ctx, testAddr)
	require.NoError(t, err, "GetAllBalances should not error")
	require.NotNil(t, balances, "GetAllBalances should return non-nil result")

	// Assert R (SUI) balance > 0 since test address is funded
	assert.True(t, balances.R.GreaterThan(decimal.Zero),
		"R balance should be positive (funded address), got %s", balances.R.String())

	// F and X balances may be zero if tokens weren't minted to test address
	// So don't assert positive amounts for them
	assert.True(t, balances.F.GreaterThanOrEqual(decimal.Zero),
		"F balance should be >= 0, got %s", balances.F.String())
	assert.True(t, balances.X.GreaterThanOrEqual(decimal.Zero),
		"X balance should be >= 0, got %s", balances.X.String())
}

// Note: Using existing TestMain from transaction_builder_test.go
// This file provides client-specific localnet integration tests

// setupLocalnetClientTests initializes the localnet environment for client tests
func setupLocalnetClientTests() error {
	// Assume localnet is already running at http://localhost:9000
	// Create client and signer for initializer
	client, signer := suiclient.NewClient(conn.LocalnetEndpointUrl).WithSignerAndFund(suisigner.TEST_SEED, suicrypto.KeySchemeFlagDefault, 0)

	// Initialize contracts using the new initializer package
	corePath := utils.GetGitRoot() + "/walrus-leafsii/"
	currentSuiPrice := uint64(2 * binance.BinanceScale) // Use $1.00 as default for tests

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create binance provider for tests
	logger := zap.NewNop().Sugar()
	provider := binance.NewProvider(logger)

	result, err := initializer.Initialize(ctx, client, signer, corePath, currentSuiPrice, provider)
	if err != nil {
		return fmt.Errorf("failed to initialize protocol: %w", err)
	}

	// Create funded signer for testing
	testMnemonic := "arena garbage light lizard champion weasel produce analyst broken pitch shine gas"
	testSigner, err := suisigner.NewSignerWithMnemonic(testMnemonic, suicrypto.KeySchemeFlagEd25519)
	if err != nil {
		return err
	}
	localnetTestAddr = testSigner.Address

	// Fund the test address
	if err := suiclient.RequestFundFromFaucet(localnetTestAddr, conn.LocalnetFaucetUrl); err != nil {
		return err
	}

	// Create onchain client with IDs from initializer result
	opts := ClientOptions{
		ProtocolId:       result.ProtocolId,
		PoolId:           result.PoolId,
		FtokenPackageId:  result.FtokenPackageId,
		XtokenPackageId:  result.XtokenPackageId,
		LeafsiiPackageId: result.LeafsiiPackageId,
		Provider:         result.Provider,
	}

	localnetClient = NewClientWithOptions(
		localnetClientRPCURL,
		localnetClientWSURL,
		"dummy-core", // not used by current code paths
		"dummy-sp",   // not used by current code paths
		"localnet",
		opts,
	)

	return nil
}

// requireLocalnetRecent asserts that the given time is within the recent window
func requireLocalnetRecent(t *testing.T, timestamp time.Time) {
	t.Helper()
	now := time.Now()
	require.WithinDuration(t, now, timestamp, localnetRecentTimeWindow,
		"timestamp should be recent (within %v)", localnetRecentTimeWindow)
}

// setupLocalnetClientTest sets up a client test - skips if env var not set
func setupLocalnetClientTest(t *testing.T) (*Client, *sui.Address) {
	t.Helper()

	// Skip unless RUN_LOCALNET_TESTS=1
	// if os.Getenv("RUN_LOCALNET_TESTS") != "1" {
	// 	t.Skip("localnet tests disabled (set RUN_LOCALNET_TESTS=1 to enable)")
	// }

	// Use the localnet client set by TestMainLocalnetClient if available
	if localnetClient != nil {
		return localnetClient, localnetTestAddr
	}

	// If TestMainLocalnetClient wasn't run, set up manually
	err := setupLocalnetClientTests()
	require.NoError(t, err, "Failed to setup localnet client tests")

	return localnetClient, localnetTestAddr
}
