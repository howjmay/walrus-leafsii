package interfaces

import "context"

// Database represents the main database interface
type Database interface {
	// Connect establishes a connection to the database
	Connect(ctx context.Context) error
	
	// Disconnect closes the database connection
	Disconnect(ctx context.Context) error
	
	// IsHealthy checks if the database connection is healthy
	IsHealthy(ctx context.Context) bool
	
	// Transaction executes a function within a database transaction
	Transaction(ctx context.Context, fn func(ctx context.Context, tx Transaction) error) error
	
	// Repository returns a repository for the given schema
	Repository(schema *Schema) Repository
	
	// Migrate creates tables and applies schema changes
	Migrate(ctx context.Context, schemas []*Schema) error
	
	// Seed inserts initial data into the database
	Seed(ctx context.Context, schema *Schema, data []map[string]interface{}) error
}