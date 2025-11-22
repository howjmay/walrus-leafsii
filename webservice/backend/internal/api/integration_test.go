package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/leafsii/leafsii-backend/internal/onchain"
	"github.com/pattonkan/sui-go/sui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Integration test that uses real transaction builder but with mock Sui client
func TestBuildUnsignedTransactionIntegration(t *testing.T) {
	// Skip integration tests if not in integration test environment
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup logger
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	sugar := logger.Sugar()

	// Setup mock metrics
	metricsObj := &MockMetrics{}

	// Create required parameters for NewTransactionBuilder
	packageId := sui.MustPackageIdFromHex("0x1234567890abcdef1234567890abcdef12345678")
	protocolId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000001")
	poolId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000002")
	adminCapId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000005")
	ftokenPackageId := sui.MustPackageIdFromHex("0x0000000000000000000000000000000000000003")
	xtokenPackageId := sui.MustPackageIdFromHex("0x0000000000000000000000000000000000000004")

	// Setup real transaction builder with test config
	txBuilder := onchain.NewTransactionBuilder(
		"http://localhost:9000", // Local Sui node
		"localnet",
		packageId,
		protocolId,
		poolId,
		adminCapId,
		ftokenPackageId,
		xtokenPackageId,
	)

	// Create handler with real transaction builder
	handler := &Handler{
		logger:    sugar,
		metrics:   metricsObj,
		txBuilder: txBuilder,
	}

	// Setup router
	r := chi.NewRouter()
	r.Post("/v1/transactions/build", handler.BuildUnsignedTransaction)

	testCases := []struct {
		name     string
		request  UnsignedTransactionRequest
		userAddr string
	}{
		{
			name: "integration_mint_ftoken",
			request: UnsignedTransactionRequest{
				Action:    "mint",
				TokenType: "ftoken",
				Amount:    "100.5",
			},
			userAddr: "0x9876543210fedcba9876543210fedcba98765432",
		},
		{
			name: "integration_redeem_xtoken",
			request: UnsignedTransactionRequest{
				Action:    "redeem",
				TokenType: "xtoken",
				Amount:    "50.25",
			},
			userAddr: "0x9876543210fedcba9876543210fedcba98765432",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tc.request)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-Address", tc.userAddr)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// The transaction builder will fail with real Sui calls since we're not connected to a real network
			// But we can test that the request parsing and validation works correctly
			if w.Code == http.StatusOK {
				var response UnsignedTransactionResponse
				err = json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.NotEmpty(t, response.TransactionBlockBytes)
				assert.NotEmpty(t, response.GasEstimate)
				assert.NotEmpty(t, response.QuoteID)
				assert.Equal(t, tc.request.Action, response.Metadata["action"])
				assert.Equal(t, tc.request.TokenType, response.Metadata["tokenType"])
				assert.Equal(t, tc.request.Amount, response.Metadata["amount"])
				assert.Equal(t, "localnet", response.Metadata["network"])
			} else {
				// If it fails, it should be due to network/client issues, not validation
				var errorResp ErrorResponse
				err = json.Unmarshal(w.Body.Bytes(), &errorResp)
				require.NoError(t, err)
				// Should not be validation errors
				assert.NotEqual(t, "INVALID_ACTION", errorResp.Code)
				assert.NotEqual(t, "INVALID_TOKEN_TYPE", errorResp.Code)
				assert.NotEqual(t, "INVALID_AMOUNT", errorResp.Code)
				assert.NotEqual(t, "MISSING_USER_ADDRESS", errorResp.Code)
				assert.NotEqual(t, "INVALID_USER_ADDRESS", errorResp.Code)
			}
		})
	}
}

// Benchmark test for transaction building performance
func BenchmarkBuildUnsignedTransaction(b *testing.B) {
	// Setup
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	// Create required parameters for NewTransactionBuilder
	packageId := sui.MustPackageIdFromHex("0x1234567890abcdef1234567890abcdef12345678")
	protocolId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000001")
	poolId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000002")
	adminCapId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000005")
	ftokenPackageId := sui.MustPackageIdFromHex("0x0000000000000000000000000000000000000003")
	xtokenPackageId := sui.MustPackageIdFromHex("0x0000000000000000000000000000000000000004")

	txBuilder := onchain.NewTransactionBuilder(
		"http://localhost:9000",
		"localnet",
		packageId,
		protocolId,
		poolId,
		adminCapId,
		ftokenPackageId,
		xtokenPackageId,
	)

	handler := &Handler{
		logger:    sugar,
		metrics:   &MockMetrics{},
		txBuilder: txBuilder,
	}

	request := UnsignedTransactionRequest{
		Action:    "mint",
		TokenType: "ftoken",
		Amount:    "100.5",
	}

	reqBody, _ := json.Marshal(request)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-Address", "0x9876543210fedcba9876543210fedcba98765432")

			w := httptest.NewRecorder()
			handler.BuildUnsignedTransaction(w, req)

			// Note: This will likely fail in benchmark due to network calls
			// but tests the parsing and validation performance
		}
	})
}

