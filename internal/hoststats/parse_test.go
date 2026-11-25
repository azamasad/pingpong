package hoststats

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseProcStat(t *testing.T) {
	text := "cpu  100 0 50 800 50 0 0 0 0 0\ncpu0 1 0 0 1 0 0 0 0 0 0\n"
	total, idle, err := ParseProcStat(text)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1000 || idle != 850 {
		t.Fatalf("total %d idle %d", total, idle)
	}
}

func TestParseMeminfo(t *testing.T) {
	text := "MemTotal: 1000 kB\nMemFree: 100 kB\nMemAvailable: 400 kB\n"
	total, used, err := ParseMeminfo(text)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1000*1024 || used != 600*1024 {
		t.Fatalf("total %d used %d", total, used)
	}
}

func TestParseProcNetSkipsLoopback(t *testing.T) {
	text := " face |bytes packets\n lo: 9 0 0 0 0 0 0 0 8 0 0 0 0 0 0 0\n eno1: 1000 0 0 0 0 0 0 0 250 0 0 0 0 0 0 0\n"
	rx, tx := ParseProcNet(text)
	if rx != 1000 || tx != 250 {
		t.Fatalf("rx %d tx %d", rx, tx)
	}
}

func TestParseNetstatIBFromTheRight(t *testing.T) {
	text := "" +
		"Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll\n" +
		"lo0 16384 <Link#1> 10 0 999 10 0 888 0\n" +
		"en0 1500 <Link#4> aa:bb 10 0 5000 8 0 2000 0\n"
	rx, tx := ParseNetstatIB(text)
	if rx != 5000 || tx != 2000 {
		t.Fatalf("rx %d tx %d", rx, tx)
	}
}

func TestParseVMStatAndLoad(t *testing.T) {
	text := "Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages active: 10.\nPages wired down: 5.\nPages occupied by compressor: 5.\n"
	used, ok := ParseVMStat(text, 16384*100)
	if !ok || used != 16384*20 {
		t.Fatalf("used %d ok %v", used, ok)
	}
	load, ok := ParseLoadAvg("{ 1.50 1.20 1.00 }")
	if !ok || load != 1.5 {
		t.Fatalf("load %v", load)
	}
}

func TestSamplerRates(t *testing.T) {
	n := 0
	s := &Sampler{
		now: func() time.Time {
			return time.Unix(int64(n*5), 0)
		},
		read: func() (raw, error) {
			n++
			cpu := uint64(n * 100)
			idle := uint64(n * 50)
			load := 0.5
			return raw{CPUTotal: cpu, CPUIdle: idle, RAMTotal: 1000, RAMUsed: 250, NetRx: uint64(n * 5000), NetTx: uint64(n * 1000), Load1: &load}, nil
		},
	}
	first := s.Sample()
	if !first.OK || first.CPUPercent != nil || first.NetRxBps != nil {
		t.Fatalf("first should have no rates: %+v", first)
	}
	if first.RAMPercent == nil || math.Abs(*first.RAMPercent-25) > 0.01 {
		t.Fatalf("ram %+v", first.RAMPercent)
	}
	second := s.Sample()
	if second.CPUPercent == nil || math.Abs(*second.CPUPercent-50) > 0.01 {
		t.Fatalf("cpu %+v", second.CPUPercent)
	}
	if second.NetRxBps == nil || math.Abs(*second.NetRxBps-1000) > 0.01 {
		t.Fatalf("rx %+v", second.NetRxBps)
	}
}

func TestTempFromSysfs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sys/class/hwmon/hwmon0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "name"), []byte("k10temp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "temp1_input"), []byte("54500\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := tempFromSysfs(root)
	if c == nil || math.Abs(*c-54.5) > 0.01 {
		t.Fatalf("temp %v", c)
	}
}

func TestCheckListen(t *testing.T) {
	if err := CheckListen("127.0.0.1:8788"); err != nil {
		t.Fatal(err)
	}
	if err := CheckListen("0.0.0.0:8788"); err == nil {
		t.Fatal("all interfaces should be refused")
	}
	if err := CheckListen("100.1.2.3:8788"); err != nil {
		t.Fatal(err)
	}
}
