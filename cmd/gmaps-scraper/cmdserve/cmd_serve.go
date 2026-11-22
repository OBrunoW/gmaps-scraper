package cmdserve

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"

	"github.com/OBrunoW/gmaps-scraper/internal/adapters/apibrasil"
	"github.com/OBrunoW/gmaps-scraper/internal/adapters/geocoder"
	"github.com/OBrunoW/gmaps-scraper/internal/adapters/hashid"
	httpadapter "github.com/OBrunoW/gmaps-scraper/internal/adapters/http"
	"github.com/OBrunoW/gmaps-scraper/internal/adapters/postgres"
	riveradapter "github.com/OBrunoW/gmaps-scraper/internal/adapters/river"
	"github.com/OBrunoW/gmaps-scraper/internal/application"
	"github.com/OBrunoW/gmaps-scraper/internal/config"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
	"github.com/OBrunoW/gmaps-scraper/internal/middleware"
	"github.com/OBrunoW/gmaps-scraper/migrations"
)

const (
	envDBMaxConns        = "DB_MAX_CONNS"
	envDBMinConns        = "DB_MIN_CONNS"
	envDBMaxConnLifetime = "DB_MAX_CONN_LIFETIME"
	envDBMaxConnIdleTime = "DB_MAX_CONN_IDLE_TIME"
)

var Command = &cli.Command{
	Name:  "serve",
	Usage: "Start the API server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "addr",
			Usage:   "Server listen address",
			Value:   "",
			Sources: cli.EnvVars(config.EnvAddr),
		},
		&cli.StringFlag{
			Name:    "database-url",
			Usage:   "PostgreSQL connection string",
			Value:   "",
			Sources: cli.EnvVars(config.EnvDatabaseURL),
		},
		&cli.IntFlag{
			Name:    "db-max-conns",
			Usage:   "Maximum database connections",
			Value:   10,
			Sources: cli.EnvVars(envDBMaxConns),
		},
		&cli.IntFlag{
			Name:    "db-min-conns",
			Usage:   "Minimum database connections",
			Value:   2,
			Sources: cli.EnvVars(envDBMinConns),
		},
		&cli.DurationFlag{
			Name:    "db-max-conn-lifetime",
			Usage:   "Maximum connection lifetime",
			Value:   time.Hour,
			Sources: cli.EnvVars(envDBMaxConnLifetime),
		},
		&cli.DurationFlag{
			Name:    "db-max-conn-idle-time",
			Usage:   "Maximum connection idle time",
			Value:   30 * time.Minute,
			Sources: cli.EnvVars(envDBMaxConnIdleTime),
		},
	},
	Action: run,
}

func run(ctx context.Context, cmd *cli.Command) error {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err = cfg.ValidateAuth(); err != nil {
		return err
	}

	addr := cmd.String("addr")
	if addr == "" {
		addr = cfg.Addr
	}

	dsn := cmd.String("database-url")
	if dsn == "" {
		dsn = cfg.DatabaseURL
	}

	logUnsetEnvs(config.EnvDatabaseURL)

	applied, err := migrations.RunWithDSN(dsn)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if applied > 0 {
		log.Info("applied migrations", "count", applied)
	}

	dbPool, err := postgres.Connect(ctx, dsn,
		postgres.WithMaxConns(int32(cmd.Int("db-max-conns"))),
		postgres.WithMinConns(int32(cmd.Int("db-min-conns"))),
		postgres.WithMaxConnLifetime(cmd.Duration("db-max-conn-lifetime")),
		postgres.WithMaxConnIdleTime(cmd.Duration("db-max-conn-idle-time")),
	)
	if err != nil {
		return err
	}
	defer dbPool.Close()

	codec, err := hashid.NewCodec(cfg.HashIDSalt)
	if err != nil {
		return err
	}

	riverClient, err := riveradapter.NewClient(dbPool)
	if err != nil {
		return err
	}

	jobService := application.NewJobService(
		riverClient.Queue(),
		postgres.NewResultRepository(dbPool),
		postgres.NewJobRepository(dbPool),
		codec,
	)
	riverClient.SetMaintenance(jobService)

	if err = riverClient.Start(ctx); err != nil {
		return err
	}

	prospectorHandlers := newProspectorHandlers(cfg, jobService)

	srv, err := httpadapter.New(
		httpadapter.NewRouter(jobService, prospectorHandlers, authMiddleware(cfg)),
		httpadapter.WithAddr(addr),
	)
	if err != nil {
		return err
	}

	log.Info("starting server", "addr", addr)

	return srv.Run(ctx)
}

func newProspectorHandlers(cfg *config.Config, jobService *application.JobService) *httpadapter.ProspectorHandlers {
	prospector := application.NewProspectorService(jobService, geocoder.NewNominatim(cfg.GeocoderURL))

	if !cfg.HasAPIBrasil() {
		log.Warn(fmt.Sprintf("environment variable %s is not set. CEP and CNPJ endpoints will respond 503", config.EnvAPIBrasilToken))
		return httpadapter.NewProspectorHandlers(prospector, nil, nil)
	}

	client := apibrasil.NewClient(cfg.APIBrasilURL, cfg.APIBrasilToken, cfg.APIBrasilHomolog)

	cepService := application.NewCEPService(client)

	return httpadapter.NewProspectorHandlers(
		prospector.WithCEPResolver(cepService),
		cepService,
		application.NewCNPJService(client),
	)
}

func authMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	if cfg.AllowNoopAuth() {
		return middleware.NoopAuth
	}

	return middleware.NewServiceAuth(cfg.ServiceKeys, cfg.APIKey).Handler
}

func logUnsetEnvs(names ...string) {
	for _, name := range names {
		if !config.IsEnvSet(name) {
			log.Warn(fmt.Sprintf("environment variable %s is not set. This can be insecure and hould be used only for testing", name))
		}
	}
}