// Test concurrent requests
func TestBuildUnsignedTransactionConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test in short mode")
	}

	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	// Create required parameters for NewTransactionBuilder
	packageId := sui.MustPackageIdFromHex("0x1234567890abcdef1234567890abcdef12345678")
	protocolId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000001")
	poolId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000002")
	adminCapId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000005")
	ftokenPackageId := sui.MustPackageIdFromHex("0x0000000000000000000000000000000000000003")
	xtokenPackageId := sui.MustPackageIdFromHex("0x0000000000000000000000000000000000000004")

	txBuilder := onchain.NewTransactionBuilder(
		"http://localhost:9000",
		"localnet",
		packageId,
		protocolId,
		poolId,
		adminCapId,
		ftokenPackageId,
		xtokenPackageId,
	)

	handler := &Handler{
		logger:    sugar,
		metrics:   &MockMetrics{},
		txBuilder: txBuilder,
	}

	r := chi.NewRouter()
	r.Post("/v1/transactions/build", handler.BuildUnsignedTransaction)

	// Test concurrent requests
	const numGoroutines = 10
	const requestsPerGoroutine = 5

	results := make(chan int, numGoroutines*requestsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			for j := 0; j < requestsPerGoroutine; j++ {
				request := UnsignedTransactionRequest{
					Action:    "mint",
					TokenType: "ftoken",
					Amount:    fmt.Sprintf("100.%d", workerID*10+j),
				}

				reqBody, _ := json.Marshal(request)
				req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build", bytes.NewReader(reqBody))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-User-Address", "0x9876543210fedcba9876543210fedcba98765432")

				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				results <- w.Code
			}
		}(i)
	}

	// Collect results
	statusCodes := make(map[int]int)
	for i := 0; i < numGoroutines*requestsPerGoroutine; i++ {
		code := <-results
		statusCodes[code]++
	}

	// Verify that we got responses (even if they're errors due to network issues)
	totalRequests := numGoroutines * requestsPerGoroutine
	totalResponses := 0
	for _, count := range statusCodes {
		totalResponses += count
	}

	assert.Equal(t, totalRequests, totalResponses, "All requests should get responses")

	// Should not have any validation errors in concurrent scenario
	assert.Equal(t, 0, statusCodes[http.StatusBadRequest], "Should not have validation errors")
}

// Test with malformed requests to ensure proper error handling
func TestBuildUnsignedTransactionMalformedRequests(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	// Create required parameters for NewTransactionBuilder
	packageId := sui.MustPackageIdFromHex("0x1234567890abcdef1234567890abcdef12345678")
	protocolId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000001")
	poolId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000002")
	adminCapId := sui.MustObjectIdFromHex("0x0000000000000000000000000000000000000005")
	ftokenPackageId := sui.MustPackageIdFromHex("0x0000000000000000000000000000000000000003")
	xtokenPackageId := sui.MustPackageIdFromHex("0x0000000000000000000000000000000000000004")

	txBuilder := onchain.NewTransactionBuilder(
		"http://localhost:9000",
		"localnet",
		packageId,
		protocolId,
		poolId,
		adminCapId,
		ftokenPackageId,
		xtokenPackageId,
	)

	handler := &Handler{
		logger:    sugar,
		metrics:   &MockMetrics{},
		txBuilder: txBuilder,
	}

	malformedRequests := []struct {
		name        string
		body        string
		expectError bool
	}{
		{"empty_body", "", true},
		{"invalid_json", "{invalid json}", true},
		{"partial_json", `{"action": "mint"`, true},
		{"extra_fields", `{"action": "mint", "tokenType": "ftoken", "amount": "100", "extraField": "value"}`, false}, // Extra fields are OK
		{"null_values", `{"action": null, "tokenType": "ftoken", "amount": "100"}`, true},
	}

	for _, test := range malformedRequests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build", bytes.NewReader([]byte(test.body)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-Address", "0x9876543210fedcba9876543210fedcba98765432")

			w := httptest.NewRecorder()
			handler.BuildUnsignedTransaction(w, req)

			if test.expectError {
				// Should return 400 for malformed requests
				assert.Equal(t, http.StatusBadRequest, w.Code)

				var errorResp ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &errorResp)
				require.NoError(t, err)
				assert.NotEmpty(t, errorResp.Code)
				assert.NotEmpty(t, errorResp.Message)
			} else {
				// Should succeed or fail due to network issues, not validation
				assert.NotEqual(t, http.StatusBadRequest, w.Code)
			}
		})
	}
}
