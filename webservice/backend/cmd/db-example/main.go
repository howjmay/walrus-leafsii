package main

import (
	"context"
	"fmt"
	"log"

	"github.com/leafsii/leafsii-backend/internal/db"
	"github.com/leafsii/leafsii-backend/internal/db/entities"
	"github.com/leafsii/leafsii-backend/internal/db/interfaces"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Database Abstraction Layer Demo ===")

	// Create in-memory database
	database := db.NewInMemoryDatabase()

	// Connect and migrate
	if err := db.ConnectAndMigrate(ctx, database, db.AllSchemas()); err != nil {
		log.Fatalf("Failed to setup database: %v", err)
	}
	defer database.Disconnect(ctx)

	// Get repositories
	userRepo := database.Repository(entities.UserSchema)
	postRepo := database.Repository(entities.PostSchema)

	fmt.Println("--- Basic CRUD Operations ---")

	// Create users
	fmt.Println("Creating users...")
	var userIDs []string
	for _, userData := range db.UserFixtures {
		user, err := userRepo.Create(ctx, userData)
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			continue
		}
		userIDs = append(userIDs, user["id"].(string))
		fmt.Printf("Created user: %s (%s)\n", user["name"], user["email"])
	}

	// Create posts
	fmt.Println("\nCreating posts...")
	for _, postData := range db.PostFixtures(userIDs) {
		post, err := postRepo.Create(ctx, postData)
		if err != nil {
			log.Printf("Failed to create post: %v", err)
			continue
		}
		fmt.Printf("Created post: %s\n", post["title"])
	}

	fmt.Println("\n--- Query Operations ---")

	// Find active users
	activeUsers, err := userRepo.FindMany(ctx, &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: []interfaces.Filter{
				{Field: "is_active", Value: true},
			},
		},
		OrderBy: []interfaces.OrderBy{
			{Field: "name", Direction: "asc"},
		},
	})
	if err != nil {
		log.Fatalf("Failed to find active users: %v", err)
	}

	fmt.Printf("Found %d active users:\n", activeUsers.Total)
	for _, user := range activeUsers.Data {
		fmt.Printf("  - %s (%s)\n", user["name"], user["email"])
	}

	// Find users with age filter
	adultUsers, err := userRepo.FindMany(ctx, &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: []interfaces.Filter{
				{
					Field: "age",
					Operator: &interfaces.FilterOperator{
						Gte: 25,
					},
				},
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to find adult users: %v", err)
	}

	fmt.Printf("\nFound %d users age 25+:\n", adultUsers.Total)
	for _, user := range adultUsers.Data {
		age := "unknown"
		if user["age"] != nil {
			age = fmt.Sprintf("%v", user["age"])
		}
		fmt.Printf("  - %s (age: %s)\n", user["name"], age)
	}

	// Search users by name pattern
	searchResults, err := userRepo.FindMany(ctx, &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: []interfaces.Filter{
				{
					Field: "name",
					Operator: &interfaces.FilterOperator{
						Like: "%o%",
					},
				},
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to search users: %v", err)
	}

	fmt.Printf("\nUsers with 'o' in name (%d found):\n", searchResults.Total)
	for _, user := range searchResults.Data {
		fmt.Printf("  - %s\n", user["name"])
	}

	fmt.Println("\n--- Pagination Example ---")

	limit := 2
	offset := 0
	page := 1

	for {
		pageResult, err := userRepo.FindMany(ctx, &interfaces.Query{
			Limit:  &limit,
			Offset: &offset,
			OrderBy: []interfaces.OrderBy{
				{Field: "name", Direction: "asc"},
			},
		})
		if err != nil {
			log.Fatalf("Failed to get page: %v", err)
		}

		if len(pageResult.Data) == 0 {
			break
		}

		fmt.Printf("Page %d (total: %d):\n", page, pageResult.Total)
		for _, user := range pageResult.Data {
			fmt.Printf("  - %s\n", user["name"])
		}

		offset += limit
		page++

		if offset >= int(pageResult.Total) {
			break
		}
	}

	fmt.Println("\n--- Transaction Example ---")

	// Successful transaction
	err = database.Transaction(ctx, func(ctx context.Context, tx interfaces.Transaction) error {
		// Create user and post in same transaction
		user, err := userRepo.Create(ctx, map[string]interface{}{
			"email":     "transaction@example.com",
			"name":      "Transaction User",
			"is_active": true,
		})
		if err != nil {
			return err
		}

		_, err = postRepo.Create(ctx, map[string]interface{}{
			"title":     "Transaction Post",
			"content":   "This post was created in a transaction",
			"author_id": user["id"],
		})
		if err != nil {
			return err
		}

		fmt.Println("Transaction completed successfully")
		return nil
	})
	if err != nil {
		log.Printf("Transaction failed: %v", err)
	}

	// Failed transaction (should rollback)
	err = database.Transaction(ctx, func(ctx context.Context, tx interfaces.Transaction) error {
		_, err := userRepo.Create(ctx, map[string]interface{}{
			"email":     "rollback@example.com",
			"name":      "Rollback User",
			"is_active": true,
		})
		if err != nil {
			return err
		}

		// Force an error to demonstrate rollback
		return fmt.Errorf("simulated error")
	})
	if err != nil {
		fmt.Printf("Transaction failed as expected: %v\n", err)
	}

	// Verify rollback worked
	rollbackUser, err := userRepo.FindOne(ctx, &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: []interfaces.Filter{
				{Field: "email", Value: "rollback@example.com"},
			},
		},
	})
	if err == interfaces.ErrNotFound {
		fmt.Println("Rollback successful - user was not created")
	} else if err != nil {
		log.Printf("Error checking rollback: %v", err)
	} else {
		log.Printf("Rollback failed - user was created: %v", rollbackUser)
	}

	fmt.Println("\n--- Constraint Examples ---")

	// Unique constraint violation
	_, err = userRepo.Create(ctx, map[string]interface{}{
		"email":     "john.doe@example.com", // Duplicate email
		"name":      "Another John",
		"is_active": true,
	})
	if err != nil {
		fmt.Printf("Unique constraint error (expected): %v\n", err)
	}

	// Foreign key constraint violation
	_, err = postRepo.Create(ctx, map[string]interface{}{
		"title":     "Invalid Post",
		"content":   "This should fail",
		"author_id": "non-existent-user-id",
	})
	if err != nil {
		fmt.Printf("Foreign key constraint error (expected): %v\n", err)
	}

	fmt.Println("\n--- Final Statistics ---")

	userCount, _ := userRepo.Count(ctx, nil)
	postCount, _ := postRepo.Count(ctx, nil)
	publishedPostCount, _ := postRepo.Count(ctx, &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: []interfaces.Filter{
				{
					Field: "published_at",
					Operator: &interfaces.FilterOperator{
						IsNotNull: true,
					},
				},
			},
		},
	})

	fmt.Printf("Total users: %d\n", userCount)
	fmt.Printf("Total posts: %d\n", postCount)
	fmt.Printf("Published posts: %d\n", publishedPostCount)

	fmt.Println("\n=== Demo completed ===")
}
