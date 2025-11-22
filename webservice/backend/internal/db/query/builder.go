package query

import (
	"fmt"
	"strings"
	"time"
	
	"github.com/leafsii/leafsii-backend/internal/db/interfaces"
)

// Builder helps construct database queries
type Builder struct {
	schema *interfaces.Schema
}

// NewBuilder creates a new query builder for a schema
func NewBuilder(schema *interfaces.Schema) *Builder {
	return &Builder{schema: schema}
}

// MatchesFilters checks if a record matches the given filters
func (b *Builder) MatchesFilters(record map[string]interface{}, filters *interfaces.Filters) bool {
	if filters == nil {
		return true
	}
	
	// Check AND conditions
	for _, andFilter := range filters.AND {
		if !b.MatchesFilters(record, andFilter) {
			return false
		}
	}
	
	// Check OR conditions
	if len(filters.OR) > 0 {
		hasMatch := false
		for _, orFilter := range filters.OR {
			if b.MatchesFilters(record, orFilter) {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			return false
		}
	}
	
	// Check individual conditions
	for _, condition := range filters.Conditions {
		if !b.matchesCondition(record, condition) {
			return false
		}
	}
	
	return true
}

func (b *Builder) matchesCondition(record map[string]interface{}, condition interfaces.Filter) bool {
	fieldValue, exists := record[condition.Field]
	
	// Handle simple equality
	if condition.Operator == nil {
		if !exists && condition.Value == nil {
			return true
		}
		return fieldValue == condition.Value
	}
	
	op := condition.Operator
	
	// Null checks
	if op.IsNull {
		return fieldValue == nil || !exists
	}
	if op.IsNotNull {
		return fieldValue != nil && exists
	}
	
	// If field doesn't exist and we're not checking for null, no match
	if !exists {
		return false
	}
	
	// Equality checks
	if op.Eq != nil {
		return fieldValue == op.Eq
	}
	if op.Ne != nil {
		return fieldValue != op.Ne
	}
	
	// Comparison checks (only for comparable types)
	if op.Gt != nil {
		return b.compare(fieldValue, op.Gt) > 0
	}
	if op.Gte != nil {
		return b.compare(fieldValue, op.Gte) >= 0
	}
	if op.Lt != nil {
		return b.compare(fieldValue, op.Lt) < 0
	}
	if op.Lte != nil {
		return b.compare(fieldValue, op.Lte) <= 0
	}
	
	// Array membership
	if len(op.In) > 0 {
		for _, val := range op.In {
			if fieldValue == val {
				return true
			}
		}
		return false
	}
	if len(op.NotIn) > 0 {
		for _, val := range op.NotIn {
			if fieldValue == val {
				return false
			}
		}
		return true
	}
	
	// String pattern matching
	if op.Like != "" {
		strValue, ok := fieldValue.(string)
		if !ok {
			return false
		}
		pattern := strings.ReplaceAll(op.Like, "%", "")
		caseSensitive := op.CaseSensitive == nil || *op.CaseSensitive
		if !caseSensitive {
			strValue = strings.ToLower(strValue)
			pattern = strings.ToLower(pattern)
		}
		return strings.Contains(strValue, pattern)
	}
	if op.NotLike != "" {
		strValue, ok := fieldValue.(string)
		if !ok {
			return true
		}
		pattern := strings.ReplaceAll(op.NotLike, "%", "")
		caseSensitive := op.CaseSensitive == nil || *op.CaseSensitive
		if !caseSensitive {
			strValue = strings.ToLower(strValue)
			pattern = strings.ToLower(pattern)
		}
		return !strings.Contains(strValue, pattern)
	}
	
	return true
}

func (b *Builder) compare(a, other interface{}) int {
	switch av := a.(type) {
	case int:
		if bv, ok := other.(int); ok {
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	case int64:
		if bv, ok := other.(int64); ok {
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	case float64:
		if bv, ok := other.(float64); ok {
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	case string:
		if bv, ok := other.(string); ok {
			return strings.Compare(av, bv)
		}
	}
	return 0
}

// ApplySort sorts records according to the OrderBy specification
func (b *Builder) ApplySort(records []map[string]interface{}, orderBy []interfaces.OrderBy) []map[string]interface{} {
	if len(orderBy) == 0 {
		return records
	}
	
	// Create a copy to avoid modifying the original slice
	sorted := make([]map[string]interface{}, len(records))
	copy(sorted, records)
	
	// Simple bubble sort for demonstration (replace with more efficient sorting if needed)
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if b.shouldSwap(sorted[j], sorted[j+1], orderBy) {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}
	
	return sorted
}

func (b *Builder) shouldSwap(a, other map[string]interface{}, orderBy []interfaces.OrderBy) bool {
	for _, order := range orderBy {
		aVal := a[order.Field]
		bVal := other[order.Field]
		
		cmp := b.compare(aVal, bVal)
		if cmp == 0 {
			continue // Equal, check next field
		}
		
		if order.Direction == "desc" {
			return cmp < 0 // Descending: swap if a < b
		} else {
			return cmp > 0 // Ascending: swap if a > b
		}
	}
	
	// If all fields are equal, maintain stable sort by comparing primary key
	return false
}

// ApplyPagination applies limit and offset to the records
func (b *Builder) ApplyPagination(records []map[string]interface{}, limit, offset *int) []map[string]interface{} {
	start := 0
	if offset != nil {
		start = *offset
	}
	
	if start >= len(records) {
		return []map[string]interface{}{}
	}
	
	end := len(records)
	if limit != nil {
		end = start + *limit
		if end > len(records) {
			end = len(records)
		}
	}
	
	return records[start:end]
}

// ValidateData validates data against the schema
func (b *Builder) ValidateData(data map[string]interface{}) error {
	for fieldName, fieldSchema := range b.schema.Fields {
		value, exists := data[fieldName]
		
		// Skip system fields that are auto-generated
		if fieldName == "id" || fieldName == "created_at" || fieldName == "updated_at" {
			continue
		}
		
		// Check required fields
		if !fieldSchema.Nullable && !exists && fieldSchema.DefaultValue == nil {
			return fmt.Errorf("field '%s' is required", fieldName)
		}
		
		// Skip validation if field is not present
		if !exists {
			continue
		}
		
		// Check null values
		if value == nil && !fieldSchema.Nullable {
			return fmt.Errorf("field '%s' cannot be null", fieldName)
		}
		
		// Type validation
		if value != nil {
			if err := b.validateFieldType(fieldName, value, fieldSchema.Type); err != nil {
				return err
			}
		}
	}
	
	return nil
}

func (b *Builder) validateFieldType(fieldName string, value interface{}, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field '%s' must be a string", fieldName)
		}
	case "int":
		if _, ok := value.(int); !ok {
			return fmt.Errorf("field '%s' must be an integer", fieldName)
		}
	case "int64":
		if _, ok := value.(int64); !ok {
			return fmt.Errorf("field '%s' must be an int64", fieldName)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field '%s' must be a boolean", fieldName)
		}
	case "float64":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("field '%s' must be a float64", fieldName)
		}
	case "time":
		// Accept both time.Time and string representations
		switch value.(type) {
		case string:
			// String time representation is valid
		case time.Time:
			// time.Time is valid
		default:
			return fmt.Errorf("field '%s' must be a time value", fieldName)
		}
	}
	
	return nil
}