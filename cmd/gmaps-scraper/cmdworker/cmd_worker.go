package cmdworker

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"

	httpadapter "github.com/OBrunoW/gmaps-scraper/internal/adapters/http"
	"github.com/OBrunoW/gmaps-scraper/internal/adapters/postgres"
	riveradapter "github.com/OBrunoW/gmaps-scraper/internal/adapters/river"
	"github.com/OBrunoW/gmaps-scraper/internal/adapters/scraper"
	"github.com/OBrunoW/gmaps-scraper/internal/application"
	"github.com/OBrunoW/gmaps-scraper/internal/config"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

const (
	envConcurrency     = "CONCURRENCY"
	envFastMode        = "FAST_MODE"
	envDebug           = "DEBUG"
	envMaxJobsPerCycle = "MAX_JOBS_PER_CYCLE"
	envProxies         = "PROXIES"

	healthAddr  = ":8080"
	flushGrace  = 90 * time.Second
	stopTimeout = 10 * time.Second
)

var Command = &cli.Command{
	Name:  "worker",
	Usage: "Start the scraper worker",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "database-url",
			Usage:   "PostgreSQL connection string",
			Value:   "",
			Sources: cli.EnvVars(config.EnvDatabaseURL),
		},
		&cli.IntFlag{
			Name:    "concurrency",
			Aliases: []string{"c"},
			Usage:   "Requested worker instances for host provisioning (each process runs one River job at a time)",
			Value:   1,
			Sources: cli.EnvVars(envConcurrency),
		},
		&cli.BoolFlag{
			Name:    "fast",
			Usage:   "Enable fast mode (stealth HTTP requests)",
			Value:   false,
			Sources: cli.EnvVars(envFastMode),
		},
		&cli.BoolFlag{
			Name:    "debug",
			Usage:   "Enable debug mode (headful browser)",
			Value:   false,
			Sources: cli.EnvVars(envDebug),
		},
		&cli.Int64Flag{
			Name:    "max-jobs-per-cycle",
			Usage:   "Maximum jobs before restarting scraper",
			Value:   100,
			Sources: cli.EnvVars(envMaxJobsPerCycle),
		},
		&cli.StringFlag{
			Name:    "proxies",
			Usage:   "Comma-separated list of proxy URLs",
			Value:   "",
			Sources: cli.EnvVars(envProxies),
		},
	},
	Action: run,
}

type workerStats struct {
	startedAt        time.Time
	concurrency      int
	manager          *scraper.ScraperManager
	runner           *application.ScrapeRunner
	jobsProcessed    atomic.Int64
	resultsCollected atomic.Int64
}

func (s *workerStats) Uptime() time.Duration       { return time.Since(s.startedAt) }
func (s *workerStats) JobsProcessed() int64        { return s.jobsProcessed.Load() }
func (s *workerStats) ResultsCollected() int64     { return s.resultsCollected.Load() }
func (s *workerStats) Concurrency() int            { return s.concurrency }
func (s *workerStats) ActiveJobs() int64           { return s.manager.ActiveJobs() }
func (s *workerStats) FlushWaitWarnTotal() int64   { return s.runner.Metrics().FlushWaitWarnTotal }
func (s *workerStats) LongRuntimeWarnTotal() int64 { return s.runner.Metrics().LongRuntimeWarnTotal }

func (s *workerStats) incrementJobsProcessed() {
	s.jobsProcessed.Add(1)
}

func (s *workerStats) addResultsCollected(count int) {
	s.resultsCollected.Add(int64(count))
}

func run(ctx context.Context, cmd *cli.Command) error {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("shutdown signal received")
		cancel()
	}()

	configuredConcurrency := max(1, cmd.Int("concurrency"))
	concurrency := 1
	maxRiverWorkers := 1
	fastMode := cmd.Bool("fast")
	debug := cmd.Bool("debug")
	maxJobsPerCycle := cmd.Int64("max-jobs-per-cycle")
	proxies := parseProxies(cmd.String("proxies"))

	dsn := cmd.String("database-url")
	if dsn == "" {
		dsn = cfg.DatabaseURL
	}
	dbMaxConns := int32(3)
	dbMinConns := int32(1)

	dbPool, err := postgres.Connect(ctx, dsn,
		postgres.WithMaxConns(dbMaxConns),
		postgres.WithMinConns(dbMinConns),
	)
	if err != nil {
		return err
	}
	defer dbPool.Close()

	log.Info("worker configuration",
		"configured_container_instances", configuredConcurrency,
		"scraper_concurrency", concurrency,
		"river_max_workers", maxRiverWorkers,
		"db_max_conns", dbMaxConns,
		"db_min_conns", dbMinConns,
		"fast_mode", fastMode,
		"max_jobs_per_cycle", maxJobsPerCycle,
		"proxy_count", len(proxies),
	)

	results := postgres.NewResultRepository(dbPool)
	manager := scraper.NewScraperManager(results, concurrency, fastMode, debug, maxJobsPerCycle, proxies)
	runner := application.NewScrapeRunner(manager, flushGrace)

	stats := &workerStats{
		startedAt:   time.Now(),
		concurrency: concurrency,
		manager:     manager,
		runner:      runner,
	}
	manager.OnJobComplete = stats.incrementJobsProcessed
	manager.CentralWriter().OnResultsSaved = stats.addResultsCollected

	go runHealthServer(ctx, stats)

	client, err := riveradapter.NewWorkerClient(dbPool, runner)
	if err != nil {
		return err
	}

	log.Info("starting River worker")

	if err := client.Start(ctx); err != nil {
		return err
	}

	client.StartRetryPromoter(ctx)

	log.Info("starting scraper manager")

	if err := manager.Run(ctx); err != nil {
		if ctx.Err() != nil {
			log.Info("scraper manager stopped due to shutdown")
		} else {
			return err
		}
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), stopTimeout)
	defer stopCancel()

	if err := client.Stop(stopCtx); err != nil {
		log.Error("error stopping River client", "error", err)
	}

	log.Info("worker shutdown complete")

	return nil
}

func runHealthServer(ctx context.Context, stats httpadapter.WorkerStats) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", httpadapter.NewWorkerHealthHandler(stats))

	srv, err := httpadapter.New(mux, httpadapter.WithAddr(healthAddr))
	if err != nil {
		log.Error("health server error", "error", err)
		return
	}

	log.Info("starting health server on " + healthAddr)

	if err := srv.Run(ctx); err != nil {
		log.Error("health server error", "error", err)
	}
}

func parseProxies(val string) []string {
	if val == "" {
		return nil
	}

	parts := strings.Split(val, ",")
	proxies := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			proxies = append(proxies, p)
		}
	}

	return proxies
}
