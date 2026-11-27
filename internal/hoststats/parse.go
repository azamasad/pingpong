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

// ParseTopCPU reads the "CPU usage" line from `top -l 1`. It sums every
// percentage except idle. The rest of the text is ignored.
func ParseTopCPU(text string) (float64, bool) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "CPU usage:") {
			continue
		}
		var sum float64
		var n int
		rest := line
		for {
			i := strings.Index(rest, "%")
			if i < 0 {
				break
			}
			j := i - 1
			for j >= 0 && (rest[j] == '.' || (rest[j] >= '0' && rest[j] <= '9')) {
				j--
			}
			num := rest[j+1 : i]
			word := strings.TrimSpace(rest[i+1:])
			if k := strings.IndexAny(word, " ,"); k >= 0 {
				word = word[:k]
			}
			v, err := strconv.ParseFloat(num, 64)
			if err == nil && word != "" && word != "idle" {
				sum += v
				n++
			}
			rest = rest[i+1:]
		}
		if n == 0 {
			return 0, false
		}
		if sum < 0 {
			sum = 0
		}
		if sum > 100 {
			sum = 100
		}
		return sum, true
	}
	return 0, false
}

// assembleDarwin builds one reading from command output. An empty string means
// that command failed. A missing CPU tick counter must not drop RAM, network,
// load or temperature.
func assembleDarwin(cp, mem, vm, netstatText, load, thermal, top string) (raw, error) {
	var r raw
	got := false
	if total, idle, err := ParseCPTime(cp); err == nil && total > 0 {
		r.CPUTotal = total
		r.CPUIdle = idle
		got = true
	} else if pct, ok := ParseTopCPU(top); ok {
		r.CPUPercent = &pct
		got = true
	}
	if n, err := strconv.ParseUint(strings.TrimSpace(mem), 10, 64); err == nil && n > 0 {
		r.RAMTotal = n
		if used, ok := ParseVMStat(vm, n); ok {
			r.RAMUsed = used
		}
		got = true
	}
	if strings.TrimSpace(netstatText) != "" {
		r.NetRx, r.NetTx = ParseNetstatIB(netstatText)
		got = true
	}
	if n, ok := ParseLoadAvg(load); ok {
		r.Load1 = &n
		got = true
	}
	if n, err := strconv.Atoi(strings.TrimSpace(thermal)); err == nil {
		r.Thermal = &n
		got = true
	}
	if !got {
		return raw{}, strconv.ErrSyntax
	}
	return r, nil
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
