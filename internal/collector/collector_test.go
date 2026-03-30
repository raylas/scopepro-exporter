package collector

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/raylas/scopepro-exporter/internal/scopepro"
	"github.com/rs/zerolog"
)

type mockExecutor struct {
	driveInfo *scopepro.DriveInfo
	driveErr  error
	smart     map[string]float64
	smartErr  error
	health    float64
	healthErr error
}

func (m *mockExecutor) DriveInfoQuery(_ context.Context, _ string) (*scopepro.DriveInfo, error) {
	return m.driveInfo, m.driveErr
}

func (m *mockExecutor) SmartInfo(_ context.Context, _ string) (map[string]float64, error) {
	return m.smart, m.smartErr
}

func (m *mockExecutor) Health(_ context.Context, _ string) (float64, error) {
	return m.health, m.healthErr
}

func TestCollect(t *testing.T) {
	tests := []struct {
		name     string
		executor *mockExecutor
		devices  []string
		check    func(t *testing.T, reg *prometheus.Registry)
	}{
		{
			name:    "SSD device metrics",
			devices: []string{"/dev/sda"},
			executor: &mockExecutor{
				driveInfo: &scopepro.DriveInfo{
					Device:    "/dev/sda",
					Type:      "SSD",
					Model:     "TS512GSSD470K",
					Firmware:  "22Z2UCFS",
					Serial:    "H735380001",
					Interface: "SATA",
				},
				health: 100,
				smart: map[string]float64{
					"power_on_hours": 5520,
					"remain_life_percentage": 100,
				},
			},
			check: func(t *testing.T, reg *prometheus.Registry) {
				expected := `
# HELP scopepro_health_percentage Drive health percentage.
# TYPE scopepro_health_percentage gauge
scopepro_health_percentage{device="/dev/sda"} 100
`
				if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "scopepro_health_percentage"); err != nil {
					t.Errorf("health metric: %v", err)
				}
			},
		},
		{
			name:    "device error increments scrape_errors",
			devices: []string{"/dev/sda"},
			executor: &mockExecutor{
				driveErr:  fmt.Errorf("device not found"),
				healthErr: fmt.Errorf("device not found"),
				smartErr:  fmt.Errorf("device not found"),
			},
			check: func(t *testing.T, reg *prometheus.Registry) {
				expected := `
# HELP scopepro_scrape_errors_total Total number of scrape errors per device.
# TYPE scopepro_scrape_errors_total counter
scopepro_scrape_errors_total{device="/dev/sda"} 3
`
				if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "scopepro_scrape_errors_total"); err != nil {
					t.Errorf("scrape errors metric: %v", err)
				}
			},
		},
		{
			name:    "build info always present",
			devices: []string{"/dev/sda"},
			executor: &mockExecutor{
				driveInfo: &scopepro.DriveInfo{Device: "/dev/sda", Type: "SSD"},
				health:    95,
				smart:     map[string]float64{},
			},
			check: func(t *testing.T, reg *prometheus.Registry) {
				expected := `
# HELP scopepro_build_info Build information.
# TYPE scopepro_build_info gauge
scopepro_build_info{version="dev"} 1
`
				if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "scopepro_build_info"); err != nil {
					t.Errorf("build info metric: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zerolog.Nop()
			c := New("scopepro", tt.devices, tt.executor, logger)

			reg := prometheus.NewRegistry()
			reg.MustRegister(c)

			tt.check(t, reg)
		})
	}
}
