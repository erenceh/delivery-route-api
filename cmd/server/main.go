package main

import (
	"database/sql"
	"delivery-route-service/internal/adapters/cache"
	"delivery-route-service/internal/adapters/distance"
	"delivery-route-service/internal/adapters/repositories"
	"delivery-route-service/internal/api"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

// main is the application composition root.
// It wires concrete adapters (SQLite, ORS) behind ports and starts the HTTP server.
func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (using environment variables)")
	}

	dbPath := getEnv("DB_PATH", "data/app.db")
	seedPath := getEnv("SEED_PATH", "data/seeds/packages.json")
	hub := getEnv("HUB_ADDRESS", "1901 W Madison St, Phoenix, AZ 85009")
	port := getEnv("PORT", "8080")

	orsKey := os.Getenv("ORS_API_KEY")
	if strings.TrimSpace(orsKey) == "" {
		log.Fatal("ORS_API_KEY is required")
	}

	db, err := openDB(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Initialize schema and seed demo data on startup for local runs.
	if err := initAndSeed(db, seedPath); err != nil {
		log.Fatal(err)
	}

	// ORS provider uses persistent SQLite caches to avoid repeated geocode/matrix calls.
	distanceCache := cache.NewSqliteDistanceCache(db)
	geocodeCache := cache.NewSqliteGeocodeCache(db)
	provider, err := distance.NewORSDistanceProvider(orsKey, distanceCache, geocodeCache)
	if err != nil {
		log.Fatal(err)
	}

	repo := repositories.NewSqlitePackageRepository(db)
	router := api.NewRouter(repo, provider, hub)

	// Timeouts are tuned for cold-cache route planning (external API latency).
	log.Printf("Server listening addr=:%s", port)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("openDB: open sqlite database %q: %w", dbPath, err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("openDB: verify sqlite connection to %q: %w", dbPath, err)
	}

	return db, nil
}

func initAndSeed(db *sql.DB, seedPath string) error {
	if err := repositories.InitSchema(db); err != nil {
		return fmt.Errorf("init and seed: %w", err)
	}

	if err := repositories.SeedFromJSON(db, seedPath); err != nil {
		return fmt.Errorf("init and seed: %w", err)
	}

	return nil
}
