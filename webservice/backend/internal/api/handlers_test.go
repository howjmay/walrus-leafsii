package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leafsii/leafsii-backend/internal/onchain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Mock transaction builder for testing
type MockTransactionBuilder struct {
	mock.Mock
}

func (m *MockTransactionBuilder) BuildMintTransaction(ctx context.Context, req onchain.MintTxRequest) (*onchain.UnsignedTransaction, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*onchain.UnsignedTransaction), args.Error(1)
}

func (m *MockTransactionBuilder) BuildRedeemTransaction(ctx context.Context, req onchain.RedeemTxRequest) (*onchain.UnsignedTransaction, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*onchain.UnsignedTransaction), args.Error(1)
}

func (m *MockTransactionBuilder) BuildUpdateOracleTransaction(ctx context.Context, req onchain.UpdateOracleTxRequest) (*onchain.UnsignedTransaction, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*onchain.UnsignedTransaction), args.Error(1)
}

// Ensure MockTransactionBuilder implements the interface
var _ onchain.TransactionBuilderInterface = (*MockTransactionBuilder)(nil)

// Mock metrics for testing
type MockMetrics struct{}

func (m *MockMetrics) RecordHTTPRequest(ctx context.Context, method, path string, status int, duration time.Duration) {
}

func createTestHandler() (*Handler, *MockTransactionBuilder) {
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	mockTxBuilder := &MockTransactionBuilder{}
	mockMetrics := &MockMetrics{}

	handler := &Handler{
		logger:    sugar,
		metrics:   mockMetrics,
		txBuilder: mockTxBuilder,
	}

	return handler, mockTxBuilder
}

