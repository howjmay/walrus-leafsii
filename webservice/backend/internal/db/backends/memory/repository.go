package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/leafsii/leafsii-backend/internal/db/interfaces"
	"github.com/leafsii/leafsii-backend/internal/db/query"
	"github.com/google/uuid"
)

// Repository implements the Repository interface for in-memory storage
type Repository struct {
	db        *Database
	schema    *interfaces.Schema
	builder   *query.Builder
	tableName string
}

// NewRepository creates a new in-memory repository
func NewRepository(db *Database, schema *interfaces.Schema) *Repository {
	return &Repository{
		db:        db,
		schema:    schema,
		builder:   query.NewBuilder(schema),
		tableName: schema.TableName,
	}
}

// GetByID retrieves a single record by its ID
func (r *Repository) GetByID(ctx context.Context, id interfaces.ID) (map[string]interface{}, error) {
	r.db.mu.RLock()
	defer r.db.mu.RUnlock()
	
	table, exists := r.db.tables[r.tableName]
	if !exists {
		return nil, interfaces.ErrNotFound
	}
	
	record, exists := table[id.String()]
	if !exists {
		return nil, interfaces.ErrNotFound
	}
	
	// Deep copy to avoid external modifications
	result := make(map[string]interface{})
	for k, v := range record {
		result[k] = v
	}
	
	return result, nil
}

// FindOne retrieves the first record matching the query
func (r *Repository) FindOne(ctx context.Context, q *interfaces.Query) (map[string]interface{}, error) {
	if q == nil {
		q = &interfaces.Query{}
	}
	
	// Set limit to 1 for efficiency
	limit := 1
	q.Limit = &limit
	
	result, err := r.FindMany(ctx, q)
	if err != nil {
		return nil, err
	}
	
	if len(result.Data) == 0 {
		return nil, interfaces.ErrNotFound
	}
	
	return result.Data[0], nil
}

