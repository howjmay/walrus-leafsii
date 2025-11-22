package interfaces

import "context"

// Transaction represents a database transaction
type Transaction interface {
	// Commit commits the transaction
	Commit(ctx context.Context) error
	
	// Rollback rolls back the transaction
	Rollback(ctx context.Context) error
	
	// IsCompleted returns true if the transaction has been committed or rolled back
	IsCompleted() bool
}