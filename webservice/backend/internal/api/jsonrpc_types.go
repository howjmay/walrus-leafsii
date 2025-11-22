package api

// JSON-RPC 2.0 request structure
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// JSON-RPC 2.0 response structure
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSON-RPC 2.0 error structure
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// getUnsignedTransaction method parameters
type GetUnsignedTransactionParams struct {
	Operation   string `json:"operation"`
	Token       string `json:"token"`
	Amount      string `json:"amount"`
	UserAddress string `json:"userAddress"`
}

// getUnsignedTransaction method result
type GetUnsignedTransactionResult struct {
	TxBytes []byte `json:"txBytes"`
}

// JSON-RPC error codes (following standard)
const (
	JSONRPCParseError     = -32700
	JSONRPCInvalidRequest = -32600
	JSONRPCMethodNotFound = -32601
	JSONRPCInvalidParams  = -32602
	JSONRPCInternalError  = -32603
)
