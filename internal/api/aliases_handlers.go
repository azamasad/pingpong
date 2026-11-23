package api

import (
	"encoding/json"
	"net/http"
	"sort"
)

func (s *Server) handleAliases(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Name      string   `json:"name"`
		Model     string   `json:"model"`
		Machine   string   `json:"machine,omitempty"`
		Installed bool     `json:"installed"`
		Machines  []string `json:"machines"`
	}
	if len(s.Cfg.Aliases) == 0 {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}
	names := make([]string, 0, len(s.Cfg.Aliases))
	for n := range s.Cfg.Aliases {
		names = append(names, n)
	}
	sort.Strings(names)
	modelToMachines := map[string][]string{}
	for _, a := range s.Reg.ListModels() {
		modelToMachines[a.Name] = append([]string{}, a.Machines...)
	}
	out := make([]row, 0, len(names))
	for _, name := range names {
		a := s.Cfg.Aliases[name]
		machines := modelToMachines[a.Model]
		out = append(out, row{
			Name: name, Model: a.Model, Machine: a.Machine,
			Installed: len(machines) > 0, Machines: machines,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
