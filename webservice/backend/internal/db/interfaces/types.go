package interfaces

import (
	"errors"
	"time"
)

// ID represents a unique identifier that can be either string or int64
type ID interface {
	String() string
}

// StringID implements ID for string identifiers
type StringID string

func (s StringID) String() string {
	return string(s)
}

// IntID implements ID for integer identifiers
type IntID int64

func (i IntID) String() string {
	return string(rune(i))
}

// Entity represents a database entity with common fields
type Entity struct {
	ID        ID        `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FilterOperator represents different filter operations
type FilterOperator struct {
	Eq       interface{}   `json:"eq,omitempty"`
	Ne       interface{}   `json:"ne,omitempty"`
	Gt       interface{}   `json:"gt,omitempty"`
	Gte      interface{}   `json:"gte,omitempty"`
	Lt       interface{}   `json:"lt,omitempty"`
	Lte      interface{}   `json:"lte,omitempty"`
	In       []interface{} `json:"in,omitempty"`
	NotIn    []interface{} `json:"not_in,omitempty"`
	Like     string        `json:"like,omitempty"`
	NotLike  string        `json:"not_like,omitempty"`
	IsNull   bool          `json:"is_null,omitempty"`
	IsNotNull bool         `json:"is_not_null,omitempty"`
	CaseSensitive *bool    `json:"case_sensitive,omitempty"`
}

// Filter represents a field filter
type Filter struct {
	Field    string          `json:"field"`
	Value    interface{}     `json:"value,omitempty"`
	Operator *FilterOperator `json:"operator,omitempty"`
}

// Filters represents complex filtering with AND/OR logic
type Filters struct {
	Conditions []Filter   `json:"conditions,omitempty"`
	AND        []*Filters `json:"and,omitempty"`
	OR         []*Filters `json:"or,omitempty"`
}

// OrderBy represents sorting configuration
type OrderBy struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // "asc" or "desc"
}

// Query represents a database query with filtering, sorting, and pagination
type Query struct {
	Where   *Filters   `json:"where,omitempty"`
	Select  []string   `json:"select,omitempty"`
	OrderBy []OrderBy  `json:"order_by,omitempty"`
	Limit   *int       `json:"limit,omitempty"`
	Offset  *int       `json:"offset,omitempty"`
	Include []string   `json:"include,omitempty"`
}

// ResultPage represents paginated query results
type ResultPage struct {
	Data     []map[string]interface{} `json:"data"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

// Schema represents entity schema definition
type Schema struct {
	TableName string                 `json:"table_name"`
	Fields    map[string]FieldSchema `json:"fields"`
	Indexes   []Index               `json:"indexes,omitempty"`
}

// FieldSchema represents a field definition
type FieldSchema struct {
	Type         string      `json:"type"`         // "string", "int", "int64", "bool", "time", "float64"
	Nullable     bool        `json:"nullable"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	Unique       bool        `json:"unique"`
	PrimaryKey   bool        `json:"primary_key"`
	ForeignKey   *ForeignKey `json:"foreign_key,omitempty"`
}

// ForeignKey represents a foreign key constraint
type ForeignKey struct {
	Table    string `json:"table"`
	Column   string `json:"column"`
	OnDelete string `json:"on_delete,omitempty"` // CASCADE, SET_NULL, RESTRICT
}

// Index represents a database index
type Index struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

// Common database errors
var (
	ErrNotFound              = errors.New("record not found")
	ErrUniqueConstraint      = errors.New("unique constraint violation")
	ErrForeignKeyConstraint  = errors.New("foreign key constraint violation")
	ErrInvalidQuery          = errors.New("invalid query")
	ErrTransactionCompleted  = errors.New("transaction already completed")
	ErrDatabaseNotConnected  = errors.New("database not connected")
)

// DatabaseError wraps database-specific errors
type DatabaseError struct {
	Op  string
	Err error
}

func (e *DatabaseError) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e *DatabaseError) Unwrap() error {
	return e.Err
}