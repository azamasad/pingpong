package jobs

import "testing"

func TestSplitTargetIndexRoundRobin(t *testing.T) {
	plan := SplitPlan{
		Assign: "round_robin",
		Targets: []SplitTarget{
			{Machine: "mane", Model: "a"},
			{Machine: "firmino", Model: "b"},
		},
	}
	want := []int{0, 1, 0, 1, 0, 1, 0, 1, 0, 1}
	for k, w := range want {
		if got := splitTargetIndex(k, plan); got != w {
			t.Fatalf("k=%d got %d want %d", k, got, w)
		}
	}
}

func TestSplitTargetIndexWeightsTen(t *testing.T) {
	plan := SplitPlan{
		Assign: "weights",
		Targets: []SplitTarget{
			{Machine: "mane", Model: "a", Weight: 3},
			{Machine: "firmino", Model: "b", Weight: 2},
		},
	}
	counts := [2]int{}
	for k := 0; k < 10; k++ {
		counts[splitTargetIndex(k, plan)]++
	}
	if counts[0] != 6 || counts[1] != 4 {
		t.Fatalf("counts %v", counts)
	}
}

func TestSplitTargetIndexWeightsSeven(t *testing.T) {
	plan := SplitPlan{
		Assign: "weights",
		Targets: []SplitTarget{
			{Machine: "mane", Model: "a", Weight: 3},
			{Machine: "firmino", Model: "b", Weight: 2},
		},
	}
	want := []int{0, 0, 0, 1, 1, 0, 0}
	for k, w := range want {
		if got := splitTargetIndex(k, plan); got != w {
			t.Fatalf("k=%d got %d want %d", k, got, w)
		}
	}
}
