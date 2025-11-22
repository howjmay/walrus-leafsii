package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	initpkg "github.com/leafsii/leafsii-backend/cmd/initializer/pkg"
	"github.com/pattonkan/sui-go/sui"
	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

type Config struct {
	Env       string `mapstructure:"LFS_ENV"`
	HTTPAddr  string `mapstructure:"LFS_HTTP_ADDR"`
	PublicURL string `mapstructure:"LFS_PUBLIC_ORIGIN"`

	Sui      SuiConfig      `mapstructure:",squash"`
	Database DBConfig       `mapstructure:",squash"`
	Cache    CacheConfig    `mapstructure:",squash"`
	Oracle   OracleConfig   `mapstructure:",squash"`
	Prices   PriceConfig    `mapstructure:",squash"`
	Security SecurityConfig `mapstructure:",squash"`
}

type SuiConfig struct {
	RPCURL           string `mapstructure:"LFS_SUI_RPC_URL"`
	WSURL            string `mapstructure:"LFS_SUI_WS_URL"`
	Network          string `mapstructure:"LFS_NETWORK"`
	LeafsiiPackageId string // Loaded from init.json
	PoolId           string // Loaded from init.json
	FTTreasuryCapId  string `mapstructure:"LFS_SUI_FTOKEN_TREASURY_CAP"`
	XTTreasuryCapId  string `mapstructure:"LFS_SUI_XTOKEN_TREASURY_CAP"`
	FTAuthorityId    string `mapstructure:"LFS_SUI_FTOKEN_AUTHORITY"`
	XTAuthorityId    string `mapstructure:"LFS_SUI_XTOKEN_AUTHORITY"`

	// Loaded from init.json
	initConfig *initpkg.InitConfig
}

type DBConfig struct {
	PostgresDSN string `mapstructure:"LFS_POSTGRES_DSN"`
}

type CacheConfig struct {
	RedisAddr string `mapstructure:"LFS_REDIS_ADDR"`
}

type OracleConfig struct {
	PriceOracleURLs []string      `mapstructure:"LFS_PRICE_ORACLE_URLS"`
	MaxAge          time.Duration `mapstructure:"LFS_ORACLE_MAX_AGE"`
}

type PriceConfig struct {
	Provider       string        `mapstructure:"LFS_PRICE_PROVIDER"`        // "binance", "mock"
	RetryInterval  time.Duration `mapstructure:"LFS_PRICE_RETRY_INTERVAL"`  // Retry failed provider
	HistoryLimit   int           `mapstructure:"LFS_PRICE_HISTORY_LIMIT"`   // Max candles to return
	MockVolatility float64       `mapstructure:"LFS_PRICE_MOCK_VOLATILITY"` // Mock data volatility
	MockBasePrice  float64       `mapstructure:"LFS_PRICE_MOCK_BASE_PRICE"` // Mock base price
}

type SecurityConfig struct {
	RateLimitRPM       int      `mapstructure:"LFS_RATE_LIMIT_RPM"`
	CORSAllowedOrigins []string `mapstructure:"LFS_CORS_ALLOWED_ORIGINS"`
}

func loadDotEnvFiles() {
	candidates := []string{
		".env",
		filepath.Join("backend", ".env"),
		filepath.Join("..", ".env"),
		filepath.Join("..", "backend", ".env"),
	}

	seen := make(map[string]struct{})
	for _, path := range candidates {
		abs := path
		if !filepath.IsAbs(path) {
			if resolved, err := filepath.Abs(path); err == nil {
				abs = resolved
			}
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}

		if _, err := os.Stat(path); err == nil {
			_ = gotenv.Load(path) // ignore errors; env vars already set take precedence
		}
	}
}

