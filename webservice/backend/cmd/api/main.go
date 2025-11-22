package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/leafsii/leafsii-backend/internal/api"
	"github.com/leafsii/leafsii-backend/internal/config"
	"github.com/leafsii/leafsii-backend/internal/crosschain"
	gdb "github.com/leafsii/leafsii-backend/internal/db"
	"github.com/leafsii/leafsii-backend/internal/jobs"
	"github.com/leafsii/leafsii-backend/internal/log"
	"github.com/leafsii/leafsii-backend/internal/markets"
	"github.com/leafsii/leafsii-backend/internal/metrics"
	"github.com/leafsii/leafsii-backend/internal/onchain"
	"github.com/leafsii/leafsii-backend/internal/prices/binance"
	"github.com/leafsii/leafsii-backend/internal/store"
	"github.com/leafsii/leafsii-backend/internal/ws"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Setup logger
	logger, err := log.NewSugar(cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Infow("Starting FX Protocol API server",
		"env", cfg.Env,
		"addr", cfg.HTTPAddr,
		"version", "v1.0.0",
	)

	// Setup metrics
	metricsObj, metricsHandler, err := metrics.Setup("fx-api")
	if err != nil {
		logger.Fatalw("Failed to setup metrics", "error", err)
	}

	// Initialize database via abstraction (defaults to in-memory with USE_IN_MEMORY=true)
	// Factory reads env: DB_TYPE, DB_DSN, USE_IN_MEMORY
	db := gdb.MustNewDatabase(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := gdb.ConnectAndMigrate(ctx, db, gdb.AllSchemas()); err != nil {
		logger.Fatalw("Failed to initialize database", "error", err)
	}
	logger.Infow("Database initialized")

	// Setup Redis cache
	cache, err := store.NewCache(cfg.Cache.RedisAddr, logger, metricsObj)
	if err != nil {
		logger.Fatalw("Failed to setup cache", "error", err)
	}
	defer cache.Close()

	// Test cache connection
	if err := cache.Ping(ctx); err != nil {
		logger.Fatalw("Cache ping failed", "error", err)
	}
	logger.Infow("Cache connection established")

	// Get required IDs for chain client and transaction builder
	packageId, err := cfg.Sui.GetPackageId()
	if err != nil {
		logger.Fatalw("Invalid package ID", "error", err)
	}
	protocolId, err := cfg.Sui.GetProtocolId()
	if err != nil {
		logger.Fatalw("Invalid protocol ID", "error", err)
	}
	poolId, err := cfg.Sui.GetPoolId()
	if err != nil {
		logger.Fatalw("Invalid pool ID", "error", err)
	}
	ftokenPackageId, err := cfg.Sui.GetFtokenPackageId()
	if err != nil {
		logger.Fatalw("Invalid ftoken package ID", "error", err)
	}
	xtokenPackageId, err := cfg.Sui.GetXtokenPackageId()
	if err != nil {
		logger.Fatalw("Invalid xtoken package ID", "error", err)
	}
	adminCapId, err := cfg.Sui.GetAdminCapId()
	if err != nil {
		logger.Fatalw("Invalid admin cap ID", "error", err)
	}

	// Setup price provider for chain client
	var priceProvider *binance.Provider
	if cfg.Prices.Provider == "binance" {
		priceProvider = binance.NewProvider(logger)
	}

	// Setup Sui chain client
	chainClient := onchain.NewClientWithOptions(
		cfg.Sui.RPCURL,
		cfg.Sui.WSURL,
		cfg.Sui.LeafsiiPackageId,
		cfg.Sui.PoolId,
		cfg.Sui.Network,
		onchain.ClientOptions{
			ProtocolId:       protocolId,
			PoolId:           poolId,
			FtokenPackageId:  ftokenPackageId,
			XtokenPackageId:  xtokenPackageId,
			LeafsiiPackageId: packageId,
			Provider:         priceProvider,
		},
	)

	txBuilder := onchain.NewTransactionBuilder(
		cfg.Sui.RPCURL,
		cfg.Sui.Network,
		packageId,
		protocolId,
		poolId,
		adminCapId,
		ftokenPackageId,
		xtokenPackageId,
	)

	// Setup services
	protocolSvc := onchain.NewProtocolService(chainClient, cache, cfg, logger)
	quoteSvc := onchain.NewQuoteService(chainClient, cache, protocolSvc, cfg, logger)
	userSvc := onchain.NewUserService(chainClient, cache, logger)
	spSvc := onchain.NewStabilityPoolService(chainClient, cache, logger)
	crosschainSvc := crosschain.NewService(logger)
	bridgeOpts := []crosschain.BridgeWorkerOption{}

	if minter, err := crosschain.NewSuiBridgeMinterFromEnv(logger); err != nil {
		logger.Warnw("Bridge mint handler disabled", "error", err)
	} else if minter != nil {
		bridgeOpts = append(bridgeOpts, crosschain.WithMintHandler(minter))
	}
	if listener, err := crosschain.NewSuiBridgeRedeemListenerFromEnv(logger); err != nil {
		logger.Warnw("Bridge redeem listener disabled", "error", err)
	} else if listener != nil {
		bridgeOpts = append(bridgeOpts, crosschain.WithRedeemListener(listener))
	}

	bridgeWorker := crosschain.NewBridgeWorker(crosschainSvc, logger, bridgeOpts...)
	marketsSvc := markets.NewService()

	// Setup WebSocket hub and SSE handler
	wsHub := ws.NewHub(cache, logger, metricsObj)
	sseHandler := ws.NewSSEHandler(cache, logger)

	// Create context for background services
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()

	// Start WebSocket hub in background
	go wsHub.Run(hubCtx)
	bridgeWorker.Start(hubCtx)

	// Setup and start price publisher with config
	pricePublisherConfig := jobs.PricePublisherConfig{
		ProviderType:   cfg.Prices.Provider,
		RetryInterval:  cfg.Prices.RetryInterval,
		MaxTicksPerSym: 10000, // Keep fixed for now
		TTL:            5 * time.Second,
		MockVolatility: cfg.Prices.MockVolatility,
		MockBasePrice:  cfg.Prices.MockBasePrice,
	}

	pricePublisher := jobs.NewPricePublisher(cache, logger, pricePublisherConfig)
	go func() {
		logger.Infow("Starting price publisher",
			"provider", cfg.Prices.Provider,
			"retryInterval", cfg.Prices.RetryInterval,
		)
		if err := pricePublisher.Start(hubCtx); err != nil && err != context.Canceled {
			logger.Errorw("Price publisher error", "error", err)
		}
	}()

	// Setup API handler and middleware
	handler := api.NewHandler(protocolSvc, quoteSvc, userSvc, spSvc, crosschainSvc, bridgeWorker, marketsSvc, wsHub, sseHandler, cache, cfg, logger, metricsObj, txBuilder, txBuilder)
	middleware := api.NewMiddleware(logger, metricsObj)

	// Create router with middleware and routes - pass security config to Routes
	router := handler.Routes(middleware, cfg.Security.CORSAllowedOrigins, cfg.Security.RateLimitRPM)

	// Log configured CORS origins for easier debugging in dev
	logger.Infow("CORS configured", "allowed_origins", cfg.Security.CORSAllowedOrigins)

	// Add metrics endpoint
	router.Handle("/metrics", metricsHandler)

	// Setup HTTP server
	server := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in background
	serverErrors := make(chan error, 1)
	go func() {
		logger.Infow("API server starting", "addr", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	// Wait for interrupt signal
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Fatalw("Server startup failed", "error", err)
	case sig := <-shutdown:
		logger.Infow("Shutdown signal received", "signal", sig.String())

		// Give outstanding requests 30 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logger.Errorw("Graceful shutdown failed", "error", err)
			server.Close()
		}

		logger.Infow("Server stopped")
	}
}
