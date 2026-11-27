//go:build darwin

package hoststats

import (
	"context"
	"os"
	"os/exec"
	"time"
)

func readHost() (raw, error) {
	cp := commandText("sysctl", "-n", "kern.cp_time")
	mem := commandText("sysctl", "-n", "hw.memsize")
	vm := commandText("vm_stat")
	netb := commandText("netstat", "-ibn")
	load := commandText("sysctl", "-n", "vm.loadavg")
	thermal := commandText("sysctl", "-n", "machdep.xcpm.cpu_thermal_level")
	top := ""
	if total, _, err := ParseCPTime(cp); err != nil || total == 0 {
		// Apple silicon does not publish kern.cp_time. The summary line is
		// the CPU percent. It is parsed and discarded, never logged.
		top = commandText("top", "-l", "1", "-n", "0")
		if _, ok := ParseTopCPU(top); !ok {
			top = commandText("top", "-l", "1", "-n", "1")
		}
	}
	return assembleDarwin(cp, mem, vm, netb, load, thermal, top)
}

// commandText runs a short read-only command. Failure, including a missing
// sysctl, is an empty string. The output is never logged.
func commandText(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(b)
}