func Load() (*Config, error) {
	loadDotEnvFiles()

	viper.SetConfigType("env")
	viper.AutomaticEnv()

	// Set defaults
	viper.SetDefault("LFS_ENV", "dev")
	viper.SetDefault("LFS_HTTP_ADDR", ":8080")
	viper.SetDefault("LFS_PUBLIC_ORIGIN", "http://localhost:3000")
	viper.SetDefault("LFS_NETWORK", "localnet")
	viper.SetDefault("LFS_SUI_RPC_URL", "http://localhost:9000")
	viper.SetDefault("LFS_SUI_WS_URL", "wss://localhost:9000")
	viper.SetDefault("LFS_POSTGRES_DSN", "postgres://user:password@localhost:5432/fx_db?sslmode=disable")
	viper.SetDefault("LFS_REDIS_ADDR", "127.0.0.1:6379")
	viper.SetDefault("LFS_ORACLE_MAX_AGE", "60s")
	viper.SetDefault("LFS_PRICE_PROVIDER", "binance")
	viper.SetDefault("LFS_PRICE_RETRY_INTERVAL", "5s")
	viper.SetDefault("LFS_PRICE_HISTORY_LIMIT", 500)
	viper.SetDefault("LFS_PRICE_MOCK_VOLATILITY", 0.002)
	viper.SetDefault("LFS_PRICE_MOCK_BASE_PRICE", 1.50)
	viper.SetDefault("LFS_RATE_LIMIT_RPM", 120)
	viper.SetDefault("LFS_CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173")

	// Handle array parsing for comma-separated values
	if urls := viper.GetString("LFS_PRICE_ORACLE_URLS"); urls != "" {
		viper.Set("LFS_PRICE_ORACLE_URLS", strings.Split(urls, ","))
	}
	if origins := viper.GetString("LFS_CORS_ALLOWED_ORIGINS"); origins != "" {
		viper.Set("LFS_CORS_ALLOWED_ORIGINS", strings.Split(origins, ","))
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.applyNetworkDefaults()

	// Load initializer config
	if err := cfg.loadInitConfig(); err != nil {
		return nil, fmt.Errorf("failed to load initializer config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// loadInitConfig loads the initializer configuration from init.json
func (c *Config) loadInitConfig() error {
	// Find the init.json file - look in common locations
	initPaths := []string{
		"./backend/cmd/initializer/init.json",
		"./cmd/initializer/init.json",
		"../cmd/initializer/init.json",
		"../../cmd/initializer/init.json",
	}

	// Also try environment variable if provided
	if envPath := os.Getenv("LFS_INIT_CONFIG_PATH"); envPath != "" {
		initPaths = append([]string{envPath}, initPaths...)
	}

	var initConfig initpkg.InitConfig
	var err error
	var foundPath string

	for _, path := range initPaths {
		initConfig, err = initpkg.ReadConfig(path)
		if err == nil {
			foundPath = path
			break
		}
		if !os.IsNotExist(err) {
			// Some other error occurred
			return fmt.Errorf("error reading init config at %s: %w", path, err)
		}
	}

	if foundPath == "" {
		return fmt.Errorf("init.json not found in any of the expected locations: %v", initPaths)
	}

	c.Sui.initConfig = &initConfig

	// Populate the string fields from initConfig
	if initConfig.LeafsiiPackageId != nil {
		c.Sui.LeafsiiPackageId = initConfig.LeafsiiPackageId.String()
	}
	if initConfig.PoolId != nil {
		c.Sui.PoolId = initConfig.PoolId.String()
	}

	return nil
}

func (c *Config) validate() error {
	if c.Sui.RPCURL == "" {
		return fmt.Errorf("LFS_SUI_RPC_URL is required")
	}
	if c.Database.PostgresDSN == "" {
		return fmt.Errorf("LFS_POSTGRES_DSN is required")
	}
	switch c.Sui.Network {
	case "localnet", "testnet", "mainnet":
	default:
		return fmt.Errorf("invalid LFS_NETWORK %q (must be localnet, testnet, or mainnet)", c.Sui.Network)
	}

	// Validate initializer config is loaded
	if c.Sui.initConfig == nil {
		return fmt.Errorf("initializer config not loaded")
	}
	if c.Sui.initConfig.ProtocolId == nil {
		return fmt.Errorf("protocol_id missing from init.json")
	}
	if c.Sui.PoolId == "" {
		return fmt.Errorf("pool_id missing from init.json")
	}
	if c.Sui.LeafsiiPackageId == "" {
		return fmt.Errorf("leafsii_package_id missing from init.json")
	}
	if c.Sui.initConfig.FtokenPackageId == nil {
		return fmt.Errorf("ftoken_package_id missing from init.json")
	}
	if c.Sui.initConfig.XtokenPackageId == nil {
		return fmt.Errorf("xtoken_package_id missing from init.json")
	}
	if c.Sui.initConfig.AdminCapId == nil {
		return fmt.Errorf("admin_cap_id missing from init.json")
	}
	return nil
}

func (c *Config) IsDev() bool {
	return c.Env == "dev"
}

func (c *Config) IsProd() bool {
	return c.Env == "prod"
}

// applyNetworkDefaults normalizes network names and fills in sensible RPC/WS defaults.
func (c *Config) applyNetworkDefaults() {
	net := strings.ToLower(strings.TrimSpace(c.Sui.Network))
	rpc := strings.TrimSpace(c.Sui.RPCURL)
	ws := strings.TrimSpace(c.Sui.WSURL)

	if net == "" {
		net = "localnet"
	}

	// If the network is still the default but the RPC points at a public endpoint,
	// infer the network from the URL to avoid mismatches when .env omits LFS_NETWORK.
	if net == "localnet" {
		if inferred := inferNetworkFromRPC(rpc); inferred != "" {
			net = inferred
		}
	}

	switch net {
	case "testnet":
		if rpc == "" || isLocalEndpoint(rpc) || rpc == "http://localhost:9000" {
			rpc = "https://fullnode.testnet.sui.io"
		}
		if ws == "" || isLocalEndpoint(ws) || ws == "wss://localhost:9000" {
			ws = "wss://fullnode.testnet.sui.io"
		}
	case "mainnet":
		if rpc == "" || isLocalEndpoint(rpc) || rpc == "http://localhost:9000" {
			rpc = "https://fullnode.mainnet.sui.io"
		}
		if ws == "" || isLocalEndpoint(ws) || ws == "wss://localhost:9000" {
			ws = "wss://fullnode.mainnet.sui.io"
		}
	default:
		net = "localnet"
		if rpc == "" {
			rpc = "http://localhost:9000"
		}
		if ws == "" {
			ws = "wss://localhost:9000"
		}
	}

	c.Sui.Network = net
	c.Sui.RPCURL = rpc
	c.Sui.WSURL = ws
}

func inferNetworkFromRPC(endpoint string) string {
	ep := strings.ToLower(endpoint)
	switch {
	case strings.Contains(ep, "testnet"):
		return "testnet"
	case strings.Contains(ep, "mainnet"):
		return "mainnet"
	default:
		return ""
	}
}

func isLocalEndpoint(endpoint string) bool {
	ep := strings.ToLower(endpoint)
	return strings.Contains(ep, "localhost") || strings.Contains(ep, "127.0.0.1") || strings.Contains(ep, "0.0.0.0")
}

// Helper methods to get addresses from initializer config
func (s *SuiConfig) GetPackageId() (*sui.PackageId, error) {
	if s.LeafsiiPackageId == "" {
		return nil, fmt.Errorf("package_id not available")
	}
	return sui.PackageIdFromHex(s.LeafsiiPackageId)
}
func (s *SuiConfig) GetProtocolId() (*sui.ObjectId, error) {
	if s.initConfig == nil || s.initConfig.ProtocolId == nil {
		return nil, fmt.Errorf("protocol_id not available")
	}
	return sui.ObjectIdFromHex(s.initConfig.ProtocolId.String())
}

func (s *SuiConfig) GetPoolId() (*sui.ObjectId, error) {
	if s.initConfig == nil || s.initConfig.PoolId == nil {
		return nil, fmt.Errorf("pool_id not available")
	}
	return sui.ObjectIdFromHex(s.initConfig.PoolId.String())
}

func (s *SuiConfig) GetFtokenPackageId() (*sui.PackageId, error) {
	if s.initConfig == nil || s.initConfig.FtokenPackageId == nil {
		return nil, fmt.Errorf("ftoken_package_id not available")
	}
	return sui.PackageIdFromHex(s.initConfig.FtokenPackageId.String())
}

func (s *SuiConfig) GetXtokenPackageId() (*sui.PackageId, error) {
	if s.initConfig == nil || s.initConfig.XtokenPackageId == nil {
		return nil, fmt.Errorf("xtoken_package_id not available")
	}
	return sui.PackageIdFromHex(s.initConfig.XtokenPackageId.String())
}

func (s *SuiConfig) GetLeafsiiPackageId() (*sui.PackageId, error) {
	if s.initConfig == nil || s.initConfig.LeafsiiPackageId == nil {
		return nil, fmt.Errorf("leafsii_package_id not available")
	}
	return sui.PackageIdFromHex(s.initConfig.LeafsiiPackageId.String())
}

func (s *SuiConfig) GetAdminCapId() (*sui.ObjectId, error) {
	if s.initConfig == nil || s.initConfig.AdminCapId == nil {
		return nil, fmt.Errorf("admin_cap_id not available")
	}
	return sui.ObjectIdFromHex(s.initConfig.AdminCapId.String())
}
