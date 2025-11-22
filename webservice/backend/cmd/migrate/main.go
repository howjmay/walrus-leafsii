package main

import (
	"database/sql"
	"flag"
	"log"

	"github.com/leafsii/leafsii-backend/internal/config"
	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	flags = flag.NewFlagSet("migrate", flag.ExitOnError)
	dir   = flags.String("dir", "sql", "directory with migration files")
)

func main() {
	flags.Parse(flag.Args())
	args := flags.Args()

	if len(args) < 1 {
		log.Fatal("Usage: migrate COMMAND\n\nCommands:\n  up\n  down\n  status")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := sql.Open("pgx", cfg.Database.PostgresDSN)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("Failed to set dialect: %v", err)
	}

	command := args[0]
	switch command {
	case "up":
		if err := goose.Up(db, *dir); err != nil {
			log.Fatalf("Migration up failed: %v", err)
		}
	case "down":
		if err := goose.Down(db, *dir); err != nil {
			log.Fatalf("Migration down failed: %v", err)
		}
	case "status":
		if err := goose.Status(db, *dir); err != nil {
			log.Fatalf("Migration status failed: %v", err)
		}
	default:
		log.Fatalf("Unknown command: %s", command)
	}
}
