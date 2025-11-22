package entities

import (
	"time"
	
	"github.com/leafsii/leafsii-backend/internal/db/interfaces"
)

// Post represents a post entity
type Post struct {
	ID          string     `json:"id" db:"id"`
	Title       string     `json:"title" db:"title"`
	Content     string     `json:"content" db:"content"`
	AuthorID    string     `json:"author_id" db:"author_id"`
	PublishedAt *time.Time `json:"published_at,omitempty" db:"published_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// PostSchema defines the database schema for posts
var PostSchema = &interfaces.Schema{
	TableName: "posts",
	Fields: map[string]interfaces.FieldSchema{
		"id": {
			Type:       "string",
			PrimaryKey: true,
		},
		"title": {
			Type: "string",
		},
		"content": {
			Type: "string",
		},
		"author_id": {
			Type: "string",
			ForeignKey: &interfaces.ForeignKey{
				Table:    "users",
				Column:   "id",
				OnDelete: "CASCADE",
			},
		},
		"published_at": {
			Type:     "time",
			Nullable: true,
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
			Name:    "idx_posts_author",
			Columns: []string{"author_id"},
		},
		{
			Name:    "idx_posts_published",
			Columns: []string{"published_at"},
		},
	},
}