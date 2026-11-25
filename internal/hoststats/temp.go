package hoststats

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// tempFromSysfs returns Celsius from a known CPU sensor, or nil when none is exposed.
// root is empty on the real machine. Tests pass a fake tree.
func tempFromSysfs(root string) *float64 {
	prefer := map[string]struct{}{
		"k10temp": {}, "coretemp": {}, "cpu_thermal": {}, "soc_thermal": {}, "zenpower": {},
	}
	base := filepath.Join(root, "sys/class/hwmon")
	entries, err := os.ReadDir(base)
	if err == nil {
		for _, e := range entries {
			dir := filepath.Join(base, e.Name())
			nameb, err := os.ReadFile(filepath.Join(dir, "name"))
			if err != nil {
				continue
			}
			name := strings.TrimSpace(string(nameb))
			if _, ok := prefer[name]; !ok {
				continue
			}
			if c := milliFile(filepath.Join(dir, "temp1_input")); c != nil {
				return c
			}
		}
	}
	zones := filepath.Join(root, "sys/class/thermal")
	zents, err := os.ReadDir(zones)
	if err != nil {
		return nil
	}
	for _, e := range zents {
		if !strings.HasPrefix(e.Name(), "thermal_zone") {
			continue
		}
		dir := filepath.Join(zones, e.Name())
		tb, err := os.ReadFile(filepath.Join(dir, "type"))
		if err != nil {
			continue
		}
		kind := strings.TrimSpace(string(tb))
		if kind != "x86_pkg_temp" && kind != "cpu-thermal" && kind != "k10temp" {
			continue
		}
		if c := milliFile(filepath.Join(dir, "temp")); c != nil {
			return c
		}
	}
	return nil
}

func milliFile(path string) *float64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	if err != nil {
		return nil
	}
	c := n / 1000
	return &c
}