func TestBuildUnsignedTransaction_Success(t *testing.T) {
	handler, mockTxBuilder := createTestHandler()

	testCases := []struct {
		name      string
		request   UnsignedTransactionRequest
		userAddr  string
		setupMock func()
	}{
		{
			name: "mint ftoken",
			request: UnsignedTransactionRequest{
				Action:    "mint",
				TokenType: "ftoken",
				Amount:    "100.5",
			},
			userAddr: "0x1234567890abcdef1234567890abcdef12345678",
			setupMock: func() {
				mockTxBuilder.On("BuildMintTransaction",
					mock.Anything, mock.MatchedBy(func(req onchain.MintTxRequest) bool {
						return req.OutTokenType == "ftoken" && req.Amount.String() == "100.5"
					})).
					Return(&onchain.UnsignedTransaction{
						TransactionBlockBytes: []byte("base64encodedtransaction"),
						GasEstimate:           1000000,
						Metadata: map[string]string{
							"action":    "mint",
							"tokenType": "ftoken",
							"amount":    "100.5",
							"network":   "testnet",
						},
					}, nil)
			},
		},
		{
			name: "mint xtoken",
			request: UnsignedTransactionRequest{
				Action:    "mint",
				TokenType: "xtoken",
				Amount:    "50.25",
			},
			userAddr: "0x1234567890abcdef1234567890abcdef12345678",
			setupMock: func() {
				mockTxBuilder.On("BuildMintTransaction",
					mock.Anything, mock.MatchedBy(func(req onchain.MintTxRequest) bool {
						return req.OutTokenType == "xtoken" && req.Amount.String() == "50.25"
					})).
					Return(&onchain.UnsignedTransaction{
						TransactionBlockBytes: []byte("base64encodedtransaction"),
						GasEstimate:           1000000,
						Metadata: map[string]string{
							"action":    "mint",
							"tokenType": "xtoken",
							"amount":    "50.25",
							"network":   "testnet",
						},
					}, nil)
			},
		},
		{
			name: "redeem ftoken",
			request: UnsignedTransactionRequest{
				Action:    "redeem",
				TokenType: "ftoken",
				Amount:    "75.0",
			},
			userAddr: "0x1234567890abcdef1234567890abcdef12345678",
			setupMock: func() {
				mockTxBuilder.On("BuildRedeemTransaction",
					mock.Anything, mock.MatchedBy(func(req onchain.RedeemTxRequest) bool {
						return req.InTokenType == "ftoken" && req.Amount.String() == "75"
					})).
					Return(&onchain.UnsignedTransaction{
						TransactionBlockBytes: []byte("base64encodedtransaction"),
						GasEstimate:           1000000,
						Metadata: map[string]string{
							"action":    "redeem",
							"tokenType": "ftoken",
							"amount":    "75",
							"network":   "testnet",
						},
					}, nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()

			reqBody, err := json.Marshal(tc.request)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-Address", tc.userAddr)

			w := httptest.NewRecorder()
			handler.BuildUnsignedTransaction(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response UnsignedTransactionResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, []byte("base64encodedtransaction"), response.TransactionBlockBytes)
			assert.Equal(t, "1000000", response.GasEstimate)
			assert.NotEmpty(t, response.QuoteID)
			assert.Equal(t, tc.request.Action, response.Metadata["action"])
			assert.Equal(t, tc.request.TokenType, response.Metadata["tokenType"])

			mockTxBuilder.AssertExpectations(t)
			mockTxBuilder.ExpectedCalls = nil // Reset for next test
		})
	}
}

func TestBuildUnsignedTransaction_UserAddressFromQuery(t *testing.T) {
	handler, mockTxBuilder := createTestHandler()

	mockTxBuilder.On("BuildMintTransaction",
		mock.Anything, mock.MatchedBy(func(req onchain.MintTxRequest) bool {
			return req.OutTokenType == "ftoken" && req.Amount.Equal(decimal.NewFromFloat(100.5))
		})).
		Return(&onchain.UnsignedTransaction{
			TransactionBlockBytes: []byte("base64encodedtransaction"),
			GasEstimate:           1000000,
			Metadata: map[string]string{
				"action":    "mint",
				"tokenType": "ftoken",
				"amount":    "100.5",
				"network":   "testnet",
			},
		}, nil)

	request := UnsignedTransactionRequest{
		Action:    "mint",
		TokenType: "ftoken",
		Amount:    "100.5",
	}

	reqBody, err := json.Marshal(request)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build?userAddress=0x1234567890abcdef1234567890abcdef12345678", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.BuildUnsignedTransaction(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockTxBuilder.AssertExpectations(t)
}

func TestBuildUnsignedTransaction_ValidationErrors(t *testing.T) {
	handler, _ := createTestHandler()

	testCases := []struct {
		name           string
		request        interface{}
		userAddr       string
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "invalid JSON",
			request:        "invalid json",
			userAddr:       "0x1234567890abcdef1234567890abcdef12345678",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_JSON",
		},
		{
			name: "invalid action",
			request: UnsignedTransactionRequest{
				Action:    "invalid",
				TokenType: "ftoken",
				Amount:    "100.5",
			},
			userAddr:       "0x1234567890abcdef1234567890abcdef12345678",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_ACTION",
		},
		{
			name: "invalid token type",
			request: UnsignedTransactionRequest{
				Action:    "mint",
				TokenType: "invalid",
				Amount:    "100.5",
			},
			userAddr:       "0x1234567890abcdef1234567890abcdef12345678",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_TOKEN_TYPE",
		},
		{
			name: "invalid amount format",
			request: UnsignedTransactionRequest{
				Action:    "mint",
				TokenType: "ftoken",
				Amount:    "invalid",
			},
			userAddr:       "0x1234567890abcdef1234567890abcdef12345678",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_AMOUNT",
		},
		{
			name: "zero amount",
			request: UnsignedTransactionRequest{
				Action:    "mint",
				TokenType: "ftoken",
				Amount:    "0",
			},
			userAddr:       "0x1234567890abcdef1234567890abcdef12345678",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_AMOUNT",
		},
		{
			name: "negative amount",
			request: UnsignedTransactionRequest{
				Action:    "mint",
				TokenType: "ftoken",
				Amount:    "-100",
			},
			userAddr:       "0x1234567890abcdef1234567890abcdef12345678",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_AMOUNT",
		},
		{
			name: "missing user address",
			request: UnsignedTransactionRequest{
				Action:    "mint",
				TokenType: "ftoken",
				Amount:    "100.5",
			},
			userAddr:       "",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "MISSING_USER_ADDRESS",
		},
		{
			name: "invalid user address format",
			request: UnsignedTransactionRequest{
				Action:    "mint",
				TokenType: "ftoken",
				Amount:    "100.5",
			},
			userAddr:       "invalid_address",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_USER_ADDRESS",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var reqBody []byte
			var err error

			if str, ok := tc.request.(string); ok {
				reqBody = []byte(str)
			} else {
				reqBody, err = json.Marshal(tc.request)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			if tc.userAddr != "" {
				req.Header.Set("X-User-Address", tc.userAddr)
			}

			w := httptest.NewRecorder()
			handler.BuildUnsignedTransaction(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)

			var errorResp ErrorResponse
			err = json.Unmarshal(w.Body.Bytes(), &errorResp)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCode, errorResp.Code)
		})
	}
}

