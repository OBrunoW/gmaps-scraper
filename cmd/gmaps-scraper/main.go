package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/OBrunoW/gmaps-scraper/cmd/gmaps-scraper/cmdserve"
	"github.com/OBrunoW/gmaps-scraper/cmd/gmaps-scraper/cmdworker"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

func main() {
	cmd := &cli.Command{
		Name:    "gmaps-scraper",
		Usage:   "Google Maps scraper microservice (API + worker)",
		Version: "1.0.0",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug logging",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			level := slog.LevelInfo
			if cmd.Bool("debug") {
				level = slog.LevelDebug
			}

			log.Init(level)

			return ctx, nil
		},
		Commands: []*cli.Command{
			cmdserve.Command,
			cmdworker.Command,
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Error("application failed", "error", err)
		os.Exit(1)
	}
}
