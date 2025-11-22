package db

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/leafsii/leafsii-backend/internal/db/backends/memory"
	"github.com/leafsii/leafsii-backend/internal/db/interfaces"
)

// Config holds database configuration
type Config struct {
	Type         string // "memory", "postgres", "sqlite"
	DSN          string // Data Source Name / Connection String
	UseInMemory  bool   // Force in-memory usage
	MaxOpenConns int    // Maximum open connections (for SQL backends)
	MaxIdleConns int    // Maximum idle connections (for SQL backends)
}

// NewDatabase creates a new database instance based on configuration
func NewDatabase(config *Config) (interfaces.Database, error) {
	if config == nil {
		config = &Config{}
	}

	// Default configuration from environment
	if config.Type == "" {
		config.Type = getEnvOrDefault("DB_TYPE", "memory")
	}
	if config.DSN == "" {
		config.DSN = os.Getenv("DB_DSN")
	}
	if !config.UseInMemory && os.Getenv("USE_IN_MEMORY") == "true" {
		config.UseInMemory = true
	}

	// Force in-memory if no DSN provided or explicitly requested
	if config.UseInMemory || (config.DSN == "" && config.Type != "memory") {
		log.Println("Using in-memory database")
		return memory.NewDatabase(), nil
	}

	switch config.Type {
	case "memory":
		log.Println("Using in-memory database")
		return memory.NewDatabase(), nil
	case "postgres":
		// TODO: Implement PostgreSQL backend
		log.Println("PostgreSQL backend not yet implemented, falling back to in-memory")
		return memory.NewDatabase(), nil
	case "sqlite":
		// TODO: Implement SQLite backend
		log.Println("SQLite backend not yet implemented, falling back to in-memory")
		return memory.NewDatabase(), nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.Type)
	}
}

// MustNewDatabase creates a new database instance and panics on error
func MustNewDatabase(config *Config) interfaces.Database {
	db, err := NewDatabase(config)
	if err != nil {
		panic(fmt.Sprintf("failed to create database: %v", err))
	}
	return db
}

// NewInMemoryDatabase creates a new in-memory database instance
func NewInMemoryDatabase() interfaces.Database {
	return memory.NewDatabase()
}

// ConnectAndMigrate connects to the database and runs migrations
func ConnectAndMigrate(ctx context.Context, db interfaces.Database, schemas []*interfaces.Schema) error {
	if err := db.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	if !db.IsHealthy(ctx) {
		return fmt.Errorf("database health check failed")
	}

	if err := db.Migrate(ctx, schemas); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
