package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the router configuration (V1 fields; V2 sections are ignored until used).
type Config struct {
	Router   RouterConfig       `yaml:"router"`
	Machines []Machine          `yaml:"machines"`
	Routing  RoutingConfig      `yaml:"routing"`
	Timeouts TimeoutConfig      `yaml:"timeouts"`
	Health   HealthConfig       `yaml:"health"`
	Retry    RetryConfig        `yaml:"retry"`
	Storage  StorageConfig      `yaml:"storage"`
	Profiles map[string]Profile `yaml:"profiles"`
}

// Profile is a named preset (V5). It fills fast, model, and machine when the client leaves them unset.
type Profile struct {
	Fast    bool   `yaml:"fast"`
	Model   string `yaml:"model"`
	Machine string `yaml:"machine"`
}

type RetryConfig struct {
	RetryInterval Duration `yaml:"retry_interval"`
	RetryWindow   Duration `yaml:"retry_window"`
}

type StorageConfig struct {
	DataDir                string   `yaml:"data_dir"`
	RetentionDays          int      `yaml:"retention_days"`
	RetentionSweepInterval Duration `yaml:"retention_sweep_interval"`
	OutputFlushInterval    Duration `yaml:"output_flush_interval"`
}

type RouterConfig struct {
	Listen       string   `yaml:"listen"`
	AllowedHosts []string `yaml:"allowed_hosts"`
	MaxBodyBytes int64    `yaml:"max_body_bytes"`
	LogLevel     string   `yaml:"log_level"`
}

type Machine struct {
	Name          string   `yaml:"name"`
	Address       string   `yaml:"address"`
	Roles         []string `yaml:"roles"`
	DefaultModel  string   `yaml:"default_model"`
	MaxConcurrent int      `yaml:"max_concurrent"`
	NumCtx        int      `yaml:"num_ctx"`
}

type RoutingConfig struct {
	AutoRole string `yaml:"auto_role"`
	FastRole string `yaml:"fast_role"`
}

type TimeoutConfig struct {
	ConnectTimeout    Duration `yaml:"connect_timeout"`
	FirstTokenTimeout Duration `yaml:"first_token_timeout"`
	IdleTokenTimeout  Duration `yaml:"idle_token_timeout"`
	MaxRunTime        Duration `yaml:"max_run_time"`
	HeartbeatInterval Duration `yaml:"heartbeat_interval"`
}

type HealthConfig struct {
	Interval                Duration `yaml:"interval"`
	Timeout                 Duration `yaml:"timeout"`
	FailuresToMarkDown      int      `yaml:"failures_to_mark_down"`
	RegistryRefreshInterval Duration `yaml:"registry_refresh_interval"`
	RefreshOnModelMiss      bool     `yaml:"refresh_on_model_miss"`
}

// Duration wraps time.Duration for YAML like "30m" and "15s".
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	if s == "" {
		d.Duration = 0
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	if d.Duration == 0 {
		return nil, nil
	}
	return d.Duration.String(), nil
}

// Load reads and validates a config file. Warnings are non-fatal (for example duplicate roles).
func Load(path string) (*Config, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var raw Config
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}
	raw.applyDefaults()
	warnings, err := raw.validate()
	if err != nil {
		return nil, warnings, err
	}
	return &raw, warnings, nil
}

// PathFromEnvOrFlag returns the config path from flag or PINGPONG_CONFIG.
func PathFromEnvOrFlag(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	if env := os.Getenv("PINGPONG_CONFIG"); env != "" {
		return env, nil
	}
	return "", errors.New("config path required: use --config or set PINGPONG_CONFIG")
}

func (c *Config) applyDefaults() {
	if c.Router.Listen == "" {
		c.Router.Listen = "127.0.0.1:8787"
	}
	if c.Router.MaxBodyBytes == 0 {
		c.Router.MaxBodyBytes = 4 << 20
	}
	if c.Router.LogLevel == "" {
		c.Router.LogLevel = "info"
	}
	for i := range c.Machines {
		m := &c.Machines[i]
		if m.MaxConcurrent == 0 {
			m.MaxConcurrent = 1
		}
		if m.NumCtx == 0 {
			m.NumCtx = 8192
		}
	}
	if c.Routing.AutoRole == "" {
		c.Routing.AutoRole = "heavy"
	}
	if c.Routing.FastRole == "" {
		c.Routing.FastRole = "fast"
	}
	if c.Timeouts.ConnectTimeout.Duration == 0 {
		c.Timeouts.ConnectTimeout = Duration{5 * time.Second}
	}
	if c.Timeouts.FirstTokenTimeout.Duration == 0 {
		c.Timeouts.FirstTokenTimeout = Duration{30 * time.Minute}
	}
	if c.Timeouts.IdleTokenTimeout.Duration == 0 {
		c.Timeouts.IdleTokenTimeout = Duration{5 * time.Minute}
	}
	if c.Timeouts.MaxRunTime.Duration == 0 {
		c.Timeouts.MaxRunTime = Duration{2 * time.Hour}
	}
	if c.Timeouts.HeartbeatInterval.Duration == 0 {
		c.Timeouts.HeartbeatInterval = Duration{15 * time.Second}
	}
	if c.Health.Interval.Duration == 0 {
		c.Health.Interval = Duration{30 * time.Second}
	}
	if c.Health.Timeout.Duration == 0 {
		c.Health.Timeout = Duration{10 * time.Second}
	}
	if c.Health.FailuresToMarkDown == 0 {
		c.Health.FailuresToMarkDown = 2
	}
	if c.Health.RegistryRefreshInterval.Duration == 0 {
		c.Health.RegistryRefreshInterval = Duration{5 * time.Minute}
	}
	if c.Retry.RetryInterval.Duration == 0 {
		c.Retry.RetryInterval = Duration{2 * time.Minute}
	}
	if c.Retry.RetryWindow.Duration == 0 {
		c.Retry.RetryWindow = Duration{3 * time.Hour}
	}
	if c.Storage.RetentionDays == 0 {
		c.Storage.RetentionDays = 90
	}
	if c.Storage.RetentionSweepInterval.Duration == 0 {
		c.Storage.RetentionSweepInterval = Duration{24 * time.Hour}
	}
	if c.Storage.OutputFlushInterval.Duration == 0 {
		c.Storage.OutputFlushInterval = Duration{time.Second}
	}
	if len(c.Profiles) == 0 {
		c.Profiles = map[string]Profile{
			"fast":     {Fast: true},
			"balanced": {},
			"deep":     {},
		}
	}
}

