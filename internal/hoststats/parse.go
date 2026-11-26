package hoststats

import (
	"strconv"
	"strings"
)

// ParseProcStat reads the aggregate "cpu " line. Idle includes iowait.
func ParseProcStat(text string) (total, idle uint64, err error) {
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || f[0] != "cpu" {
			continue
		}
		var sum uint64
		nums := make([]uint64, 0, len(f)-1)
		for _, s := range f[1:] {
			n, e := strconv.ParseUint(s, 10, 64)
			if e != nil {
				return 0, 0, e
			}
			nums = append(nums, n)
			sum += n
		}
		idle = nums[3]
		if len(nums) > 4 {
			idle += nums[4]
		}
		return sum, idle, nil
	}
	return 0, 0, strconv.ErrSyntax
}

// ParseMeminfo returns total and used bytes. Used is total minus available.
func ParseMeminfo(text string) (total, used uint64, err error) {
	var memTotal, memAvail uint64
	var sawTotal, sawAvail bool
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		n, e := strconv.ParseUint(f[1], 10, 64)
		if e != nil {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			memTotal = n * 1024
			sawTotal = true
		case "MemAvailable:":
			memAvail = n * 1024
			sawAvail = true
		}
	}
	if !sawTotal || !sawAvail || memTotal == 0 {
		return 0, 0, strconv.ErrSyntax
	}
	if memAvail > memTotal {
		memAvail = memTotal
	}
	return memTotal, memTotal - memAvail, nil
}

// ParseProcNet sums receive and transmit bytes on every interface except lo.
func ParseProcNet(text string) (rx, tx uint64) {
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		if name == "" || name == "lo" || name == "face" {
			continue
		}
		f := strings.Fields(parts[1])
		if len(f) < 9 {
			continue
		}
		rin, e1 := strconv.ParseUint(f[0], 10, 64)
		tout, e2 := strconv.ParseUint(f[8], 10, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		rx += rin
		tx += tout
	}
	return rx, tx
}

// ParseLoadAvg reads the first field of /proc/loadavg or a sysctl load string.
func ParseLoadAvg(text string) (float64, bool) {
	text = strings.Trim(text, " {}\t\r\n")
	f := strings.Fields(text)
	if len(f) == 0 {
		return 0, false
	}
	n, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// ParseCPTime reads a space-separated tick list. The fourth number is idle.
func ParseCPTime(text string) (total, idle uint64, err error) {
	f := strings.Fields(text)
	if len(f) < 4 {
		return 0, 0, strconv.ErrSyntax
	}
	var nums []uint64
	for _, s := range f {
		n, e := strconv.ParseUint(s, 10, 64)
		if e != nil {
			return 0, 0, e
		}
		nums = append(nums, n)
		total += n
	}
	return total, nums[3], nil
}

// ParseVMStat estimates used bytes from vm_stat text and a total from hw.memsize.
// Used is active + wired + compressor, clamped to the total.
func ParseVMStat(text string, memTotal uint64) (uint64, bool) {
	page := uint64(4096)
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "page size of") {
			f := strings.Fields(line)
			for i, s := range f {
				if s == "of" && i+1 < len(f) {
					n, err := strconv.ParseUint(f[i+1], 10, 64)
					if err == nil && n > 0 {
						page = n
					}
				}
			}
		}
	}
	pages := map[string]uint64{}
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		num := strings.Trim(strings.TrimSpace(parts[1]), ".")
		fields := strings.Fields(num)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		pages[key] = n
	}
	if len(pages) == 0 {
		return 0, false
	}
	usedPages := pages["Pages active"] + pages["Pages wired down"] + pages["Pages occupied by compressor"]
	used := usedPages * page
	if memTotal > 0 && used > memTotal {
		used = memTotal
	}
	return used, true
}

// ParseNetstatIB sums Ibytes and Obytes from `netstat -ibn` link rows, skipping loopback.
// Counts are taken from the right so a missing address column does not shift them.
func ParseNetstatIB(text string) (rx, tx uint64) {
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "<Link") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 8 {
			continue
		}
		name := f[0]
		if strings.HasPrefix(name, "lo") {
			continue
		}
		rin, e1 := strconv.ParseUint(f[len(f)-5], 10, 64)
		tout, e2 := strconv.ParseUint(f[len(f)-2], 10, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		rx += rin
		tx += tout
	}
	return rx, tx
}
