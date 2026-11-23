package route

import (
	"strings"
)

// Error codes match docs/API.md.
const (
	CodeMachineUnknown    = "MACHINE_UNKNOWN"
	CodeModelNotInstalled = "MODEL_NOT_INSTALLED"
	CodeModelAmbiguous    = "MODEL_AMBIGUOUS"
	CodeModelsUnknown     = "MODELS_UNKNOWN"
	CodeNoTarget          = "NO_TARGET"
	CodeMachineDown       = "MACHINE_DOWN"
)

const (
	ModeAuto                  = "auto"
	ModeFast                  = "fast"
	ModePinnedModel           = "pinned_model"
	ModePinnedMachine         = "pinned_machine"
	ModePinnedModelAndMachine = "pinned_model_and_machine"
)

// MachineState is a routing-time view of one machine.
type MachineState struct {
	Name         string
	Roles        []string
	DefaultModel string
	Models       []string
	Down         bool
	NeverReached bool
	InFlight     int
	Waiting      int
}

// Snapshot is registry state for a routing decision.
type Snapshot struct {
	Machines        []MachineState
	AutoRole        string
	FastRole        string
	RefreshedModels bool
}

// Request is client routing inputs (pins override Fast).
type Request struct {
	Fast    bool
	Model   string
	Machine string
}

// Result of a routing decision.
type Result struct {
	OK          bool
	Machine     string
	Model       string
	Mode        string
	Code        string
	NeedRefresh bool
}

// Decide picks machine and model per docs/ROUTING.md section 3.
func Decide(req Request, snap Snapshot) Result {
	machineName, modelWanted, mode, code := pickTarget(req, snap)
	if code != "" {
		return Result{Code: code}
	}
	if req.Model != "" && req.Machine == "" && modelWanted != "" {
		if len(machinesWithModel(snap, modelWanted)) == 0 && !anyNeverReached(snap) && !snap.RefreshedModels {
			return Result{NeedRefresh: true}
		}
	}

	m := findMachine(snap, machineName)
	if m == nil {
		return Result{Code: CodeMachineUnknown}
	}
	if m.Down {
		return Result{Code: CodeMachineDown}
	}

	installed, ok := resolveInstalledName(m.Models, modelWanted)
	if !ok {
		if req.Model != "" && !snap.RefreshedModels {
			return Result{NeedRefresh: true}
		}
		return Result{Code: CodeModelNotInstalled}
	}
	return Result{
		OK:      true,
		Machine: machineName,
		Model:   installed,
		Mode:    mode,
	}
}

func pickTarget(req Request, snap Snapshot) (machine, model, mode, code string) {
	hasMachine := req.Machine != ""
	hasModel := req.Model != ""

	if hasMachine {
		m := findMachine(snap, req.Machine)
		if m == nil {
			return "", "", "", CodeMachineUnknown
		}
		if hasModel {
			return req.Machine, req.Model, ModePinnedModelAndMachine, ""
		}
		return req.Machine, m.DefaultModel, ModePinnedMachine, ""
	}

	if hasModel {
		matches := machinesWithModel(snap, req.Model)
		switch len(matches) {
		case 1:
			return matches[0].Name, req.Model, ModePinnedModel, ""
		case 0:
			if anyNeverReached(snap) {
				return "", "", "", CodeModelsUnknown
			}
			if !snap.RefreshedModels {
				return "", req.Model, ModePinnedModel, ""
			}
			return "", "", "", CodeModelNotInstalled
		default:
			return "", "", "", CodeModelAmbiguous
		}
	}

	role := snap.AutoRole
	mode = ModeAuto
	if req.Fast {
		role = snap.FastRole
		mode = ModeFast
	}
	m := pickRole(snap, role)
	if m == nil {
		return "", "", "", CodeNoTarget
	}
	return m.Name, m.DefaultModel, mode, ""
}

// pickRole chooses among machines that have role.
// Up machines beat down ones. Among up machines, the fewest in-flight
// requests wins, then the shortest wait queue, then file order.
// If every match is down, the first in file order is returned so the
// caller reports MACHINE_DOWN.
func pickRole(snap Snapshot, role string) *MachineState {
	var all []*MachineState
	for i := range snap.Machines {
		for _, r := range snap.Machines[i].Roles {
			if r == role {
				all = append(all, &snap.Machines[i])
				break
			}
		}
	}
	if len(all) == 0 {
		return nil
	}
	var up []*MachineState
	for _, m := range all {
		if !m.Down {
			up = append(up, m)
		}
	}
	if len(up) == 0 {
		return all[0]
	}
	best := up[0]
	for _, m := range up[1:] {
		if m.InFlight < best.InFlight || (m.InFlight == best.InFlight && m.Waiting < best.Waiting) {
			best = m
		}
	}
	return best
}

func findMachine(snap Snapshot, name string) *MachineState {
	for i := range snap.Machines {
		if snap.Machines[i].Name == name {
			return &snap.Machines[i]
		}
	}
	return nil
}

func machinesWithModel(snap Snapshot, want string) []MachineState {
	var out []MachineState
	for _, m := range snap.Machines {
		if m.NeverReached {
			continue
		}
		if modelInstalled(m.Models, want) {
			out = append(out, m)
		}
	}
	return out
}

func anyNeverReached(snap Snapshot) bool {
	for _, m := range snap.Machines {
		if m.NeverReached {
			return true
		}
	}
	return false
}

func modelInstalled(models []string, want string) bool {
	_, ok := resolveInstalledName(models, want)
	return ok
}

func resolveInstalledName(models []string, want string) (installed string, ok bool) {
	if want == "" {
		return "", false
	}
	for _, m := range models {
		if m == want {
			return m, true
		}
	}
	if !strings.Contains(want, ":") {
		tag := want + ":latest"
		for _, m := range models {
			if m == tag {
				return m, true
			}
		}
	}
	return "", false
}
