//go:build linux

package hoststats

import (
	"os"
)

func readHost() (raw, error) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return raw{}, err
	}
	total, idle, err := ParseProcStat(string(stat))
	if err != nil {
		return raw{}, err
	}
	mem, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return raw{}, err
	}
	ramTotal, ramUsed, err := ParseMeminfo(string(mem))
	if err != nil {
		return raw{}, err
	}
	netb, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return raw{}, err
	}
	rx, tx := ParseProcNet(string(netb))
	var load *float64
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if n, ok := ParseLoadAvg(string(b)); ok {
			load = &n
		}
	}
	return raw{
		CPUTotal: total,
		CPUIdle:  idle,
		RAMTotal: ramTotal,
		RAMUsed:  ramUsed,
		NetRx:    rx,
		NetTx:    tx,
		TempC:    tempFromSysfs(""),
		Load1:    load,
	}, nil
}
