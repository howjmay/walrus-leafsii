package db

import (
	"time"

	"github.com/leafsii/leafsii-backend/internal/db/entities"
	"github.com/leafsii/leafsii-backend/internal/db/interfaces"
)

// UserFixtures provides sample user data for seeding
var UserFixtures = []map[string]interface{}{
	{
		"email":     "john.doe@example.com",
		"name":      "John Doe",
		"age":       30,
		"is_active": true,
	},
	{
		"email":     "jane.smith@example.com",
		"name":      "Jane Smith",
		"age":       25,
		"is_active": true,
	},
	{
		"email":     "bob.johnson@example.com",
		"name":      "Bob Johnson",
		"age":       35,
		"is_active": false,
	},
	{
		"email":     "alice.brown@example.com",
		"name":      "Alice Brown",
		"is_active": true,
		// age is omitted (null)
	},
}

// PostFixtures provides sample post data for seeding
// Note: author_id fields will need to be populated with actual user IDs after users are created
func PostFixtures(authorIDs []string) []map[string]interface{} {
	if len(authorIDs) == 0 {
		return []map[string]interface{}{}
	}

	now := time.Now()
	fixtures := []map[string]interface{}{
		{
			"title":        "Introduction to Go",
			"content":      "Go is a programming language developed at Google...",
			"author_id":    authorIDs[0],
			"published_at": now.Add(-24 * time.Hour),
		},
		{
			"title":   "Database Design Patterns",
			"content": "When designing databases, there are several patterns...",
			"author_id": func() string {
				if len(authorIDs) > 1 {
					return authorIDs[1]
				}
				return authorIDs[0]
			}(),
			"published_at": now.Add(-12 * time.Hour),
		},
		{
			"title":     "Advanced Go Techniques",
			"content":   "This post covers advanced Go programming techniques...",
			"author_id": authorIDs[0],
			// published_at is omitted (draft post)
		},
	}

	return fixtures
}

// AllSchemas returns all entity schemas for migration
func AllSchemas() []*interfaces.Schema {
	return []*interfaces.Schema{
		entities.UserSchema,
		entities.PostSchema,
	}
}
