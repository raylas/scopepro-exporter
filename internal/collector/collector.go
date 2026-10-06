package collector

import (
	"context"
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/raylas/scopepro-exporter/internal/scopepro"
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
	version   string
	executor  Executor
	logger    *slog.Logger

	driveInfo    *prometheus.Desc
	healthPct    *prometheus.Desc
	scrapeErrors *prometheus.Desc
	buildInfo    *prometheus.Desc

	// mu serializes scrapes so concurrent /metrics requests never run
	// multiple scopepro processes against the same device at once.
	// It also guards the fields below.
	mu          sync.Mutex
	smartDescs  map[string]*prometheus.Desc
	errorCounts map[string]float64
}

// New creates a new ScopePro metrics collector.
func New(namespace, version string, devices []string, executor Executor, logger *slog.Logger) *Collector {
	return &Collector{
		devices:   devices,
		namespace: namespace,
		version:   version,
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
		smartDescs:  make(map[string]*prometheus.Desc),
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

// Collect gathers metrics from all configured devices.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch <- prometheus.MustNewConstMetric(c.buildInfo, prometheus.GaugeValue, 1, c.version)

	ctx := context.Background()
	for _, device := range c.devices {
		c.collectDevice(ctx, device, ch)
	}
}

func (c *Collector) collectDevice(ctx context.Context, device string, ch chan<- prometheus.Metric) {
	log := c.logger.With("device", device)

	info, err := c.executor.DriveInfoQuery(ctx, device)
	if err != nil {
		log.Error("failed to collect drive info", "err", err)
		c.errorCounts[device]++
	} else {
		ch <- prometheus.MustNewConstMetric(
			c.driveInfo, prometheus.GaugeValue, 1,
			device, info.Type, info.Model, info.Firmware, info.Serial,
			info.Interface, info.Manufacturer, info.Product, info.Revision,
		)
	}

	health, err := c.executor.Health(ctx, device)
	if err != nil {
		log.Error("failed to collect health", "err", err)
		c.errorCounts[device]++
	} else {
		ch <- prometheus.MustNewConstMetric(c.healthPct, prometheus.GaugeValue, health, device)
	}

	attrs, err := c.executor.SmartInfo(ctx, device)
	if err != nil {
		log.Error("failed to collect SMART info", "err", err)
		c.errorCounts[device]++
	} else {
		for name, val := range attrs {
			ch <- prometheus.MustNewConstMetric(c.smartDesc(name), prometheus.GaugeValue, val, device)
		}
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, c.errorCounts[device], device)
}

// smartDesc returns the cached descriptor for a SMART attribute, creating it on first use.
func (c *Collector) smartDesc(name string) *prometheus.Desc {
	desc, ok := c.smartDescs[name]
	if !ok {
		desc = prometheus.NewDesc(
			prometheus.BuildFQName(c.namespace, "smart", name),
			"S.M.A.R.T attribute: "+name,
			[]string{"device"}, nil,
		)
		c.smartDescs[name] = desc
	}
	return desc
}
