// Package metrics reads the local machine's resource usage for the exporter endpoints.
package metrics

import (
	"math"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

type Reading struct {
	Unit  string  `json:"unit,omitempty"`
	Value float64 `json:"value"`
	Total float64 `json:"total"`
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// Processor returns the summed per-core usage, where total is cores*100
// (e.g. 120 of 400 on a 4-core machine).
func Processor() (Reading, error) {
	per, err := cpu.Percent(500*time.Millisecond, true)
	if err != nil {
		return Reading{}, err
	}
	var sum float64
	for _, p := range per {
		sum += p
	}
	return Reading{Value: round2(sum), Total: float64(len(per) * 100)}, nil
}

// Memory returns used memory (total - available) in MB.
func Memory() (Reading, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return Reading{}, err
	}
	const mb = 1024 * 1024
	return Reading{
		Unit:  "MB",
		Value: math.Round(float64(vm.Total-vm.Available) / mb),
		Total: math.Round(float64(vm.Total) / mb),
	}, nil
}

// DiskFree returns free space on the given mount path in GB.
func DiskFree(path string) (Reading, error) {
	u, err := disk.Usage(path)
	if err != nil {
		return Reading{}, err
	}
	const gb = 1024 * 1024 * 1024
	return Reading{
		Unit:  "GB",
		Value: round2(float64(u.Free) / gb),
		Total: round2(float64(u.Total) / gb),
	}, nil
}
