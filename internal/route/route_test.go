package route

import "testing"

func baseSnap() Snapshot {
	return Snapshot{
		AutoRole: "heavy",
		FastRole: "fast",
		Machines: []MachineState{
			{
				Name:         "mane",
				Roles:        []string{"fast"},
				DefaultModel: "llama3.1:8b",
				Models:       []string{"llama3.1:8b", "llama3.2:3b", "llama3.1:latest"},
			},
			{
				Name:         "firmino",
				Roles:        []string{"heavy"},
				DefaultModel: "qwen2.5:14b",
				Models:       []string{"qwen2.5:14b"},
			},
		},
	}
}

func TestAuto(t *testing.T) {
	r := Decide(Request{}, baseSnap())
	if !r.OK || r.Machine != "firmino" || r.Mode != ModeAuto {
		t.Fatalf("%+v", r)
	}
}

func TestFast(t *testing.T) {
	r := Decide(Request{Fast: true}, baseSnap())
	if !r.OK || r.Machine != "mane" || r.Mode != ModeFast {
		t.Fatalf("%+v", r)
	}
}

func TestPinModel(t *testing.T) {
	r := Decide(Request{Model: "llama3.2:3b"}, baseSnap())
	if !r.OK || r.Machine != "mane" || r.Mode != ModePinnedModel {
		t.Fatalf("%+v", r)
	}
}

func TestPinMachine(t *testing.T) {
	r := Decide(Request{Machine: "firmino"}, baseSnap())
	if !r.OK || r.Machine != "firmino" || r.Model != "qwen2.5:14b" || r.Mode != ModePinnedMachine {
		t.Fatalf("%+v", r)
	}
}

func TestPinBothInstalled(t *testing.T) {
	r := Decide(Request{Machine: "firmino", Model: "qwen2.5:14b"}, baseSnap())
	if !r.OK || r.Mode != ModePinnedModelAndMachine {
		t.Fatalf("%+v", r)
	}
}

func TestPinBothNotInstalled(t *testing.T) {
	s := baseSnap()
	s.RefreshedModels = true
	r := Decide(Request{Machine: "mane", Model: "nope:7b"}, s)
	if r.Code != CodeModelNotInstalled {
		t.Fatalf("%+v", r)
	}
}

func TestModelAmbiguous(t *testing.T) {
	s := baseSnap()
	s.Machines[1].Models = append(s.Machines[1].Models, "shared:1b")
	s.Machines[0].Models = append(s.Machines[0].Models, "shared:1b")
	r := Decide(Request{Model: "shared:1b"}, s)
	if r.Code != CodeModelAmbiguous {
		t.Fatalf("%+v", r)
	}
}

func TestRolePicksLeastLoadedUpMachine(t *testing.T) {
	s := baseSnap()
	s.Machines = append(s.Machines, MachineState{
		Name: "studio", Roles: []string{"heavy"}, DefaultModel: "qwen2.5:32b",
		Models: []string{"qwen2.5:32b"}, InFlight: 0,
	})
	s.Machines[1].InFlight = 1
	r := Decide(Request{}, s)
	if !r.OK || r.Machine != "studio" || r.Model != "qwen2.5:32b" {
		t.Fatalf("%+v", r)
	}
}

func TestRoleSkipsDownPeer(t *testing.T) {
	s := baseSnap()
	s.Machines = append(s.Machines, MachineState{
		Name: "studio", Roles: []string{"heavy"}, DefaultModel: "qwen2.5:32b",
		Models: []string{"qwen2.5:32b"}, Down: true,
	})
	r := Decide(Request{}, s)
	if !r.OK || r.Machine != "firmino" {
		t.Fatalf("%+v", r)
	}
}

func TestRoleAllDownUsesFirst(t *testing.T) {
	s := baseSnap()
	s.Machines[1].Down = true
	s.Machines = append(s.Machines, MachineState{
		Name: "studio", Roles: []string{"heavy"}, DefaultModel: "qwen2.5:32b",
		Models: []string{"qwen2.5:32b"}, Down: true,
	})
	r := Decide(Request{}, s)
	if r.Code != CodeMachineDown {
		t.Fatalf("%+v", r)
	}
}

func TestUnknownMachine(t *testing.T) {
	r := Decide(Request{Machine: "ghost"}, baseSnap())
	if r.Code != CodeMachineUnknown {
		t.Fatalf("%+v", r)
	}
}

func TestModelNotInstalledAfterRefresh(t *testing.T) {
	s := baseSnap()
	r := Decide(Request{Model: "70b-example"}, s)
	if !r.NeedRefresh {
		t.Fatalf("want refresh first: %+v", r)
	}
	s.RefreshedModels = true
	r = Decide(Request{Model: "70b-example"}, s)
	if r.Code != CodeModelNotInstalled {
		t.Fatalf("%+v", r)
	}
}

func TestModelsUnknown(t *testing.T) {
	s := baseSnap()
	s.Machines[1].NeverReached = true
	s.Machines[1].Models = nil
	r := Decide(Request{Model: "maybe:on-firmino"}, s)
	if r.Code != CodeModelsUnknown {
		t.Fatalf("%+v", r)
	}
}

func TestPinOnDownMachine(t *testing.T) {
	s := baseSnap()
	s.Machines[1].Down = true
	r := Decide(Request{Machine: "firmino"}, s)
	if r.Code != CodeMachineDown {
		t.Fatalf("%+v", r)
	}
}

func TestAutoDownNotModelMissing(t *testing.T) {
	s := baseSnap()
	s.Machines[1].Down = true
	r := Decide(Request{}, s)
	if r.Code != CodeMachineDown {
		t.Fatalf("%+v", r)
	}
}

func TestDefaultNotInstalledMachineUp(t *testing.T) {
	s := baseSnap()
	s.Machines[1].Models = []string{}
	r := Decide(Request{}, s)
	if r.Code != CodeModelNotInstalled {
		t.Fatalf("%+v", r)
	}
}

func TestFastPlusPinWins(t *testing.T) {
	r := Decide(Request{Fast: true, Machine: "firmino"}, baseSnap())
	if !r.OK || r.Machine != "firmino" {
		t.Fatalf("%+v", r)
	}
}

func TestModelWithoutTag(t *testing.T) {
	r := Decide(Request{Model: "llama3.1"}, baseSnap())
	if !r.OK || r.Model != "llama3.1:latest" {
		t.Fatalf("%+v", r)
	}
}

func TestNoTargetRole(t *testing.T) {
	s := baseSnap()
	s.Machines[1].Roles = nil
	r := Decide(Request{}, s)
	if r.Code != CodeNoTarget {
		t.Fatalf("%+v", r)
	}
}
