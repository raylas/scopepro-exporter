package scopepro

import (
	"math"
	"testing"
)

const ssdDriveInfoOutput = `
---------- Disk Information ----------

Model :TS512GSSD470K

FW Version :22Z2UCFS

Serial No :H735380001

Support Interface :SATA
`

const sdDriveInfoOutput = `
type :SD

Manufacturer ID :0x74 Transcend

Product Name :USDU1

Product Revision :0x20

Manufacture Date :2024 sep

CRC checksum :0x00
`

const ssdSmartOutput = `
---------------- S.M.A.R.T Information ----------------

01 Read Error Rate 0

05 Reallocated Sectors Count 1

09 Power-On Hours 5520

OC Power Cycle Count 664

A0 Uncorrectable sectors count when read/write 1

A1 Number of Valid Spare Blocks 108

A3 Number of Initial Invalid Blocks 47

A4 Total Erase Count 55937

A5 Maximum Erase Count 78

A6 Minimum Erase Count 9

A7 Average Erase Count 27

A8 Max Erase Count of Spec 3000

A9 Remain Life (percentage) 100

AF Worst Component Program Fail Count 0

BO Worst Component Erase Fail Count 0

B1 Total Wear Level Count 0

B2 Grown Bad Block Count 0

B5 Total Program Fail Count 0

B6 Total Erase Fail Count 0

CO Sudden Power Count 164

C2 Enclosure Temperature 26

C3 Hardware ECC Recovered 870

C4 Reallocation Event Count 2

C5 Current Pending Sector Count 0

C6 Reported Uncorrectable Errors 1

C7 CRC Error Count 8

E8 Available Reserved Space 100

F1 Host 32MB/unit Written (TLC) 733037

F2 Host 32MB/unit Read (TLC) 766228

F5 NAND 32MB/unit Written (TLC) 894992
`

const sdSmartOutput = `
---------------- S.M.A.R.T Information ----------------

Card Marker: Transcend

Bus Width: 4 bit Width

Secured Mode: Not in the secured mode

Speed Class: Class 10

UHS Speed Grade: 30MB/s and above

New Bad block Count: 0

Spare Block: 0

Min Erase Count: 0

Max Erase count: 1

Total Erase Count: 3

Avg. Erase Count: 0

NAND P/E Cycle: 3000

Card Life: 100%

Current SD Card Speed Mode: SDR104

Total Write CRC Count: 0

Power On/Off Count: 9

NAND Flash ID: 45-48-98-03-76-66

SMI SD Controller P/N: SM2706

SD Firmware Version: V0217

Abnormal Power Detect: 0
`

const healthOutput = `
---------------- Health Information ----------------

Health Percentage: 100%
`

func TestParseDriveInfo(t *testing.T) {
	tests := []struct {
		name   string
		output string
		device string
		want   DriveInfo
	}{
		{
			name:   "SSD drive info",
			output: ssdDriveInfoOutput,
			device: "/dev/sda",
			want: DriveInfo{
				Device:    "/dev/sda",
				Type:      "SSD",
				Model:     "TS512GSSD470K",
				Firmware:  "22Z2UCFS",
				Serial:    "H735380001",
				Interface: "SATA",
			},
		},
		{
			name:   "SD card drive info",
			output: sdDriveInfoOutput,
			device: "/dev/sdb",
			want: DriveInfo{
				Device:       "/dev/sdb",
				Type:         "SD",
				Manufacturer: "0x74 Transcend",
				Product:      "USDU1",
				Revision:     "0x20",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDriveInfo(tt.output, tt.device)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *got != tt.want {
				t.Errorf("got %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestParseSmartInfo(t *testing.T) {
	tests := []struct {
		name   string
		output string
		check  map[string]float64 // subset of expected attributes
	}{
		{
			name:   "SSD SMART attributes",
			output: ssdSmartOutput,
			check: map[string]float64{
				"read_error_rate":        0,
				"reallocated_sectors_count": 1,
				"power_on_hours":         5520,
				"remain_life_percentage": 100,
				"enclosure_temperature":  26,
				"crc_error_count":        8,
			},
		},
		{
			name:   "SD card SMART attributes",
			output: sdSmartOutput,
			check: map[string]float64{
				"new_bad_block_count":   0,
				"spare_block":           0,
				"min_erase_count":       0,
				"max_erase_count":       1,
				"total_erase_count":     3,
				"nand_p_e_cycle":        3000,
				"card_life":             100,
				"total_write_crc_count": 0,
				"power_on_off_count":    9,
				"abnormal_power_detect": 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSmartInfo(tt.output)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, want := range tt.check {
				v, ok := got[k]
				if !ok {
					t.Errorf("missing attribute %q", k)
					continue
				}
				if math.Abs(v-want) > 0.001 {
					t.Errorf("attribute %q: got %v, want %v", k, v, want)
				}
			}
		})
	}
}

func TestParseHealth(t *testing.T) {
	got, err := ParseHealth(healthOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 100 {
		t.Errorf("got %v, want 100", got)
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Power-On Hours", "power_on_hours"},
		{"Remain Life (percentage)", "remain_life_percentage"},
		{"Card Life", "card_life"},
		{"CRC Error Count", "crc_error_count"},
		{"Avg. Erase Count", "avg_erase_count"},
		{"NAND P/E Cycle", "nand_p_e_cycle"},
		{"Host 32MB/unit Written (TLC)", "host_32mb_unit_written_tlc"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeName(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
