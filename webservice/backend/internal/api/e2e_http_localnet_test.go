//go:build e2e

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/leafsii/leafsii-backend/internal/initializer"
	"github.com/leafsii/leafsii-backend/internal/onchain"
	"github.com/leafsii/leafsii-backend/internal/prices/binance"
	"github.com/pattonkan/sui-go/suiclient"
	"github.com/pattonkan/sui-go/suiclient/conn"
	"github.com/pattonkan/sui-go/suisigner"
	"github.com/pattonkan/sui-go/suisigner/suicrypto"
	"github.com/pattonkan/sui-go/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	e2eTestTimeout          = 5 * time.Minute
	e2eLocalnetRPCURL       = "http://localhost:9000"
	e2eLocalnetReadyTimeout = 2 * time.Minute
)

var (
	e2eSuiProcess *exec.Cmd
	e2eInitResult *initializer.Result
)

// TestE2EHttpLocalnet is a comprehensive E2E test that uses real localnet
func TestE2EHttpLocalnet(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Skip if sui binary is not available
	if _, err := exec.LookPath("sui"); err != nil {
		t.Skip("sui binary not available, skipping E2E integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), e2eTestTimeout)
	defer cancel()

	// Setup localnet and initialize contracts
	if err := e2eSetupLocalnetAndInitialize(ctx, t); err != nil {
		t.Fatalf("E2E setup failed: %v", err)
	}
	defer e2eCleanup()

	// Use initialized values from e2eInitResult
	require.NotNil(t, e2eInitResult, "Initialization result should be available")

	// Create funded signer
	client, signer := e2eNewFundedSignerAndClient(t)

	// Build transaction builder with real IDs from initializer result
	txBuilder := onchain.NewTransactionBuilder(
		e2eLocalnetRPCURL,
		"localnet",
		e2eInitResult.LeafsiiPackageId,
		e2eInitResult.ProtocolId,
		e2eInitResult.PoolId,
		e2eInitResult.AdminCapId,
		e2eInitResult.FtokenPackageId,
		e2eInitResult.XtokenPackageId,
	)

	// Setup HTTP server with real handlers and routes
	httpServer := e2eSetupHTTPServer(t, txBuilder)
	defer httpServer.Close()

	// Test complete build→sign→submit flow
	t.Run("E2E mint ftoken flow", func(t *testing.T) {
		e2eTestMintFTokenFlow(t, httpServer, client, signer)
		time.Sleep(10 * time.Second)
		e2eTestMintFTokenFlow(t, httpServer, client, signer)
	})
}

// e2eSetupLocalnetAndInitialize starts Sui localnet and runs initializer
func e2eSetupLocalnetAndInitialize(ctx context.Context, t *testing.T) error {
	t.Helper()

	// Check if localnet is already running
	// if err := e2eCheckLocalnetReady(ctx); err != nil {
	// 	t.Logf("Starting Sui localnet...")

	// 	// Start localnet
	// 	e2eSuiProcess = exec.CommandContext(ctx, "sui", "start", "--force-regenesis", "--with-faucet")
	// 	if err := e2eSuiProcess.Start(); err != nil {
	// 		return fmt.Errorf("failed to start sui localnet: %w", err)
	// 	}

	// 	// Wait for localnet to be ready with generous timeout
	// 	if err := e2eWaitForLocalnetReady(ctx); err != nil {
	// 		return fmt.Errorf("localnet did not become ready: %w", err)
	// 	}
	// }

	// Run initializer to deploy contracts and write init.json
	t.Logf("Running initializer...")
	client, signer := suiclient.NewClient(conn.LocalnetEndpointUrl).WithSignerAndFund(suisigner.TEST_SEED, suicrypto.KeySchemeFlagDefault, 0)
	corePath := utils.GetGitRoot() + "/walrus-leafsii/"
	currentSuiPrice := uint64(binance.BinanceScale) // Use $1.00 as default for tests

	// Create binance provider for tests
	logger := zap.NewNop().Sugar()
	provider := binance.NewProvider(logger)

	result, err := initializer.Initialize(ctx, client, signer, corePath, currentSuiPrice, provider)
	if err != nil {
		return fmt.Errorf("failed to initialize protocol: %w", err)
	}

	// Validate that all required IDs were initialized
	if result.ProtocolId == nil || result.PoolId == nil || result.FtokenPackageId == nil || result.XtokenPackageId == nil || result.LeafsiiPackageId == nil {
		return fmt.Errorf("initializer returned nil IDs: protocolId=%v, poolId=%v, ftokenPackageId=%v, xtokenPackageId=%v, leafsiiPackageId=%v",
			result.ProtocolId, result.PoolId, result.FtokenPackageId, result.XtokenPackageId, result.LeafsiiPackageId)
	}

	// Store result globally for use in tests
	e2eInitResult = &result

	fmt.Printf("E2E setup - initialized IDs: protocolId=%s, poolId=%s, ftokenPackageId=%s, xtokenPackageId=%s, leafsiiPackageId=%s\n",
		result.ProtocolId, result.PoolId, result.FtokenPackageId, result.XtokenPackageId, result.LeafsiiPackageId)

	return nil
}

// e2eCheckLocalnetReady checks if localnet is already running
func e2eCheckLocalnetReady(ctx context.Context) error {
	client := suiclient.NewClient(e2eLocalnetRPCURL)
	_, err := client.GetChainIdentifier(ctx)
	return err
}

// e2eWaitForLocalnetReady waits for localnet to be ready with retries
func e2eWaitForLocalnetReady(ctx context.Context) error {
	readyCtx, cancel := context.WithTimeout(ctx, e2eLocalnetReadyTimeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-readyCtx.Done():
			return fmt.Errorf("localnet ready timeout: %w", readyCtx.Err())
		case <-ticker.C:
			if err := e2eCheckLocalnetReady(readyCtx); err == nil {
				time.Sleep(2 * time.Second) // Give it a bit more time to stabilize
				return nil
			}
		}
	}
}

// e2eNewFundedSignerAndClient creates a funded signer and client
func e2eNewFundedSignerAndClient(t *testing.T) (*suiclient.ClientImpl, *suisigner.Signer) {
	t.Helper()
	client := suiclient.NewClient(conn.LocalnetEndpointUrl)

	// Use default test setup or custom mnemonic if provided
	testMnemonic := "arena garbage light lizard champion weasel produce analyst broken pitch shine gas"
	signer, err := suisigner.NewSignerWithMnemonic(testMnemonic, suicrypto.KeySchemeFlagEd25519)
	require.NoError(t, err)

	err = suiclient.RequestFundFromFaucet(signer.Address, conn.LocalnetFaucetUrl)
	require.NoError(t, err)

	return client, signer
}

// e2eSetupHTTPServer creates an in-process HTTP test server with real handler and routes
func e2eSetupHTTPServer(t *testing.T, txBuilder *onchain.TransactionBuilder) *httptest.Server {
	t.Helper()

	// Setup logger
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	sugar := logger.Sugar()

	// Create handler with production wiring
	handler := &Handler{
		logger:      sugar,
		metrics:     &MockMetrics{},
		txBuilder:   txBuilder,
		txSubmitter: txBuilder, // Same builder implements both interfaces
	}

	// Mount routes via handler.Routes() to use production route setup
	r := chi.NewRouter()
	r.Post("/v1/transactions/build", handler.BuildUnsignedTransaction)
	r.Post("/v1/transactions/submit", handler.SubmitSignedTransaction)

	// Return test server
	return httptest.NewServer(r)
}

// e2eTestMintFTokenFlow tests the complete mint ftoken flow
func e2eTestMintFTokenFlow(t *testing.T, server *httptest.Server, client *suiclient.ClientImpl, signer *suisigner.Signer) {
	t.Helper()

	// Step 1: Build unsigned transaction
	buildReq := map[string]interface{}{
		"action":    "mint",
		"tokenType": "ftoken",
		"amount":    "1.5",
	}

	buildResp := e2eHttpPostJSON(t, server, "/v1/transactions/build", buildReq,
		map[string]string{"X-User-Address": signer.Address.String()})

	require.Equal(t, http.StatusOK, buildResp.StatusCode, "Build request should succeed")

	var buildResult struct {
		TransactionBlockBytes []byte `json:"transactionBlockBytes"`
		QuoteID               string `json:"quoteId"`
	}
	err := json.NewDecoder(buildResp.Body).Decode(&buildResult)
	require.NoError(t, err)
	require.NotEmpty(t, buildResult.TransactionBlockBytes, "Should return transaction bytes")
	require.NotEmpty(t, buildResult.QuoteID, "Should return quote ID")

	// Log transaction details for debugging
	t.Logf("Built transaction - Quote ID: %s, Tx bytes length: %d, First 80 chars: %.80s",
		buildResult.QuoteID,
		len(buildResult.TransactionBlockBytes),
		fmt.Sprintf("%x", buildResult.TransactionBlockBytes),
	)

	// Step 2: Sign transaction using real signer for localnet
	sig := e2eSignTransaction(t, client, signer, buildResult.TransactionBlockBytes)
	sigB64 := base64.StdEncoding.EncodeToString(sig.Ed25519SuiSignature.Signature[:])

	// Step 3: Submit signed transaction
	submitReq := map[string]interface{}{
		"tx_bytes":  buildResult.TransactionBlockBytes,
		"signature": sigB64,
		"quoteId":   buildResult.QuoteID,
	}

	submitResp := e2eHttpPostJSON(t, server, "/v1/transactions/submit", submitReq, nil)

	// Log response details for debugging
	if submitResp.StatusCode != http.StatusOK {
		bodyBytes, _ := json.Marshal(submitReq)
		t.Logf("Submit request failed - Status: %d, Request body length: %d, Quote ID: %s",
			submitResp.StatusCode, len(bodyBytes), buildResult.QuoteID)

		// Read response body for error details
		var errorResp map[string]interface{}
		json.NewDecoder(submitResp.Body).Decode(&errorResp)
		t.Logf("Error response: %+v", errorResp)
	}

	require.Equal(t, http.StatusOK, submitResp.StatusCode, "Submit request should succeed")

	var submitResult struct {
		TransactionDigest string `json:"transactionDigest"`
		Status            string `json:"status"`
	}
	err = json.NewDecoder(submitResp.Body).Decode(&submitResult)
	require.NoError(t, err)
	require.NotEmpty(t, submitResult.TransactionDigest, "Should return transaction digest")
	assert.Equal(t, "success", submitResult.Status, "Transaction should be successful")

	t.Logf("E2E mint transaction successful: %s", submitResult.TransactionDigest)
}

// e2eSignTransaction signs transaction bytes using a real signer for localnet
func e2eSignTransaction(t *testing.T, client *suiclient.ClientImpl, signer *suisigner.Signer, txBytes []byte) *suisigner.Signature {
	t.Helper()

	signature, err := signer.SignDigest(txBytes, suisigner.IntentTransaction())
	require.NoError(t, err)
	return signature
}

// e2eHttpPostJSON makes a POST request with JSON body
func e2eHttpPostJSON(t *testing.T, server *httptest.Server, path string, body interface{}, headers map[string]string) *http.Response {
	t.Helper()

	jsonData, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", server.URL+path, strings.NewReader(string(jsonData)))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

// e2eCleanup stops sui localnet if we started it
func e2eCleanup() {
	if e2eSuiProcess != nil && e2eSuiProcess.Process != nil {
		e2eSuiProcess.Process.Kill()
		e2eSuiProcess.Wait()
	}
}
