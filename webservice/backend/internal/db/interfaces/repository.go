package interfaces

import "context"

// Repository provides CRUD operations for a specific entity type
type Repository interface {
	// GetByID retrieves a single record by its ID
	GetByID(ctx context.Context, id ID) (map[string]interface{}, error)
	
	// FindOne retrieves the first record matching the query
	FindOne(ctx context.Context, query *Query) (map[string]interface{}, error)
	
	// FindMany retrieves multiple records matching the query with pagination
	FindMany(ctx context.Context, query *Query) (*ResultPage, error)
	
	// Create inserts a new record
	Create(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)
	
	// Update modifies an existing record by ID
	Update(ctx context.Context, id ID, data map[string]interface{}) (map[string]interface{}, error)
	
	// Upsert inserts or updates based on unique field constraints
	Upsert(ctx context.Context, uniqueFields map[string]interface{}, data map[string]interface{}) (map[string]interface{}, error)
	
	// Delete removes a record by ID
	Delete(ctx context.Context, id ID) error
	
	// Count returns the number of records matching the query
	Count(ctx context.Context, query *Query) (int64, error)
	
	// GetSchema returns the schema for this repository
	GetSchema() *Schema
}