// FindMany retrieves multiple records matching the query with pagination
func (r *Repository) FindMany(ctx context.Context, q *interfaces.Query) (*interfaces.ResultPage, error) {
	if q == nil {
		q = &interfaces.Query{}
	}
	
	r.db.mu.RLock()
	table, exists := r.db.tables[r.tableName]
	if !exists {
		r.db.mu.RUnlock()
		return &interfaces.ResultPage{
			Data:     []map[string]interface{}{},
			Total:    0,
			Page:     1,
			PageSize: 0,
		}, nil
	}
	
	// Convert to slice for processing
	var records []map[string]interface{}
	for _, record := range table {
		// Deep copy
		recordCopy := make(map[string]interface{})
		for k, v := range record {
			recordCopy[k] = v
		}
		records = append(records, recordCopy)
	}
	r.db.mu.RUnlock()
	
	// Apply filters
	if q.Where != nil {
		var filtered []map[string]interface{}
		for _, record := range records {
			if r.builder.MatchesFilters(record, q.Where) {
				filtered = append(filtered, record)
			}
		}
		records = filtered
	}
	
	total := int64(len(records))
	
	// Apply sorting
	if len(q.OrderBy) > 0 {
		records = r.builder.ApplySort(records, q.OrderBy)
	}
	
	// Apply pagination
	offset := 0
	if q.Offset != nil {
		offset = *q.Offset
	}
	pageSize := len(records)
	if q.Limit != nil {
		pageSize = *q.Limit
	}
	
	records = r.builder.ApplyPagination(records, q.Limit, q.Offset)
	
	// Apply field selection
	if len(q.Select) > 0 {
		var projected []map[string]interface{}
		for _, record := range records {
			projectedRecord := make(map[string]interface{})
			for _, field := range q.Select {
				if value, exists := record[field]; exists {
					projectedRecord[field] = value
				}
			}
			projected = append(projected, projectedRecord)
		}
		records = projected
	}
	
	page := 1
	if pageSize > 0 {
		page = (offset / pageSize) + 1
	}
	
	return &interfaces.ResultPage{
		Data:     records,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// Create inserts a new record
func (r *Repository) Create(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	// Validate data
	if err := r.builder.ValidateData(data); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}
	
	// Prepare record with defaults and timestamps
	record := make(map[string]interface{})
	for k, v := range data {
		record[k] = v
	}
	
	// Set ID if not provided
	if _, exists := record["id"]; !exists {
		record["id"] = uuid.New().String()
	}
	
	// Set timestamps
	now := time.Now()
	record["created_at"] = now
	record["updated_at"] = now
	
	// Apply default values
	for fieldName, fieldSchema := range r.schema.Fields {
		if _, exists := record[fieldName]; !exists && fieldSchema.DefaultValue != nil {
			record[fieldName] = fieldSchema.DefaultValue
		}
	}
	
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	
	// Ensure table exists
	if _, exists := r.db.tables[r.tableName]; !exists {
		r.db.tables[r.tableName] = make(map[string]map[string]interface{})
	}
	
	table := r.db.tables[r.tableName]
	id := record["id"].(string)
	
	// Check if ID already exists
	if _, exists := table[id]; exists {
		return nil, fmt.Errorf("record with id '%s' already exists", id)
	}
	
	// Validate unique constraints
	if err := r.validateUniqueConstraints(table, record, ""); err != nil {
		return nil, err
	}
	
	// Validate foreign key constraints
	if err := r.validateForeignKeyConstraints(record); err != nil {
		return nil, err
	}
	
	// Store record
	table[id] = record
	
	// Return copy
	result := make(map[string]interface{})
	for k, v := range record {
		result[k] = v
	}
	
	return result, nil
}

// Update modifies an existing record by ID
func (r *Repository) Update(ctx context.Context, id interfaces.ID, data map[string]interface{}) (map[string]interface{}, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	
	table, exists := r.db.tables[r.tableName]
	if !exists {
		return nil, interfaces.ErrNotFound
	}
	
	existing, exists := table[id.String()]
	if !exists {
		return nil, interfaces.ErrNotFound
	}
	
	// Create updated record
	updated := make(map[string]interface{})
	for k, v := range existing {
		updated[k] = v
	}
	for k, v := range data {
		updated[k] = v
	}
	updated["updated_at"] = time.Now()
	
	// Validate unique constraints (excluding this record)
	if err := r.validateUniqueConstraints(table, updated, id.String()); err != nil {
		return nil, err
	}
	
	// Validate foreign key constraints
	if err := r.validateForeignKeyConstraints(updated); err != nil {
		return nil, err
	}
	
	// Update record
	table[id.String()] = updated
	
	// Return copy
	result := make(map[string]interface{})
	for k, v := range updated {
		result[k] = v
	}
	
	return result, nil
}

// Upsert inserts or updates based on unique field constraints
func (r *Repository) Upsert(ctx context.Context, uniqueFields map[string]interface{}, data map[string]interface{}) (map[string]interface{}, error) {
	// Try to find existing record by unique fields
	q := &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: make([]interfaces.Filter, 0, len(uniqueFields)),
		},
	}
	
	for field, value := range uniqueFields {
		q.Where.Conditions = append(q.Where.Conditions, interfaces.Filter{
			Field: field,
			Value: value,
		})
	}
	
	existing, err := r.FindOne(ctx, q)
	if err != nil && err != interfaces.ErrNotFound {
		return nil, err
	}
	
	if existing != nil {
		// Update existing record
		id := existing["id"].(string)
		return r.Update(ctx, interfaces.StringID(id), data)
	}
	
	// Create new record
	createData := make(map[string]interface{})
	for k, v := range data {
		createData[k] = v
	}
	for k, v := range uniqueFields {
		createData[k] = v
	}
	
	return r.Create(ctx, createData)
}

// Delete removes a record by ID
func (r *Repository) Delete(ctx context.Context, id interfaces.ID) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	
	table, exists := r.db.tables[r.tableName]
	if !exists {
		return interfaces.ErrNotFound
	}
	
	if _, exists := table[id.String()]; !exists {
		return interfaces.ErrNotFound
	}
	
	// Check foreign key constraints from other tables
	if err := r.validateForeignKeyConstraintsOnDelete(id.String()); err != nil {
		return err
	}
	
	delete(table, id.String())
	return nil
}

