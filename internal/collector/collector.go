package collector

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/raylas/scopepro-exporter/internal/scopepro"
	"github.com/rs/zerolog"
)

// Executor abstracts ScopePro CLI interactions for testability.
type Executor interface {
	DriveInfoQuery(ctx context.Context, device string) (*scopepro.DriveInfo, error)
	SmartInfo(ctx context.Context, device string) (map[string]float64, error)
	Health(ctx context.Context, device string) (float64, error)
}

// Collector implements prometheus.Collector for ScopePro metrics.
type Collector struct {
	devices   []string
	namespace string
	executor  Executor
	logger    zerolog.Logger

	driveInfo     *prometheus.Desc
	healthPct     *prometheus.Desc
	scrapeErrors  *prometheus.Desc
	buildInfo     *prometheus.Desc

	// Track scrape errors across scrapes
	mu          sync.Mutex
	errorCounts map[string]float64
}

// New creates a new ScopePro metrics collector.
func New(namespace string, devices []string, executor Executor, logger zerolog.Logger) *Collector {
	return &Collector{
		devices:   devices,
		namespace: namespace,
		executor:  executor,
		logger:    logger,
		driveInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "drive_info"),
			"Drive identification information.",
			[]string{"device", "type", "model", "firmware", "serial", "interface", "manufacturer", "product", "revision"}, nil,
		),
		healthPct: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "health_percentage"),
			"Drive health percentage.",
			[]string{"device"}, nil,
		),
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "scrape_errors_total"),
			"Total number of scrape errors per device.",
			[]string{"device"}, nil,
		),
		buildInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "build_info"),
			"Build information.",
			[]string{"version"}, nil,
		),
		errorCounts: make(map[string]float64),
	}
}

// Describe sends metric descriptors to the channel.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.driveInfo
	ch <- c.healthPct
	ch <- c.scrapeErrors
	ch <- c.buildInfo
}

// Version is set at build time via ldflags.
var Version = "dev"

// Collect gathers metrics from all configured devices.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.buildInfo, prometheus.GaugeValue, 1, Version)

	ctx := context.Background()

	for _, device := range c.devices {
		c.collectDevice(ctx, device, ch)
	}
}

func (c *Collector) collectDevice(ctx context.Context, device string, ch chan<- prometheus.Metric) {
	// Drive info
	info, err := c.executor.DriveInfoQuery(ctx, device)
	if err != nil {
		c.logger.Error().Err(err).Str("device", device).Msg("failed to collect drive info")
		c.incError(device)
	} else {
		ch <- prometheus.MustNewConstMetric(
			c.driveInfo, prometheus.GaugeValue, 1,
			device, info.Type, info.Model, info.Firmware, info.Serial,
			info.Interface, info.Manufacturer, info.Product, info.Revision,
		)
	}

	// Health
	health, err := c.executor.Health(ctx, device)
	if err != nil {
		c.logger.Error().Err(err).Str("device", device).Msg("failed to collect health")
		c.incError(device)
	} else {
		ch <- prometheus.MustNewConstMetric(c.healthPct, prometheus.GaugeValue, health, device)
	}

	// SMART attributes
	attrs, err := c.executor.SmartInfo(ctx, device)
	if err != nil {
		c.logger.Error().Err(err).Str("device", device).Msg("failed to collect SMART info")
		c.incError(device)
	} else {
		for name, val := range attrs {
			desc := prometheus.NewDesc(
				prometheus.BuildFQName(c.namespace, "smart", name),
				"S.M.A.R.T attribute: "+name,
				[]string{"device"}, nil,
			)
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, val, device)
		}
	}

	// Emit scrape errors
	c.mu.Lock()
	count := c.errorCounts[device]
	c.mu.Unlock()
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, count, device)
}

func (c *Collector) incError(device string) {
	c.mu.Lock()
	c.errorCounts[device]++
	c.mu.Unlock()
}
