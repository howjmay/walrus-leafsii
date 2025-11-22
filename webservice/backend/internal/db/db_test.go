package db

import (
	"context"
	"testing"

	"github.com/leafsii/leafsii-backend/internal/db/entities"
	"github.com/leafsii/leafsii-backend/internal/db/interfaces"
)

func TestInMemoryDatabase(t *testing.T) {
	ctx := context.Background()
	
	// Create database
	db := NewInMemoryDatabase()
	
	// Connect and migrate
	if err := ConnectAndMigrate(ctx, db, AllSchemas()); err != nil {
		t.Fatalf("Failed to connect and migrate: %v", err)
	}
	defer db.Disconnect(ctx)
	
	// Test health check
	if !db.IsHealthy(ctx) {
		t.Fatal("Database should be healthy")
	}
	
	// Get repositories
	userRepo := db.Repository(entities.UserSchema)
	postRepo := db.Repository(entities.PostSchema)
	
	t.Run("CRUD Operations", func(t *testing.T) {
		testCRUDOperations(t, ctx, userRepo)
	})
	
	t.Run("Query Operations", func(t *testing.T) {
		testQueryOperations(t, ctx, userRepo)
	})
	
	t.Run("Constraint Validation", func(t *testing.T) {
		testConstraintValidation(t, ctx, userRepo, postRepo)
	})
	
	t.Run("Transactions", func(t *testing.T) {
		testTransactions(t, ctx, db, userRepo)
	})
}

func testCRUDOperations(t *testing.T, ctx context.Context, repo interfaces.Repository) {
	// Create
	userData := map[string]interface{}{
		"email":     "test@example.com",
		"name":      "Test User",
		"age":       30,
		"is_active": true,
	}
	
	user, err := repo.Create(ctx, userData)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	
	if user["email"] != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%v'", user["email"])
	}
	
	userID := user["id"].(string)
	if userID == "" {
		t.Fatal("User ID should not be empty")
	}
	
	// Read
	retrieved, err := repo.GetByID(ctx, interfaces.StringID(userID))
	if err != nil {
		t.Fatalf("Failed to get user by ID: %v", err)
	}
	
	if retrieved["email"] != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%v'", retrieved["email"])
	}
	
	// Update
	updated, err := repo.Update(ctx, interfaces.StringID(userID), map[string]interface{}{
		"name": "Updated User",
		"age":  35,
	})
	if err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}
	
	if updated["name"] != "Updated User" {
		t.Errorf("Expected name 'Updated User', got '%v'", updated["name"])
	}
	if updated["age"] != 35 {
		t.Errorf("Expected age 35, got '%v'", updated["age"])
	}
	
	// Delete
	if err := repo.Delete(ctx, interfaces.StringID(userID)); err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}
	
	// Verify deletion
	_, err = repo.GetByID(ctx, interfaces.StringID(userID))
	if err != interfaces.ErrNotFound {
		t.Errorf("Expected ErrNotFound after deletion, got: %v", err)
	}
}

