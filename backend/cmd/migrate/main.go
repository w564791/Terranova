package main

import (
	"context"
	"log"

	"iac-platform/internal/config"
	"iac-platform/internal/database"
	"iac-platform/internal/migration"
)

func main() {
	db, err := database.Initialize(config.LoadDatabase())
	if err != nil {
		log.Fatalf("connect database for migration: %v", err)
	}
	if err := migration.Run(context.Background(), db); err != nil {
		log.Fatalf("database migration failed: %v", err)
	}
	log.Println("database migrations completed successfully")
}
