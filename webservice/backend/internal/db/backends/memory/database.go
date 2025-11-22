package memory

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/leafsii/leafsii-backend/internal/db/interfaces"
)

var (
	ErrTransactionCompleted = fmt.Errorf("transaction already completed")
)

// Database implements the Database interface for in-memory storage
type Database struct {
	mu      sync.RWMutex
	tables  map[string]map[string]map[string]interface{} // tableName -> recordID -> record
	schemas map[string]*interfaces.Schema                 // tableName -> schema
	connected bool
}

// NewDatabase creates a new in-memory database
func NewDatabase() *Database {
	return &Database{
		tables:  make(map[string]map[string]map[string]interface{}),
		schemas: make(map[string]*interfaces.Schema),
	}
}

// Connect establishes a connection to the database
func (db *Database) Connect(ctx context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	
	db.connected = true
	log.Println("Connected to in-memory database")
	return nil
}

// Disconnect closes the database connection
func (db *Database) Disconnect(ctx context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	
	db.connected = false
	db.tables = make(map[string]map[string]map[string]interface{})
	db.schemas = make(map[string]*interfaces.Schema)
	log.Println("Disconnected from in-memory database")
	return nil
}

// IsHealthy checks if the database connection is healthy
func (db *Database) IsHealthy(ctx context.Context) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	return db.connected
}

// Transaction executes a function within a database transaction
func (db *Database) Transaction(ctx context.Context, fn func(ctx context.Context, tx interfaces.Transaction) error) error {
	if !db.connected {
		return interfaces.ErrDatabaseNotConnected
	}
	
	tx := NewTransaction(db)
	
	defer func() {
		if !tx.IsCompleted() {
			tx.Rollback(ctx)
		}
	}()
	
	if err := fn(ctx, tx); err != nil {
		tx.Rollback(ctx)
		return err
	}
	
	return tx.Commit(ctx)
}

// Repository returns a repository for the given schema
func (db *Database) Repository(schema *interfaces.Schema) interfaces.Repository {
	db.mu.Lock()
	db.schemas[schema.TableName] = schema
	db.mu.Unlock()
	
	return NewRepository(db, schema)
}

// Migrate creates tables and applies schema changes
func (db *Database) Migrate(ctx context.Context, schemas []*interfaces.Schema) error {
	if !db.connected {
		return interfaces.ErrDatabaseNotConnected
	}
	
	db.mu.Lock()
	defer db.mu.Unlock()
	
	for _, schema := range schemas {
		db.schemas[schema.TableName] = schema
		
		// Create table if it doesn't exist
		if _, exists := db.tables[schema.TableName]; !exists {
			db.tables[schema.TableName] = make(map[string]map[string]interface{})
			log.Printf("Created in-memory table: %s", schema.TableName)
		}
	}
	
	log.Printf("Migration completed for %d schemas", len(schemas))
	return nil
}

// Seed inserts initial data into the database
func (db *Database) Seed(ctx context.Context, schema *interfaces.Schema, data []map[string]interface{}) error {
	if !db.connected {
		return interfaces.ErrDatabaseNotConnected
	}
	
	repo := db.Repository(schema)
	
	for i, record := range data {
		if _, err := repo.Create(ctx, record); err != nil {
			log.Printf("Failed to seed record %d in table %s: %v", i, schema.TableName, err)
			// Continue with other records rather than failing completely
		}
	}
	
	log.Printf("Seeded %d records into table %s", len(data), schema.TableName)
	return nil
}

// GetTables returns all table names (for debugging/testing)
func (db *Database) GetTables() []string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	tables := make([]string, 0, len(db.tables))
	for name := range db.tables {
		tables = append(tables, name)
	}
	return tables
}

// GetTableData returns all data for a specific table (for debugging/testing)
func (db *Database) GetTableData(tableName string) map[string]map[string]interface{} {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	table, exists := db.tables[tableName]
	if !exists {
		return nil
	}
	
	// Return a deep copy to prevent external modifications
	result := make(map[string]map[string]interface{})
	for id, record := range table {
		recordCopy := make(map[string]interface{})
		for k, v := range record {
			recordCopy[k] = v
		}
		result[id] = recordCopy
	}
	
	return result
}

// Clear removes all data from all tables (for testing)
func (db *Database) Clear() {
	db.mu.Lock()
	defer db.mu.Unlock()
	
	for tableName := range db.tables {
		db.tables[tableName] = make(map[string]map[string]interface{})
	}
}