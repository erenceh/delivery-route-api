package main

import (
	"database/sql"
	"delivery-route-service/internal/adapters/repositories"
	"delivery-route-service/internal/config"
	"delivery-route-service/internal/platform/db"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (using environment variables)")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(databaseURL) == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := db.Open(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	seedPath := config.Get("SEED_PATH", "data/seeds/packages.json")
	if err := initAndSeed(db, seedPath); err != nil {
		log.Fatal(err)
	}

}

func initAndSeed(db *sql.DB, seedPath string) error {
	log.Println("Initializing database schema...")
	if err := repositories.InitSchema(db); err != nil {
		log.Fatalf("schema initialization failed: %v", err)
	}
	log.Println("Schema ready.")

	log.Println("Seeding database...")
	if err := repositories.SeedFromJSON(db, seedPath); err != nil {
		log.Fatalf("seeding failed: %v", err)
	}
	log.Println("Seeding complete.")

	return nil
}