// Count returns the number of records matching the query
func (r *Repository) Count(ctx context.Context, q *interfaces.Query) (int64, error) {
	if q == nil {
		r.db.mu.RLock()
		table, exists := r.db.tables[r.tableName]
		count := int64(0)
		if exists {
			count = int64(len(table))
		}
		r.db.mu.RUnlock()
		return count, nil
	}
	
	// Use FindMany but without pagination to get accurate count
	countQuery := &interfaces.Query{
		Where: q.Where,
	}
	
	result, err := r.FindMany(ctx, countQuery)
	if err != nil {
		return 0, err
	}
	
	return result.Total, nil
}

// GetSchema returns the schema for this repository
func (r *Repository) GetSchema() *interfaces.Schema {
	return r.schema
}

// Helper methods for constraint validation

func (r *Repository) validateUniqueConstraints(table map[string]map[string]interface{}, record map[string]interface{}, excludeID string) error {
	// Check unique fields
	for fieldName, fieldSchema := range r.schema.Fields {
		if !fieldSchema.Unique {
			continue
		}
		
		value, exists := record[fieldName]
		if !exists || value == nil {
			continue
		}
		
		// Check if any other record has the same value
		for id, existing := range table {
			if id == excludeID {
				continue
			}
			if existingValue, exists := existing[fieldName]; exists && existingValue == value {
				return fmt.Errorf("%w: field '%s' value '%v'", interfaces.ErrUniqueConstraint, fieldName, value)
			}
		}
	}
	
	// Check unique indexes
	for _, index := range r.schema.Indexes {
		if !index.Unique {
			continue
		}
		
		// Build composite key
		var keyParts []interface{}
		for _, column := range index.Columns {
			if value, exists := record[column]; exists {
				keyParts = append(keyParts, value)
			} else {
				keyParts = append(keyParts, nil)
			}
		}
		
		// Check if any other record has the same composite key
		for id, existing := range table {
			if id == excludeID {
				continue
			}
			
			var existingKeyParts []interface{}
			for _, column := range index.Columns {
				if value, exists := existing[column]; exists {
					existingKeyParts = append(existingKeyParts, value)
				} else {
					existingKeyParts = append(existingKeyParts, nil)
				}
			}
			
			// Compare composite keys
			if len(keyParts) == len(existingKeyParts) {
				match := true
				for i, part := range keyParts {
					if part != existingKeyParts[i] {
						match = false
						break
					}
				}
				if match {
					return fmt.Errorf("%w: unique index '%s'", interfaces.ErrUniqueConstraint, index.Name)
				}
			}
		}
	}
	
	return nil
}

func (r *Repository) validateForeignKeyConstraints(record map[string]interface{}) error {
	for fieldName, fieldSchema := range r.schema.Fields {
		if fieldSchema.ForeignKey == nil {
			continue
		}
		
		value, exists := record[fieldName]
		if !exists || value == nil {
			continue
		}
		
		// Check if referenced record exists
		refTable, exists := r.db.tables[fieldSchema.ForeignKey.Table]
		if !exists {
			return fmt.Errorf("%w: referenced table '%s' does not exist", interfaces.ErrForeignKeyConstraint, fieldSchema.ForeignKey.Table)
		}
		
		found := false
		for _, refRecord := range refTable {
			if refValue, exists := refRecord[fieldSchema.ForeignKey.Column]; exists && refValue == value {
				found = true
				break
			}
		}
		
		if !found {
			return fmt.Errorf("%w: field '%s' references non-existent record '%v'", interfaces.ErrForeignKeyConstraint, fieldName, value)
		}
	}
	
	return nil
}

func (r *Repository) validateForeignKeyConstraintsOnDelete(id string) error {
	// Check all tables for foreign key references to this record
	for tableName, table := range r.db.tables {
		if tableName == r.tableName {
			continue // Skip self
		}
		
		// Get schema for this table (this is a simplified approach)
		// In a real implementation, you'd want to track schemas per table
		for _, record := range table {
			for fieldName, value := range record {
				// Check if this field might be a foreign key to our table
				// This is simplified - in practice you'd want to track FK relationships
				if value == id {
					return fmt.Errorf("%w: record is referenced by table '%s', field '%s'", interfaces.ErrForeignKeyConstraint, tableName, fieldName)
				}
			}
		}
	}
	
	return nil
}