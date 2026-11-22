package saas

const (
	DefaultDatabaseURL = "postgres://postgres:postgres@localhost:5432/gmaps_scraper?sslmode=disable" //nolint:gosec

	EnvAddr              = "ADDR"
	EnvDatabaseURL       = "DATABASE_URL"
	EnvDBMaxConns        = "DB_MAX_CONNS"
	EnvDBMinConns        = "DB_MIN_CONNS"
	EnvDBMaxConnLifetime = "DB_MAX_CONN_LIFETIME"
	EnvDBMaxConnIdleTime = "DB_MAX_CONN_IDLE_TIME"

	EnvConcurrency     = "CONCURRENCY"
	EnvFastMode        = "FAST_MODE"
	EnvDebug           = "DEBUG"
	EnvMaxJobsPerCycle = "MAX_JOBS_PER_CYCLE"
	EnvProxies         = "PROXIES"
	EnvRiverMaxWorkers = "RIVER_MAX_WORKERS"
)