func TestBuildUnsignedTransaction_TransactionBuildError(t *testing.T) {
	handler, mockTxBuilder := createTestHandler()

	mockTxBuilder.On("BuildMintTransaction",
		mock.Anything, mock.MatchedBy(func(req onchain.MintTxRequest) bool {
			return req.OutTokenType == "ftoken" && req.Amount.Equal(decimal.NewFromFloat(100.5))
		})).
		Return(nil, assert.AnError)

	request := UnsignedTransactionRequest{
		Action:    "mint",
		TokenType: "ftoken",
		Amount:    "100.5",
	}

	reqBody, err := json.Marshal(request)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Address", "0x1234567890abcdef1234567890abcdef12345678")

	w := httptest.NewRecorder()
	handler.BuildUnsignedTransaction(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var errorResp ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &errorResp)
	require.NoError(t, err)
	assert.Equal(t, "TRANSACTION_BUILD_ERROR", errorResp.Code)

	mockTxBuilder.AssertExpectations(t)
}

func TestBuildUnsignedTransaction_EdgeCases(t *testing.T) {
	handler, mockTxBuilder := createTestHandler()

	t.Run("very small amount", func(t *testing.T) {
		mockTxBuilder.On("BuildMintTransaction",
			mock.Anything, mock.MatchedBy(func(req onchain.MintTxRequest) bool {
				return req.OutTokenType == "ftoken" && req.Amount.String() == "0.000000001"
			})).
			Return(&onchain.UnsignedTransaction{
				TransactionBlockBytes: []byte("base64encodedtransaction"),
				GasEstimate:           1000000,
				Metadata: map[string]string{
					"action":    "mint",
					"tokenType": "ftoken",
					"amount":    "0.000000001",
					"network":   "testnet",
				},
			}, nil)

		request := UnsignedTransactionRequest{
			Action:    "mint",
			TokenType: "ftoken",
			Amount:    "0.000000001",
		}

		reqBody, err := json.Marshal(request)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Address", "0x1234567890abcdef1234567890abcdef12345678")

		w := httptest.NewRecorder()
		handler.BuildUnsignedTransaction(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		mockTxBuilder.AssertExpectations(t)
	})

	t.Run("very large amount", func(t *testing.T) {
		mockTxBuilder.ExpectedCalls = nil // Reset
		mockTxBuilder.On("BuildMintTransaction",
			mock.Anything, mock.MatchedBy(func(req onchain.MintTxRequest) bool {
				return req.OutTokenType == "ftoken" && req.Amount.String() == "999999999999999999.999999999"
			})).
			Return(&onchain.UnsignedTransaction{
				TransactionBlockBytes: []byte("base64encodedtransaction"),
				GasEstimate:           1000000,
				Metadata: map[string]string{
					"action":    "mint",
					"tokenType": "ftoken",
					"amount":    "999999999999999999.999999999",
					"network":   "testnet",
				},
			}, nil)

		request := UnsignedTransactionRequest{
			Action:    "mint",
			TokenType: "ftoken",
			Amount:    "999999999999999999.999999999",
		}

		reqBody, err := json.Marshal(request)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/transactions/build", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Address", "0x1234567890abcdef1234567890abcdef12345678")

		w := httptest.NewRecorder()
		handler.BuildUnsignedTransaction(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		mockTxBuilder.AssertExpectations(t)
	})
}
