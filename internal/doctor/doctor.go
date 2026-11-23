package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/ollama"
	"github.com/azamasad/pingpong/internal/route"
)

// Status values for a check.
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
	StatusInfo = "info"
)

// Check is one doctor result.
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Options controls doctor behaviour.
type Options struct {
	Chat bool
	// DiskFree overrides free-space lookup (tests).
	DiskFree func(path string) (uint64, error)
}

// Run performs all non-chat checks on a loaded config.
func Run(ctx context.Context, cfg *config.Config, loadWarnings []string, opt Options) []Check {
	var out []Check
	for _, w := range loadWarnings {
		out = append(out, Check{
			Name: "config.warning", Status: StatusWarn, Message: w,
			Hint: "review config; routing may differ from what you expect",
		})
	}
	out = append(out, storageChecks(cfg, opt)...)
	for _, m := range cfg.Machines {
		out = append(out, machineChecks(ctx, cfg, m)...)
	}
	if opt.Chat {
		out = append(out, ChatChecks(ctx, cfg)...)
	}
	// Alias warnings (V7) when aliases and model lists are available.
	out = append(out, aliasWarnings(cfg)...)
	return out
}

func storageChecks(cfg *config.Config, opt Options) []Check {
	dir := strings.TrimSpace(cfg.Storage.DataDir)
	jobsOn := cfg.JobsEnabled()
	if dir == "" {
		if jobsOn {
			return []Check{{
				Name: "storage.data_dir", Status: StatusFail, Message: "data_dir is empty but jobs need it",
				Hint: "set storage.data_dir to an absolute path on Mane",
			}}
		}
		return []Check{{
			Name: "storage.data_dir", Status: StatusWarn, Message: "jobs are off (data_dir not set)",
			Hint: "set storage.data_dir to enable saved requests, retries and batches",
		}}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return []Check{{
			Name: "storage.data_dir", Status: StatusFail, Message: fmt.Sprintf("cannot create data_dir: %v", err),
			Hint: "fix permissions or choose another path",
		}}
	}
	test := filepath.Join(dir, ".pingpong-doctor-write")
	if err := os.WriteFile(test, []byte("x"), 0600); err != nil {
		return []Check{{
			Name: "storage.data_dir", Status: StatusFail, Message: fmt.Sprintf("data_dir not writable: %v", err),
			Hint: "fix permissions on the folder",
		}}
	}
	_ = os.Remove(test)

	chk := Check{Name: "storage.data_dir", Status: StatusOK, Message: "data_dir exists and is writable"}
	if !jobsOn {
		chk.Status = StatusWarn
		chk.Message = "data_dir is set but jobs are off until the router loads this config"
		chk.Hint = "router must use the same config path"
	}
	out := []Check{chk}

	freeFn := opt.DiskFree
	if freeFn == nil {
		freeFn = freeBytesOnFS
	}
	free, err := freeFn(dir)
	if err != nil {
		out = append(out, Check{
			Name: "storage.disk", Status: StatusWarn, Message: fmt.Sprintf("could not read free space: %v", err),
		})
		return out
	}
	const gb = 1024 * 1024 * 1024
	switch {
	case free < 500*1024*1024:
		out = append(out, Check{
			Name: "storage.disk", Status: StatusFail,
			Message: fmt.Sprintf("%.1f MB free under data_dir", float64(free)/(1024*1024)),
			Hint:    "free disk space before saved requests fill it",
		})
	case free < 5*gb:
		out = append(out, Check{
			Name: "storage.disk", Status: StatusWarn,
			Message: fmt.Sprintf("%.1f GB free under data_dir", float64(free)/float64(gb)),
			Hint:    "consider freeing space or pruning old jobs",
		})
	default:
		out = append(out, Check{
			Name: "storage.disk", Status: StatusOK,
			Message: fmt.Sprintf("%.1f GB free under data_dir", float64(free)/float64(gb)),
		})
	}
	return out
}

func machineChecks(ctx context.Context, cfg *config.Config, m config.Machine) []Check {
	prefix := "machine." + m.Name
	cli := ollama.NewClient(m.Address, cfg.Health.Timeout.Duration)
	tags, err := cli.Tags(ctx)
	if err != nil {
		return []Check{{
			Name: prefix + ".reachable", Status: StatusFail,
			Message: fmt.Sprintf("cannot reach %s: %v", m.Name, err),
			Hint:    "check address, Ollama, and network",
		}}
	}
	names := modelNames(tags)
	out := []Check{{
		Name: prefix + ".reachable", Status: StatusOK,
		Message: fmt.Sprintf("%s answers within health timeout", m.Name),
	}}

	ver, err := cli.Version(ctx)
	if err != nil {
		out = append(out, Check{
			Name: prefix + ".version", Status: StatusInfo,
			Message: fmt.Sprintf("version not reported: %v", err),
		})
	} else {
		out = append(out, Check{
			Name: prefix + ".version", Status: StatusInfo,
			Message: fmt.Sprintf("Ollama version %s", ver),
		})
	}

	if route.ModelInstalled(names, m.DefaultModel) {
		out = append(out, Check{
			Name: prefix + ".default_model", Status: StatusOK,
			Message: fmt.Sprintf("default model %q is installed", m.DefaultModel),
		})
	} else {
		out = append(out, Check{
			Name: prefix + ".default_model", Status: StatusFail,
			Message: fmt.Sprintf("default model %q is not installed", m.DefaultModel),
			Hint:    "run ollama pull on the machine or change default_model",
		})
	}

	used := referencedModels(cfg)
	var unused []string
	for _, n := range names {
		if _, ok := used[n]; !ok {
			unused = append(unused, n)
		}
	}
	sort.Strings(unused)
	if len(unused) == 0 {
		out = append(out, Check{
			Name: prefix + ".models_unused", Status: StatusOK,
			Message: "every listed model is referenced in config",
		})
	} else {
		out = append(out, Check{
			Name: prefix + ".models_unused", Status: StatusInfo,
			Message: fmt.Sprintf("models not referenced in config: %s", strings.Join(unused, ", ")),
		})
	}
	return out
}

func modelNames(tags []ollama.ModelTag) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, t.Name)
	}
	return out
}

func referencedModels(cfg *config.Config) map[string]struct{} {
	used := make(map[string]struct{})
	for _, m := range cfg.Machines {
		used[m.DefaultModel] = struct{}{}
	}
	for _, p := range cfg.Profiles {
		if p.Model != "" {
			used[p.Model] = struct{}{}
		}
	}
	return used
}

// ExitCode maps check results to a process exit code.
func ExitCode(checks []Check, commandErr error) int {
	if commandErr != nil {
		return 2
	}
	for _, c := range checks {
		if c.Status == StatusFail {
			return 1
		}
	}
	return 0
}

// Failed reports whether any check failed.
func Failed(checks []Check) bool {
	return ExitCode(checks, nil) == 1
}
