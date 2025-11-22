# Database Abstraction Layer

A Go database abstraction layer that provides identical interfaces for SQL and in-memory backends. Currently implements in-memory storage with plans for SQL backends (PostgreSQL, SQLite).

## Features

- **Unified Interface**: Identical API for all backends
- **In-Memory Storage**: Fast, thread-safe in-memory database for development and testing
- **Type-Safe Queries**: Structured query building with filtering, sorting, and pagination
- **Constraint Enforcement**: Unique constraints, foreign keys, and data validation
- **Transaction Support**: ACID transactions with rollback capability
- **Schema Management**: Code-first schema definitions and migrations
- **Concurrent Access**: Thread-safe operations with proper locking

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/leafsii/leafsii-backend/internal/db"
    "github.com/leafsii/leafsii-backend/internal/db/entities"
    "github.com/leafsii/leafsii-backend/internal/db/interfaces"
)

func main() {
    ctx := context.Background()
    
    // Create database
    database := db.NewInMemoryDatabase()
    
    // Connect and migrate
    if err := db.ConnectAndMigrate(ctx, database, db.AllSchemas()); err != nil {
        log.Fatal(err)
    }
    defer database.Disconnect(ctx)
    
    // Get repository
    userRepo := database.Repository(entities.UserSchema)
    
    // Create user
    user, err := userRepo.Create(ctx, map[string]interface{}{
        "email":     "john@example.com",
        "name":      "John Doe",
        "age":       30,
        "is_active": true,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Query users
    activeUsers, err := userRepo.FindMany(ctx, &interfaces.Query{
        Where: &interfaces.Filters{
            Conditions: []interfaces.Filter{
                {Field: "is_active", Value: true},
            },
        },
        OrderBy: []interfaces.OrderBy{
            {Field: "name", Direction: "asc"},
        },
        Limit: func(i int) *int { return &i }(10),
    })
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Found %d active users\n", activeUsers.Total)
}
```

## Entity Definition

Define your entities and schemas:

```go
// User entity
type User struct {
    ID        string    `json:"id" db:"id"`
    Email     string    `json:"email" db:"email"`
    Name      string    `json:"name" db:"name"`
    Age       *int      `json:"age,omitempty" db:"age"`
    IsActive  bool      `json:"is_active" db:"is_active"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
    UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Schema definition
var UserSchema = &interfaces.Schema{
    TableName: "users",
    Fields: map[string]interfaces.FieldSchema{
        "id":         {Type: "string", PrimaryKey: true},
        "email":      {Type: "string", Unique: true},
        "name":       {Type: "string"},
        "age":        {Type: "int", Nullable: true},
        "is_active":  {Type: "bool", DefaultValue: true},
        "created_at": {Type: "time"},
        "updated_at": {Type: "time"},
    },
    Indexes: []interfaces.Index{
        {Name: "idx_users_email", Columns: []string{"email"}, Unique: true},
    },
}
```

## Repository Operations

### CRUD Operations

```go
// Create
user, err := repo.Create(ctx, map[string]interface{}{
    "email": "user@example.com",
    "name":  "User Name",
})

// Read
user, err := repo.GetByID(ctx, interfaces.StringID("user-id"))

// Update
user, err := repo.Update(ctx, interfaces.StringID("user-id"), map[string]interface{}{
    "name": "Updated Name",
})

// Delete
err := repo.Delete(ctx, interfaces.StringID("user-id"))

// Count
count, err := repo.Count(ctx, &interfaces.Query{
    Where: &interfaces.Filters{
        Conditions: []interfaces.Filter{
            {Field: "is_active", Value: true},
        },
    },
})
```

### Advanced Queries

```go
// Complex filtering
users, err := repo.FindMany(ctx, &interfaces.Query{
    Where: &interfaces.Filters{
        Conditions: []interfaces.Filter{
            {Field: "is_active", Value: true},
            {
                Field: "age",
                Operator: &interfaces.FilterOperator{
                    Gte: 18,
                    Lt:  65,
                },
            },
        },
        OR: []*interfaces.Filters{
            {
                Conditions: []interfaces.Filter{
                    {Field: "name", Operator: &interfaces.FilterOperator{Like: "%admin%"}},
                },
            },
        },
    },
    OrderBy: []interfaces.OrderBy{
        {Field: "created_at", Direction: "desc"},
        {Field: "name", Direction: "asc"},
    },
    Limit:  func(i int) *int { return &i }(20),
    Offset: func(i int) *int { return &i }(0),
})
```

### Filter Operators

- **Equality**: `Value: "exact match"`
- **Comparison**: `Operator: &FilterOperator{Gt: 10, Lte: 100}`
- **Array**: `Operator: &FilterOperator{In: []interface{}{1, 2, 3}}`
- **Pattern**: `Operator: &FilterOperator{Like: "%pattern%"}`
- **Null checks**: `Operator: &FilterOperator{IsNull: true}`

## Transactions

```go
err := db.Transaction(ctx, func(ctx context.Context, tx interfaces.Transaction) error {
    // Create user
    user, err := userRepo.Create(ctx, userData)
    if err != nil {
        return err // Triggers rollback
    }
    
    // Create related post
    _, err = postRepo.Create(ctx, map[string]interface{}{
        "title":     "User's First Post",
        "author_id": user["id"],
    })
    if err != nil {
        return err // Triggers rollback
    }
    
    return nil // Commits transaction
})
```

## Configuration

```go
// Database configuration
config := &db.Config{
    Type:        "memory", // "memory", "postgres", "sqlite"
    DSN:         "",       // Connection string for SQL databases
    UseInMemory: false,    // Force in-memory usage
}

database, err := db.NewDatabase(config)

// Or use environment variables
// DB_TYPE=memory
// DB_DSN=postgres://user:pass@host/db
// USE_IN_MEMORY=true
```

## Testing

Run tests with:

```bash
go test ./internal/db/...
```

Run the example:

```bash
go run cmd/db-example/main.go
```

## Architecture

```
internal/db/
├── interfaces/          # Core interfaces and types
│   ├── database.go     # Database interface
│   ├── repository.go   # Repository interface  
│   ├── transaction.go  # Transaction interface
│   └── types.go        # Common types and errors
├── backends/
│   └── memory/         # In-memory implementation
│       ├── database.go
│       ├── repository.go
│       └── transaction.go
├── entities/           # Entity definitions and schemas
│   ├── user.go
│   └── post.go
├── query/              # Query building utilities
│   └── builder.go
├── factory.go          # Database factory
├── fixtures.go         # Test data fixtures
└── README.md
```

## Supported Features

### Current (In-Memory)
- ✅ CRUD operations with auto-generated IDs and timestamps
- ✅ Complex filtering with AND/OR logic and comparison operators
- ✅ Sorting with multiple fields and directions
- ✅ Pagination with limit/offset
- ✅ Unique and foreign key constraint enforcement
- ✅ ACID transactions with rollback support
- ✅ Concurrent access with proper locking
- ✅ Schema validation and type checking

### Planned (SQL Backends)
- 🔄 PostgreSQL backend with connection pooling
- 🔄 SQLite backend for file-based storage
- 🔄 Query optimization and prepared statements
- 🔄 Database migrations and schema versioning
- 🔄 Connection health checking and retry logic

## Performance Considerations

The in-memory backend is designed for:
- **Development**: Fast local development without external dependencies
- **Testing**: Isolated, predictable test environments
- **Small Datasets**: Datasets that fit comfortably in memory
- **High Performance**: Zero network latency, pure in-process access

For production use with large datasets or persistence requirements, use SQL backends when available.

## Error Handling

The package defines standard error types:

```go
// Standard errors
interfaces.ErrNotFound              // Record not found
interfaces.ErrUniqueConstraint      // Unique constraint violation
interfaces.ErrForeignKeyConstraint  // Foreign key constraint violation
interfaces.ErrInvalidQuery          // Malformed query
interfaces.ErrDatabaseNotConnected  // Database connection issue

// Custom errors with context
&interfaces.DatabaseError{
    Op:  "create_user",
    Err: interfaces.ErrUniqueConstraint,
}
```

## Contributing

1. Add new entity schemas in `entities/`
2. Extend query capabilities in `query/builder.go`
3. Implement SQL backends in `backends/sql/`
4. Add comprehensive tests for new features
5. Update documentation and examples

## License

This package is part of the webservice project.