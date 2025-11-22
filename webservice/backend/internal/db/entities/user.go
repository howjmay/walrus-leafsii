package entities

import (
	"time"
	
	"github.com/leafsii/leafsii-backend/internal/db/interfaces"
)

// User represents a user entity
type User struct {
	ID        string     `json:"id" db:"id"`
	Email     string     `json:"email" db:"email"`
	Name      string     `json:"name" db:"name"`
	Age       *int       `json:"age,omitempty" db:"age"`
	IsActive  bool       `json:"is_active" db:"is_active"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}

// UserSchema defines the database schema for users
var UserSchema = &interfaces.Schema{
	TableName: "users",
	Fields: map[string]interfaces.FieldSchema{
		"id": {
			Type:       "string",
			PrimaryKey: true,
		},
		"email": {
			Type:   "string",
			Unique: true,
		},
		"name": {
			Type: "string",
		},
		"age": {
			Type:     "int",
			Nullable: true,
		},
		"is_active": {
			Type:         "bool",
			DefaultValue: true,
		},
		"created_at": {
			Type: "time",
		},
		"updated_at": {
			Type: "time",
		},
	},
	Indexes: []interfaces.Index{
		{
			Name:    "idx_users_email",
			Columns: []string{"email"},
			Unique:  true,
		},
		{
			Name:    "idx_users_active",
			Columns: []string{"is_active"},
		},
	},
}