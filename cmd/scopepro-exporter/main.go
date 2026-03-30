package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/raylas/scopepro-exporter/internal/collector"
	"github.com/raylas/scopepro-exporter/internal/scopepro"
	"github.com/rs/zerolog"
)

var version = "dev"

func run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	addr := flag.String("addr", ":9993", "server bind address")
	devices := flag.String("devices", "", "comma-separated device paths (required)")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	namespace := flag.String("namespace", "scopepro", "metric namespace prefix")
	scopeproPath := flag.String("scopepro-path", "scopepro", "path to scopepro binary")
	flag.Parse()

	if *devices == "" {
		return fmt.Errorf("at least one device is required: -devices /dev/sda,/dev/nvme0n1")
	}

	// Logging
	level, _ := zerolog.ParseLevel(*logLevel)
	zerolog.SetGlobalLevel(level)
	logger := zerolog.New(
		zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.Stamp},
	).With().Timestamp().Logger()

	// Parse devices
	devList := strings.Split(*devices, ",")
	for i := range devList {
		devList[i] = strings.TrimSpace(devList[i])
	}

	// Build executor and collector
	executor := scopepro.New(*scopeproPath)
	collector.Version = version
	c := collector.New(*namespace, devList, executor, logger)
	prometheus.MustRegister(c)

	// HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/metrics", http.StatusMovedPermanently)
	})

	srv := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	logger.Info().
		Str("version", version).
		Str("addr", *addr).
		Strs("devices", devList).
		Msg("starting scopepro exporter")

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		logger.Info().Msg("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
