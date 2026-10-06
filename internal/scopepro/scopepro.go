package scopepro

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DriveInfo holds parsed drive identification data.
type DriveInfo struct {
	Type      string // "SSD" or "SD"
	Model     string
	Firmware  string
	Serial    string
	Interface string
	// SD-specific fields
	Manufacturer string
	Product      string
	Revision     string
}

// ScopePro executes the scopepro CLI and parses its output.
type ScopePro struct {
	path    string
	timeout time.Duration
}

// New creates a ScopePro executor. If path is empty, "scopepro" is used.
// Each CLI invocation is killed after timeout; zero means no timeout.
func New(path string, timeout time.Duration) *ScopePro {
	if path == "" {
		path = "scopepro"
	}
	return &ScopePro{path: path, timeout: timeout}
}

func (s *ScopePro) exec(ctx context.Context, args ...string) (string, error) {
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, s.path, args...)
	// Stop waiting on stdout shortly after a timeout kill, even if a child
	// process inherited the pipe.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("scopepro %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// DriveInfoQuery executes scopepro -id and parses the output.
func (s *ScopePro) DriveInfoQuery(ctx context.Context, device string) (*DriveInfo, error) {
	out, err := s.exec(ctx, "-id", device)
	if err != nil {
		return nil, err
	}
	return ParseDriveInfo(out)
}

// SmartInfo executes scopepro -smart and parses S.M.A.R.T attributes.
func (s *ScopePro) SmartInfo(ctx context.Context, device string) (map[string]float64, error) {
	out, err := s.exec(ctx, "-smart", device)
	if err != nil {
		return nil, err
	}
	return ParseSmartInfo(out)
}

// Health executes scopepro -health and parses the health percentage.
func (s *ScopePro) Health(ctx context.Context, device string) (float64, error) {
	out, err := s.exec(ctx, "-health", device)
	if err != nil {
		return 0, err
	}
	return ParseHealth(out)
}

// ParseDriveInfo parses the output of scopepro -id.
func ParseDriveInfo(output string) (*DriveInfo, error) {
	info := &DriveInfo{}

	for _, line := range strings.Split(output, "\n") {
		key, val, ok := cutField(line)
		if !ok {
			continue
		}

		switch strings.ToLower(key) {
		case "model":
			info.Type = "SSD"
			info.Model = val
		case "fw version":
			info.Firmware = val
		case "serial no":
			info.Serial = val
		case "support interface":
			info.Interface = val
		case "type":
			info.Type = val
		case "manufacturer id":
			info.Manufacturer = val
		case "product name":
			info.Product = val
		case "product revision":
			info.Revision = val
		}
	}

	if info.Type == "" {
		return nil, fmt.Errorf("could not determine device type from output")
	}
	return info, nil
}

// ParseSmartInfo parses the output of scopepro -smart into attribute name→value pairs.
// Non-numeric attributes are skipped. SD cards print "<name>: <value>" lines;
// SSDs print "<id> <name> <value>" lines.
func ParseSmartInfo(output string) (map[string]float64, error) {
	attrs := make(map[string]float64)

	for _, line := range strings.Split(output, "\n") {
		if key, val, ok := cutField(line); ok {
			if f, err := parseValue(val); err == nil {
				attrs[NormalizeName(key)] = f
			}
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 || strings.HasPrefix(fields[0], "-") {
			continue
		}
		if f, err := parseValue(fields[len(fields)-1]); err == nil {
			attrs[NormalizeName(strings.Join(fields[1:len(fields)-1], " "))] = f
		}
	}

	if len(attrs) == 0 {
		return nil, fmt.Errorf("no S.M.A.R.T attributes found in output")
	}
	return attrs, nil
}

// ParseHealth parses the output of scopepro -health.
func ParseHealth(output string) (float64, error) {
	for _, line := range strings.Split(output, "\n") {
		key, val, ok := cutField(line)
		if ok && strings.ToLower(key) == "health percentage" {
			return parseValue(val)
		}
	}
	return 0, fmt.Errorf("health percentage not found in output")
}

// NormalizeName converts an attribute name to a valid Prometheus metric name
// fragment: lowercase ASCII snake_case.
func NormalizeName(s string) string {
	var b strings.Builder
	prev := '_'
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prev = r
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 'a' - 'A')
			prev = r
		default:
			if prev != '_' {
				b.WriteByte('_')
				prev = '_'
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
}

// cutField splits a "key :value" or "key: value" line into trimmed key and value.
func cutField(line string) (string, string, bool) {
	key, val, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(val), true
}

// parseValue parses a numeric value, tolerating a trailing percent sign.
func parseValue(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
}
