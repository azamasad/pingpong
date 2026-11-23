package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/doctor"
)

func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to YAML config (or set PINGPONG_CONFIG)")
	chat := fs.Bool("chat", false, "run a tiny chat against each machine (slow)")
	asJSON := fs.Bool("json", false, "print results as JSON")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *chat {
		fmt.Fprintln(os.Stderr, "doctor: --chat is not implemented yet (M11b)")
		return 2
	}

	path, err := config.PathFromEnvOrFlag(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor: %v\n", err)
		return 2
	}

	var checks []doctor.Check

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "doctor: config file %q: %v\n", path, err)
			return 2
		}
		fmt.Fprintf(os.Stderr, "doctor: %v\n", err)
		return 2
	}

	cfg, warnings, err := config.Load(path)
	if err != nil {
		checks = append(checks, doctor.Check{
			Name: "config.load", Status: doctor.StatusFail, Message: err.Error(),
			Hint: "fix the config file and try again",
		})
		printDoctorResults(checks, *asJSON)
		return doctor.ExitCode(checks, nil)
	}
	checks = append(checks, doctor.Check{
		Name: "config.load", Status: doctor.StatusOK, Message: "config loaded",
	})
	checks = append(checks, doctor.Run(context.Background(), cfg, warnings, doctor.Options{})...)

	printDoctorResults(checks, *asJSON)
	return doctor.ExitCode(checks, nil)
}

func printDoctorResults(checks []doctor.Check, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(checks)
		return
	}
	for _, c := range checks {
		line := fmt.Sprintf("%-6s %s", c.Status, c.Name)
		if c.Message != "" {
			line += ": " + c.Message
		}
		fmt.Println(line)
		if c.Hint != "" && c.Status != doctor.StatusOK && c.Status != doctor.StatusInfo {
			fmt.Printf("       hint: %s\n", c.Hint)
		}
	}
}
