package api

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (h *Handler) Routes(m *Middleware, corsOrigins []string, rateLimitRPM int) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(m.RequestID)
	r.Use(m.RequestLogger)
	r.Use(m.Recoverer)
	r.Use(m.SecurityHeaders)
	r.Use(m.Compress)
	r.Use(m.Timeout(15 * time.Second))
	r.Use(middleware.Heartbeat("/ping"))

	// CORS and rate limiting - configured from main
	r.Use(m.CORS(corsOrigins))
	r.Use(m.RateLimit(rateLimitRPM))

	// Health endpoints
	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Readyz)

	// v1 API routes
	r.Route("/v1", func(r chi.Router) {
		// JSON-RPC endpoint
		r.Post("/jsonrpc", h.HandleJSONRPC)

		// Markets
		r.Get("/markets", h.ListMarkets)

		// Protocol & Metrics
		r.Route("/protocol", func(r chi.Router) {
			r.Get("/state", h.GetProtocolState)
			r.Get("/health", h.GetProtocolHealth)
			r.Get("/build-info", h.GetTransactionBuildInfo)
			r.Get("/metrics", h.GetProtocolMetrics)
			// TODO: Add rebalances endpoint
		})

		// Quotes & Previews
		r.Route("/quotes", func(r chi.Router) {
			r.Get("/mintF", h.GetQuoteMintF)
			r.Get("/redeemF", h.GetQuoteRedeemF)
			r.Get("/mintX", h.GetQuoteMintX)
			r.Get("/redeemX", h.GetQuoteRedeemX)
			// TODO: Add stake quote endpoint
		})

		// Transaction Building
		r.Route("/transactions", func(r chi.Router) {
			r.Post("/build", h.BuildUnsignedTransaction)
			r.Post("/submit", h.SubmitSignedTransaction)
			r.Post("/monitor", h.ReportTransactionAttempt)
		})

		// Stability Pool
		r.Route("/sp", func(r chi.Router) {
			r.Get("/index", h.GetSPIndex)
			r.Get("/user/{address}", h.GetSPUser)
		})

		// User Portfolio
		r.Route("/users", func(r chi.Router) {
			r.Get("/{address}/positions", h.GetUserPositions)
			r.Get("/{address}/balances", h.GetUserBalances)
			r.Get("/{address}/transactions", h.GetUserTransactions)
		})

		// Chart data
		r.Get("/candles", h.GetCandles)

		// Oracle management
		r.Route("/oracle", func(r chi.Router) {
			r.Post("/update/build", h.BuildUpdateOracleTransaction)
			r.Post("/update/submit", h.SubmitUpdateOracleTransaction)
		})

		// Live updates
		r.Get("/stream", h.HandleSSE)
		r.Get("/ws", h.HandleWebSocket)

		// Cross-chain collateral (ETH on Ethereum -> Sui)
		r.Route("/crosschain", func(r chi.Router) {
			r.Get("/checkpoint", h.GetLatestCheckpoint)
			r.Post("/checkpoint", h.SubmitCheckpoint)
			r.Post("/deposit", h.SubmitCrossChainDeposit)
			r.Post("/redeem", h.SubmitCrossChainRedeem)
			r.Get("/balance", h.GetCrossChainBalance)
			r.Get("/voucher", h.GetVoucher)
			r.Get("/vouchers", h.ListVouchers)
			r.Post("/voucher", h.CreateVoucher)
			r.Get("/params", h.GetCollateralParams)
			r.Get("/vault", h.GetVaultInfo)
		})
	})

	return r
}