// ApplyProfile fills unset routing fields from a named preset.
// An explicit fast flag, model, or machine on the request wins.
func (c *Config) ApplyProfile(name string, fast bool, model, machine string) (bool, string, string, error) {
	if name == "" {
		return fast, model, machine, nil
	}
	p, ok := c.Profiles[name]
	if !ok {
		return fast, model, machine, fmt.Errorf("unknown profile %q", name)
	}
	if !fast {
		fast = p.Fast
	}
	if model == "" {
		model = p.Model
	}
	if machine == "" {
		machine = p.Machine
	}
	return fast, model, machine, nil
}

// JobsEnabled is true when durable job storage is configured.
func (c *Config) JobsEnabled() bool {
	return strings.TrimSpace(c.Storage.DataDir) != ""
}

func (c *Config) validate() ([]string, error) {
	var warnings []string
	if len(c.Machines) == 0 {
		return warnings, errors.New("at least one machine is required")
	}
	seen := make(map[string]struct{})
	for _, m := range c.Machines {
		if strings.TrimSpace(m.Name) == "" {
			return warnings, errors.New("machine name must be non-empty")
		}
		if _, dup := seen[m.Name]; dup {
			return warnings, fmt.Errorf("duplicate machine name %q", m.Name)
		}
		seen[m.Name] = struct{}{}
		if m.DefaultModel == "" {
			return warnings, fmt.Errorf("machine %q: default_model is required", m.Name)
		}
		if m.MaxConcurrent < 1 {
			return warnings, fmt.Errorf("machine %q: max_concurrent must be >= 1", m.Name)
		}
		if m.NumCtx <= 0 {
			return warnings, fmt.Errorf("machine %q: num_ctx must be > 0", m.Name)
		}
		u, err := url.Parse(m.Address)
		if err != nil || u.Scheme != "http" || u.Host == "" {
			return warnings, fmt.Errorf("machine %q: address must be a valid http URL", m.Name)
		}
	}
	if !roleExists(c, c.Routing.AutoRole) {
		return warnings, fmt.Errorf("auto_role %q: no machine has this role", c.Routing.AutoRole)
	}
	if !roleExists(c, c.Routing.FastRole) {
		return warnings, fmt.Errorf("fast_role %q: no machine has this role", c.Routing.FastRole)
	}
	roleCount := map[string]int{}
	for _, m := range c.Machines {
		for _, r := range m.Roles {
			roleCount[r]++
		}
	}
	for role, n := range roleCount {
		if n > 1 {
			warnings = append(warnings, fmt.Sprintf("role %q is on %d machines; V4 picks the least loaded machine that is up", role, n))
		}
	}
	if c.Timeouts.FirstTokenTimeout.Duration < c.Timeouts.IdleTokenTimeout.Duration {
		return warnings, errors.New("first_token_timeout must not be shorter than idle_token_timeout")
	}
	if _, _, err := net.SplitHostPort(c.Router.Listen); err != nil {
		return warnings, fmt.Errorf("router.listen: %w", err)
	}
	host, _, _ := net.SplitHostPort(c.Router.Listen)
	if host != "127.0.0.1" && host != "localhost" {
		return warnings, fmt.Errorf("router.listen must bind to localhost (got host %q)", host)
	}
	return warnings, nil
}

func roleExists(c *Config, role string) bool {
	for _, m := range c.Machines {
		for _, r := range m.Roles {
			if r == role {
				return true
			}
		}
	}
	return false
}

// MachineByName returns a machine config by registry name.
func (c *Config) MachineByName(name string) (*Machine, bool) {
	for i := range c.Machines {
		if c.Machines[i].Name == name {
			return &c.Machines[i], true
		}
	}
	return nil, false
}

// FirstMachineWithRole returns the first machine in file order with the role.
func (c *Config) FirstMachineWithRole(role string) (*Machine, bool) {
	for i := range c.Machines {
		for _, r := range c.Machines[i].Roles {
			if r == role {
				return &c.Machines[i], true
			}
		}
	}
	return nil, false
}
