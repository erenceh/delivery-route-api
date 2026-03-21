package main

import (
	"delivery-route-service/internal/adapters/cache"
	"delivery-route-service/internal/adapters/distance"
	"delivery-route-service/internal/adapters/repositories"
	"delivery-route-service/internal/api"
	"delivery-route-service/internal/config"
	"delivery-route-service/internal/platform/db"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

// main is the application composition root.
// It wires concrete adapters (Postgres, ORS) behind ports and starts the HTTP server.
func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (using environment variables)")
	}
	hub := config.Get("HUB_ADDRESS", "1901 W Madison St, Phoenix, AZ 85009")
	port := config.Get("PORT", "8080")
	orsKey := os.Getenv("ORS_API_KEY")

	databaseURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(databaseURL) == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := db.Open(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	redisURL := os.Getenv("REDIS_URL")
	if strings.TrimSpace(redisURL) == "" {
		log.Fatal("REDIS_URL is required")
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatal("invalid REDIS_URL: ", err)
	}
	rdb := redis.NewClient(opt)

	if strings.TrimSpace(orsKey) == "" {
		log.Fatal("ORS_API_KEY is required")
	}

	// ORS provider uses persistent DB caches to avoid repeated geocode/matrix calls.
	distanceCache := cache.NewRedisDistanceCache(rdb)
	geocodeCache := cache.NewRedisGeocodeCache(rdb)
	provider, err := distance.NewORSDistanceProvider(orsKey, distanceCache, geocodeCache)
	if err != nil {
		log.Fatal(err)
	}

	repo := repositories.NewSQLPackageRepository(db)
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
