package memory

import (
	"context"
	"sync"
)

// Transaction represents an in-memory transaction
type Transaction struct {
	mu        sync.RWMutex
	db        *Database
	snapshot  map[string]map[string]map[string]interface{} // table -> id -> record
	committed bool
	rolledBack bool
}

// NewTransaction creates a new in-memory transaction
func NewTransaction(db *Database) *Transaction {
	tx := &Transaction{
		db:       db,
		snapshot: make(map[string]map[string]map[string]interface{}),
	}
	
	// Create snapshot of current state
	db.mu.RLock()
	for tableName, table := range db.tables {
		tx.snapshot[tableName] = make(map[string]map[string]interface{})
		for id, record := range table {
			// Deep copy the record
			recordCopy := make(map[string]interface{})
			for k, v := range record {
				recordCopy[k] = v
			}
			tx.snapshot[tableName][id] = recordCopy
		}
	}
	db.mu.RUnlock()
	
	return tx
}

// Commit commits the transaction
func (tx *Transaction) Commit(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	
	if tx.committed || tx.rolledBack {
		return ErrTransactionCompleted
	}
	
	tx.committed = true
	return nil
}

// Rollback rolls back the transaction
func (tx *Transaction) Rollback(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	
	if tx.committed || tx.rolledBack {
		return ErrTransactionCompleted
	}
	
	// Restore snapshot
	tx.db.mu.Lock()
	tx.db.tables = tx.snapshot
	tx.db.mu.Unlock()
	
	tx.rolledBack = true
	return nil
}

// IsCompleted returns true if the transaction has been committed or rolled back
func (tx *Transaction) IsCompleted() bool {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	
	return tx.committed || tx.rolledBack
}