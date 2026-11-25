//go:build darwin

package hoststats

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

func readHost() (raw, error) {
	cp, err := exec.Command("sysctl", "-n", "kern.cp_time").Output()
	if err != nil {
		return raw{}, err
	}
	total, idle, err := ParseCPTime(string(cp))
	if err != nil {
		return raw{}, err
	}
	mem, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return raw{}, err
	}
	ramTotal, err := strconv.ParseUint(strings.TrimSpace(string(mem)), 10, 64)
	if err != nil {
		return raw{}, err
	}
	vm, err := exec.Command("vm_stat").Output()
	if err != nil {
		return raw{}, err
	}
	ramUsed, ok := ParseVMStat(string(vm), ramTotal)
	if !ok {
		return raw{}, errors.New("vm_stat")
	}
	netb, err := exec.Command("netstat", "-ibn").Output()
	if err != nil {
		return raw{}, err
	}
	rx, tx := ParseNetstatIB(string(netb))
	var load *float64
	if b, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
		if n, ok := ParseLoadAvg(string(b)); ok {
			load = &n
		}
	}
	var thermal *int
	if b, err := exec.Command("sysctl", "-n", "machdep.xcpm.cpu_thermal_level").Output(); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(string(b))); e == nil {
			thermal = &n
		}
	}
	return raw{
		CPUTotal: total,
		CPUIdle:  idle,
		RAMTotal: ramTotal,
		RAMUsed:  ramUsed,
		NetRx:    rx,
		NetTx:    tx,
		Thermal:  thermal,
		Load1:    load,
	}, nil
}
