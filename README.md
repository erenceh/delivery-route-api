# delivery-route-api

Backend delivery route planning service written in Go with OpenRouteService integration, SQLite persistence, and cold/warm cache performance optimization.

## Overview

Delivery Route API is a backend service written in Go that performs greedy route planning for delivery trucks using real-world road distance data from OpenRouteService (ORS). The service emphasizes clean separation of concerns, explicit interface boundaries, and observable request behavior.

The service demonstrates:

- Layered backend architecture (domain, ports, adapters)
- Integration with an external routing API (OpenRouteService)
- SQLite persistence
- Cold vs warm cache optimization
- Context-aware request handling
- Retry/backoff for resilient API communication
- HTTP server observability (latency + response metrics)

This project began as a CLI-based routing program written in Python and was redesigned in Go as a layered HTTP backend service to explore production-style architecture, persistence, and external API integration.

## Features

- Greedy nearest-neighbor route planning
- Destination assignment across multiple trucks
- OpenRouteService integration (geocoding + matrix API)
- SQLite-backed:
  - Package storage
  - Distance cache
  - Geocode cache
- Cold-start performance optimization
- Retry and exponential backoff on external API calls
- Request latency and byte-level logging middleware

## Architecture

This is a **single-process HTTP service** using layered architecture.

```
cmd/server          -> application entrypoint
internal/api        -> HTTP handlers + DTOs + middleware
internal/services   -> routing & assignment logic
internal/domain     -> core business entities
internal/ports      -> interfaces (DistanceProvider, Repository)
internal/adapters   -> SQLite + ORS implementations
```

The domain layer is independent of infrastructure concerns.

External integrations (SQLite and ORS) are implemented as adapters behind interface boundaries.

## Route Planning Strategy

The service uses a greedy nearest-neighbor algorithm:

1. Start at the hub.
2. Repeatedly select the next destination with the shortest travel duration.
3. Continue until all packages assigned to a truck are delivered.

Package assignment across trucks uses a distance-sorted chunking heuristic to distribute destinations deterministically across available trucks:

- Destinations are sorted by distance from the hub.
- Destinations are evenly distributed across trucks.

This approach is intentionally simple and deterministic. Full logistics optimization (VRP solvers, time windows, etc.) is out of scope for this project.

## Performance & Caching

The system maintains persistent SQLite caches for:

- Geocode results (address -> coordinates)
- Distance matrix results (origin -> destination)

### Cold Run

- Requires ORS geocode + matrix API calls
- Typical latency (20 destinations): ~20-25 seconds

### Warm Run

- All distances served from SQLite cache
- Typical latency: ~2-5 milliseconds

This demonstrates the impact of persistent caching on reducing repeated external API latency.

## API Endpoints

### Health Check

GET `/health`

### List Packages

GET `/packages`

### Plan Routes

POST `/plans`

Request body (optional):

```
{
    "hub": "1901 W Madison St, Phoenix, AZ 85009",
    "depart_at": "2026-02-18T08:00:00Z",
    "return_to_start": false,
    "truck_count": 3,
    "truck_capacity": 16
}
```

## Running Locally

### Requirements

- Go 1.22+
- OpenRouteService API key

### Environment Variables

```
ORS_API_KEY=YOUR_KEY_HERE
DB_PATH="data/app.db"
SEED_PATH="data/seeds/packages.json"
PORT="8080"
HUB_ADDRESS="1901 W Madison St, Phoenix, AZ 85009"
```

### Run

```
go run ./cmd/server
```

Then:

```
curl -X POST http://localhost:8080/plans \
    -H "Content-Type: application/json" \
    -d '{}'
```

## Observability

The server includes request logging middleware:

- HTTP method
- Path
- Status code
- Bytes written
- Request duration (ms)

`method=POST path=/plans status=200 bytes=2889 dur=24344ms`

## Future Improvements

- Smarter geographic clustering for truck assignment
- Parallelized geocoding with bounded concurrency
- Rate-limit-aware ORS call coordination
- Metrics integration (Prometheus/OpenTelemetry)
- Docker containerization

## About the Architecture Choice

This project intentionally uses a single-service architecture. The focus is correctness, maintainability, and clean layering rather than distributed system complexity.
