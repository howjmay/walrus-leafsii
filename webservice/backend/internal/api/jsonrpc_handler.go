package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/leafsii/leafsii-backend/internal/onchain"
	"github.com/pattonkan/sui-go/sui"
	"github.com/shopspring/decimal"
)

// HandleJSONRPC handles JSON-RPC 2.0 requests
func (h *Handler) HandleJSONRPC(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse JSON-RPC request
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSONRPCError(w, r, nil, JSONRPCParseError, "Parse error", err.Error())
		return
	}

	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidRequest, "Invalid Request", "jsonrpc must be '2.0'")
		return
	}

	// Handle method
	switch req.Method {
	case "getUnsignedTransaction":
		h.handleGetUnsignedTransaction(w, r, &req)
	default:
		h.sendJSONRPCError(w, r, req.ID, JSONRPCMethodNotFound, "Method not found", fmt.Sprintf("Method '%s' not found", req.Method))
	}
}

func (h *Handler) handleGetUnsignedTransaction(w http.ResponseWriter, r *http.Request, req *JSONRPCRequest) {
	// Parse parameters
	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid params", "Failed to parse parameters")
		return
	}

	var params GetUnsignedTransactionParams
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid params", err.Error())
		return
	}

	// Validate parameters manually (same logic as REST handler)
	if params.Operation != "mint" && params.Operation != "redeem" {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid operation", "operation must be 'mint' or 'redeem'")
		return
	}

	if params.Token != "xtoken" && params.Token != "ftoken" {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid token", "token must be 'xtoken' or 'ftoken'")
		return
	}

	if params.Amount == "" {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid amount", "amount is required")
		return
	}

	if params.UserAddress == "" {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid userAddress", "userAddress is required")
		return
	}

	// Parse amount
	amount, err := decimal.NewFromString(params.Amount)
	if err != nil {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid amount", "Amount must be a valid decimal number")
		return
	}

	if amount.LessThanOrEqual(decimal.Zero) {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid amount", "Amount must be greater than zero")
		return
	}

	// Parse user address
	userAddr, err := sui.AddressFromHex(params.UserAddress)
	if err != nil {
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid user address", "User address must be a valid Sui address")
		return
	}

	// Determine mode from params (defaulting to execution mode)
	mode := onchain.TxBuildModeExecution
	// Add devInspect param support if needed later

	// Build transaction based on operation
	var unsignedTx *onchain.UnsignedTransaction
	ctx := r.Context()

	switch params.Operation {
	case "mint":
		unsignedTx, err = h.txBuilder.BuildMintTransaction(ctx, onchain.MintTxRequest{
			OutTokenType: params.Token,
			Amount:       amount,
			UserAddress:  userAddr,
			Mode:         mode,
		})
	case "redeem":
		unsignedTx, err = h.txBuilder.BuildRedeemTransaction(ctx, onchain.RedeemTxRequest{
			InTokenType: params.Token,
			Amount:      amount,
			UserAddress: userAddr,
			Mode:        mode,
		})
	default:
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInvalidParams, "Invalid operation", "Operation must be 'mint' or 'redeem'")
		return
	}

	if err != nil {
		h.logger.Errorw("Failed to build transaction", "error", err, "operation", params.Operation, "token", params.Token, "amount", params.Amount)
		h.sendJSONRPCError(w, r, req.ID, JSONRPCInternalError, "Internal error", "Failed to build transaction")
		return
	}

	// Create response
	result := GetUnsignedTransactionResult{
		TxBytes: unsignedTx.TransactionBlockBytes,
	}

	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}

	// Send response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	// Log metrics using the same pattern as REST handler
	h.metrics.RecordHTTPRequest(ctx, r.Method, r.URL.Path, http.StatusOK, 0)
}

func (h *Handler) sendJSONRPCError(w http.ResponseWriter, r *http.Request, id interface{}, code int, message string, data interface{}) {
	errorResp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	w.WriteHeader(http.StatusOK) // JSON-RPC errors are sent with HTTP 200
	json.NewEncoder(w).Encode(errorResp)

	// Log error metrics using similar pattern
	h.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, http.StatusBadRequest, 0)
}
