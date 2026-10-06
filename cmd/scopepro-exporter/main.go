package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/raylas/scopepro-exporter/internal/collector"
	"github.com/raylas/scopepro-exporter/internal/scopepro"
)

// version is set at build time via ldflags.
var version = "dev"

func run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	addr := flag.String("addr", ":9993", "server bind address")
	devices := flag.String("devices", "", "comma-separated device paths (required)")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	namespace := flag.String("namespace", "scopepro", "metric namespace prefix")
	scopeproPath := flag.String("scopepro-path", "scopepro", "path to scopepro binary")
	timeout := flag.Duration("timeout", 30*time.Second, "timeout per scopepro invocation (0 to disable)")
	flag.Parse()

	var devList []string
	for dev := range strings.SplitSeq(*devices, ",") {
		if dev = strings.TrimSpace(dev); dev != "" {
			devList = append(devList, dev)
		}
	}
	if len(devList) == 0 {
		return fmt.Errorf("at least one device is required: -devices /dev/sda,/dev/nvme0n1")
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		return fmt.Errorf("invalid -log-level %q", *logLevel)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	executor := scopepro.New(*scopeproPath, *timeout)
	c := collector.New(*namespace, version, devList, executor, logger)
	prometheus.MustRegister(c)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/{$}", http.RedirectHandler("/metrics", http.StatusMovedPermanently))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("starting scopepro exporter", "version", version, "addr", *addr, "devices", devList)

	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown failed", "err", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
