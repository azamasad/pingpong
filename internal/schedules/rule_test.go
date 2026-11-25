package schedules

import (
	"testing"
	"time"

	_ "time/tzdata"
)

func TestNextDueDaily(t *testing.T) {
	loc := time.Local
	rule, err := ValidateRule(Rule{Every: "day", At: "02:00"})
	if err != nil {
		t.Fatal(err)
	}
	ref := time.Date(2025, 6, 10, 3, 0, 0, 0, loc)
	next := rule.NextDueAfter(ref, loc)
	want := time.Date(2025, 6, 11, 2, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
	ref2 := time.Date(2025, 6, 10, 2, 0, 0, 0, loc)
	next2 := rule.NextDueAfter(ref2, loc)
	want2 := time.Date(2025, 6, 11, 2, 0, 0, 0, loc)
	if !next2.Equal(want2) {
		t.Fatalf("exact ref got %v", next2)
	}
}

func TestNextDueWeekly(t *testing.T) {
	loc := time.Local
	rule, _ := ValidateRule(Rule{Every: "week", At: "09:30", Weekday: "mon"})
	ref := time.Date(2025, 6, 9, 10, 0, 0, 0, loc) // Monday
	next := rule.NextDueAfter(ref, loc)
	want := time.Date(2025, 6, 16, 9, 30, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}

func TestDSTSpringForward(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	rule, _ := ValidateRule(Rule{Every: "day", At: "02:30"})
	// US spring 2025: March 9 — 2:30 AM does not exist
	d := time.Date(2025, 3, 8, 12, 0, 0, 0, ny)
	next := rule.NextDueAfter(d, ny)
	// time.Date for non-existent wall time shifts forward once
	cand := rule.WallTime(2025, time.March, 9, ny)
	if !next.Equal(cand) {
		t.Fatalf("spring got %v cand %v", next, cand)
	}
}

func TestDSTFallBackFirstOnly(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	rule, _ := ValidateRule(Rule{Every: "day", At: "01:30"})
	d1 := rule.WallTime(2025, time.November, 2, ny)
	d2 := rule.NextDueAfter(d1, ny)
	if d1.Equal(d2) {
		t.Fatal("should not schedule duplicate fall-back hour")
	}
}
