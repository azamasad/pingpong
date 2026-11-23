package jobs

import (
	"encoding/json"

	"github.com/azamasad/pingpong/internal/config"
)

// aliasForRequest returns the alias name when request JSON model is an alias key.
func aliasForRequest(cfg *config.Config, requestJSON string) string {
	if cfg.Aliases == nil {
		return ""
	}
	var body struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(requestJSON), &body); err != nil || body.Model == "" {
		return ""
	}
	if _, ok := cfg.Aliases[body.Model]; ok {
		return body.Model
	}
	return ""
}
