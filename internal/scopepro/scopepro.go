package scopepro

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"unicode"
)

// DriveInfo holds parsed drive identification data.
type DriveInfo struct {
	Device    string
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
	path string
}

// New creates a ScopePro executor. If path is empty, "scopepro" is used.
func New(path string) *ScopePro {
	if path == "" {
		path = "scopepro"
	}
	return &ScopePro{path: path}
}

func (s *ScopePro) exec(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, s.path, args...)
	out, err := cmd.Output()
	if err != nil {
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
	return ParseDriveInfo(out, device)
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
func ParseDriveInfo(output, device string) (*DriveInfo, error) {
	info := &DriveInfo{Device: device}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}

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
func ParseSmartInfo(output string) (map[string]float64, error) {
	attrs := make(map[string]float64)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "S.M.A.R.T") {
			continue
		}

		// Try SSD format: "<hex_id> <name> <value>" — hex ID followed by a space
		if len(line) >= 3 && isHexByte(line[:2]) && line[2] == ' ' {
			name, val := parseSSDSmartLine(line)
			if name != "" {
				attrs[NormalizeName(name)] = val
			}
			continue
		}

		// Try SD format: "<name>: <value>"
		key, val, ok := cutField(line)
		if !ok {
			continue
		}
		val = strings.TrimSuffix(val, "%")
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			continue // skip non-numeric values
		}
		attrs[NormalizeName(key)] = f
	}

	return attrs, nil
}

// ParseHealth parses the output of scopepro -health.
func ParseHealth(output string) (float64, error) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		key, val, ok := cutField(line)
		if !ok {
			continue
		}
		if strings.ToLower(key) == "health percentage" {
			val = strings.TrimSuffix(val, "%")
			return strconv.ParseFloat(val, 64)
		}
	}
	return 0, fmt.Errorf("health percentage not found in output")
}

// NormalizeName converts an attribute name to lowercase snake_case.
func NormalizeName(s string) string {
	var b strings.Builder
	prev := '_'
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
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

// cutField splits a line on ":" or " :" and returns trimmed key/value.
func cutField(line string) (string, string, bool) {
	// Try " :" separator first (ScopePro common format)
	if key, val, ok := strings.Cut(line, " :"); ok {
		return strings.TrimSpace(key), strings.TrimSpace(val), true
	}
	// Try ":" separator (SD card format)
	if key, val, ok := strings.Cut(line, ":"); ok {
		return strings.TrimSpace(key), strings.TrimSpace(val), true
	}
	return "", "", false
}

// isHexByte checks if s looks like a two-character hex byte (e.g., "01", "A9", "F1").
func isHexByte(s string) bool {
	if len(s) != 2 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// parseSSDSmartLine parses a line like "09 Power-On Hours 5520".
// Returns the attribute name and its numeric value.
func parseSSDSmartLine(line string) (string, float64) {
	// Skip the hex ID prefix (2 chars + space)
	rest := strings.TrimSpace(line[2:])
	// The value is the last whitespace-separated field
	lastSpace := strings.LastIndexFunc(rest, unicode.IsSpace)
	if lastSpace < 0 {
		return "", 0
	}
	name := strings.TrimSpace(rest[:lastSpace])
	valStr := strings.TrimSpace(rest[lastSpace+1:])
	f, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return "", 0
	}
	return name, f
}
