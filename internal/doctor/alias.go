package doctor

import (
	"context"
	"fmt"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/ollama"
)

func aliasWarnings(cfg *config.Config) []Check {
	if len(cfg.Aliases) == 0 {
		return nil
	}
	installed := map[string]struct{}{}
	for _, m := range cfg.Machines {
		cli := ollama.NewClient(m.Address, cfg.Health.Timeout.Duration)
		tags, err := cli.Tags(context.Background())
		if err != nil {
			continue
		}
		for _, t := range tags {
			installed[t.Name] = struct{}{}
		}
	}
	var out []Check
	for name, a := range cfg.Aliases {
		if _, ok := installed[name]; ok {
			out = append(out, Check{
				Name: "alias." + name + ".name_collision", Status: StatusWarn,
				Message: fmt.Sprintf("alias %q matches an installed model name; the alias wins", name),
			})
		}
		found := false
		for mname := range installed {
			if mname == a.Model {
				found = true
				break
			}
		}
		if !found && len(installed) > 0 {
			out = append(out, Check{
				Name: "alias." + name + ".target", Status: StatusWarn,
				Message: fmt.Sprintf("alias %q targets %q, not seen on any reachable machine", name, a.Model),
			})
		}
	}
	return out
}
