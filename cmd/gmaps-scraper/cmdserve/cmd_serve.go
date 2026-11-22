package cmdserve

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"

	"github.com/OBrunoW/gmaps-scraper/api"
	"github.com/OBrunoW/gmaps-scraper/env"
	"github.com/OBrunoW/gmaps-scraper/httpext"
	"github.com/OBrunoW/gmaps-scraper/log"
	"github.com/OBrunoW/gmaps-scraper/migrations"
	"github.com/OBrunoW/gmaps-scraper/postgres"
	"github.com/OBrunoW/gmaps-scraper/rqueue"
	saas "github.com/OBrunoW/gmaps-scraper/saas"
)

var Command = &cli.Command{
	Name:  "serve",
	Usage: "Start the API server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "addr",
			Usage:   "Server listen address",
			Value:   "",
			Sources: cli.EnvVars(saas.EnvAddr),
		},
		&cli.StringFlag{
			Name:    "database-url",
			Usage:   "PostgreSQL connection string",
			Value:   "",
			Sources: cli.EnvVars(saas.EnvDatabaseURL),
		},
		&cli.IntFlag{
			Name:    "db-max-conns",
			Usage:   "Maximum database connections",
			Value:   10,
			Sources: cli.EnvVars(saas.EnvDBMaxConns),
		},
		&cli.IntFlag{
			Name:    "db-min-conns",
			Usage:   "Minimum database connections",
			Value:   2,
			Sources: cli.EnvVars(saas.EnvDBMinConns),
		},
		&cli.DurationFlag{
			Name:    "db-max-conn-lifetime",
			Usage:   "Maximum connection lifetime",
			Value:   time.Hour,
			Sources: cli.EnvVars(saas.EnvDBMaxConnLifetime),
		},
		&cli.DurationFlag{
			Name:    "db-max-conn-idle-time",
			Usage:   "Maximum connection idle time",
			Value:   30 * time.Minute,
			Sources: cli.EnvVars(saas.EnvDBMaxConnIdleTime),
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		_ = godotenv.Load()

		addr := cmd.String("addr")
		if addr == "" {
			port := os.Getenv("PORT")
			if port == "" {
				port = "8080"
			}
			addr = ":" + port
		}

		dsn := cmd.String("database-url")
		if dsn == "" {
			dsn = buildDatabaseURL()
		}
		if dsn == "" {
			dsn = saas.DefaultDatabaseURL
		}

		env.LogUnsetEnvs(saas.EnvDatabaseURL)

		n, err := migrations.RunWithDSN(dsn)
		if err != nil {
			return fmt.Errorf("migrations: %w", err)
		}
		if n > 0 {
			log.Info("applied migrations", "count", n)
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

		rqueueClient, err := rqueue.NewClient(dbPool)
		if err != nil {
			return err
		}

		if err = rqueueClient.Start(ctx); err != nil {
			return err
		}

		auth := api.AuthFromEnv()
		apiState := api.NewAppState(rqueueClient, auth)

		mainRouter := chi.NewRouter()
		mainRouter.Use(middleware.Recoverer)
		api.Routes(mainRouter, apiState)

		srv, err := httpext.New(mainRouter, httpext.WithAddr(addr))
		if err != nil {
			return err
		}

		log.Info("starting server", "addr", addr)

		return srv.Run(ctx)
	},
}

func buildDatabaseURL() string {
	host := os.Getenv("DB_HOST")
	if host == "" {
		return ""
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	name := os.Getenv("DB_NAME")
	ssl := os.Getenv("DB_SSLMODE")
	if ssl == "" {
		ssl = "disable"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, ssl)
}