func testQueryOperations(t *testing.T, ctx context.Context, repo interfaces.Repository) {
	// Create test data
	users := []map[string]interface{}{
		{"email": "alice@example.com", "name": "Alice", "age": 25, "is_active": true},
		{"email": "bob@example.com", "name": "Bob", "age": 30, "is_active": false},
		{"email": "charlie@example.com", "name": "Charlie", "age": 35, "is_active": true},
	}
	
	for _, userData := range users {
		if _, err := repo.Create(ctx, userData); err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}
	}
	
	// Test filtering
	result, err := repo.FindMany(ctx, &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: []interfaces.Filter{
				{Field: "is_active", Value: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to find active users: %v", err)
	}
	
	if result.Total != 2 {
		t.Errorf("Expected 2 active users, got %d", result.Total)
	}
	
	// Test sorting
	result, err = repo.FindMany(ctx, &interfaces.Query{
		OrderBy: []interfaces.OrderBy{
			{Field: "age", Direction: "desc"},
		},
	})
	if err != nil {
		t.Fatalf("Failed to sort users: %v", err)
	}
	
	if len(result.Data) != 3 {
		t.Errorf("Expected 3 users, got %d", len(result.Data))
	}
	
	// Check sorting order
	if result.Data[0]["age"] != 35 {
		t.Errorf("Expected first user age 35, got %v", result.Data[0]["age"])
	}
	
	// Test pagination
	limit := 2
	result, err = repo.FindMany(ctx, &interfaces.Query{
		Limit: &limit,
		OrderBy: []interfaces.OrderBy{
			{Field: "name", Direction: "asc"},
		},
	})
	if err != nil {
		t.Fatalf("Failed to paginate users: %v", err)
	}
	
	if len(result.Data) != 2 {
		t.Errorf("Expected 2 users per page, got %d", len(result.Data))
	}
	if result.Total != 3 {
		t.Errorf("Expected total 3 users, got %d", result.Total)
	}
	
	// Test count
	count, err := repo.Count(ctx, &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: []interfaces.Filter{
				{Field: "is_active", Value: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to count active users: %v", err)
	}
	
	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

func testConstraintValidation(t *testing.T, ctx context.Context, userRepo, postRepo interfaces.Repository) {
	// Create a user first
	user, err := userRepo.Create(ctx, map[string]interface{}{
		"email":     "constraint@example.com",
		"name":      "Constraint User",
		"is_active": true,
	})
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	
	userID := user["id"].(string)
	
	// Test unique constraint violation
	_, err = userRepo.Create(ctx, map[string]interface{}{
		"email":     "constraint@example.com", // Duplicate email
		"name":      "Another User",
		"is_active": true,
	})
	if err == nil {
		t.Error("Expected unique constraint error for duplicate email")
	}
	
	// Test foreign key constraint - valid reference
	_, err = postRepo.Create(ctx, map[string]interface{}{
		"title":     "Test Post",
		"content":   "Test content",
		"author_id": userID,
	})
	if err != nil {
		t.Fatalf("Failed to create post with valid foreign key: %v", err)
	}
	
	// Test foreign key constraint - invalid reference
	_, err = postRepo.Create(ctx, map[string]interface{}{
		"title":     "Invalid Post",
		"content":   "Test content",
		"author_id": "non-existent-id",
	})
	if err == nil {
		t.Error("Expected foreign key constraint error for invalid author_id")
	}
}

func testTransactions(t *testing.T, ctx context.Context, db interfaces.Database, repo interfaces.Repository) {
	// Test successful transaction
	err := db.Transaction(ctx, func(ctx context.Context, tx interfaces.Transaction) error {
		_, err := repo.Create(ctx, map[string]interface{}{
			"email":     "tx@example.com",
			"name":      "TX User",
			"is_active": true,
		})
		return err
	})
	if err != nil {
		t.Fatalf("Transaction should succeed: %v", err)
	}
	
	// Verify user was created
	result, err := repo.FindMany(ctx, &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: []interfaces.Filter{
				{Field: "email", Value: "tx@example.com"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to find transaction user: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Expected 1 user from successful transaction, got %d", result.Total)
	}
	
	// Test failed transaction (should rollback)
	err = db.Transaction(ctx, func(ctx context.Context, tx interfaces.Transaction) error {
		_, err := repo.Create(ctx, map[string]interface{}{
			"email":     "rollback@example.com",
			"name":      "Rollback User",
			"is_active": true,
		})
		if err != nil {
			return err
		}
		
		// Force an error to trigger rollback
		return interfaces.ErrInvalidQuery
	})
	if err == nil {
		t.Error("Transaction should fail")
	}
	
	// Verify user was not created due to rollback
	result, err = repo.FindMany(ctx, &interfaces.Query{
		Where: &interfaces.Filters{
			Conditions: []interfaces.Filter{
				{Field: "email", Value: "rollback@example.com"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to search for rollback user: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Expected 0 users after rollback, got %d", result.Total)
	}
}