package templates

import "testing"

func TestSubstituteBasic(t *testing.T) {
	out, miss, un := Substitute("Hi {{name}}, limit {{n}}", map[string]string{"name": "Ada", "n": "3"})
	if out != "Hi Ada, limit 3" || len(miss) != 0 {
		t.Fatalf("out=%q miss=%v", out, miss)
	}
	if len(un) != 0 {
		t.Fatalf("unused %v", un)
	}
}

func TestSubstituteRepeatedBlank(t *testing.T) {
	out, miss, _ := Substitute("{{x}} and {{x}}", map[string]string{"x": "same"})
	if out != "same and same" || len(miss) != 0 {
		t.Fatalf("out=%q miss=%v", out, miss)
	}
}

func TestSubstituteLiteralBraces(t *testing.T) {
	out, miss, _ := Substitute(`{{ x }} stays, {{valid}} ok`, map[string]string{"valid": "v"})
	if out != `{{ x }} stays, v ok` || len(miss) != 0 {
		t.Fatalf("out=%q miss=%v", out, miss)
	}
}

func TestSubstituteNoRescan(t *testing.T) {
	out, miss, _ := Substitute("{{a}}", map[string]string{"a": "{{b}}"})
	if out != "{{b}}" || len(miss) != 0 {
		t.Fatalf("out=%q miss=%v", out, miss)
	}
}

func TestSubstituteMissingAndUnused(t *testing.T) {
	_, miss, un := Substitute("{{need}}", map[string]string{"extra": "x"})
	if len(miss) != 1 || miss[0] != "need" {
		t.Fatalf("miss=%v", miss)
	}
	if len(un) != 1 || un[0] != "extra" {
		t.Fatalf("un=%v", un)
	}
}

func TestBlanksInOrder(t *testing.T) {
	got := BlanksInOrder("{{b}} then {{a}} and {{b}}")
	want := []string{"b", "a"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
}

func TestValidName(t *testing.T) {
	if !ValidName("sum-mary_1") || ValidName("") || ValidName("UPPER") {
		t.Fatal("name validation wrong")
	}
}
