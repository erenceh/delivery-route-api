CREATE TABLE IF NOT EXISTS packages (
	package_id INTEGER PRIMARY KEY,
	destination TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS distance_cache (
    origin TEXT NOT NULL,
    destination TEXT NOT NULL,
    distance_meters INTEGER NOT NULL,
    duration_seconds INTEGER NOT NULL,
    PRIMARY KEY (origin, destination)
);


CREATE TABLE IF NOT EXISTS geocode_cache (
    address TEXT PRIMARY KEY,
    lon DOUBLE PRECISION NOT NULL,
    lat DOUBLE PRECISION NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_distance_cache_destination_origin
    ON distance_cache(destination, origin);
