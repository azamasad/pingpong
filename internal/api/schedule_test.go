package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestScheduleCRUD(t *testing.T) {
	_, _, mURL, fURL := fakeollama.Pair(t, []string{"qwen2.5:14b"}, []string{"llama3.1:8b"})
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	tpl, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/templates", strings.NewReader(
		`{"name":"notes","body":"Sum {{x}}"}`))
	tpl.Host = "localhost"
	tpl.Header.Set("Content-Type", "application/json")
	_, _ = http.DefaultClient.Do(tpl)

	body := `{"name":"nightly-notes","rule":{"every":"day","at":"02:00"},"template":"notes","vars":{"x":"1"},
"machine":"firmino","model":"qwen2.5:14b","enabled":true}`
	c, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/schedules", strings.NewReader(body))
	c.Host = "localhost"
	c.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(c)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create %d", res.StatusCode)
	}
	var sch map[string]any
	_ = json.NewDecoder(res.Body).Decode(&sch)
	id := sch["id"].(string)
	if sch["next_due"] == nil {
		t.Fatal("expected next_due")
	}

	patch, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/schedules/"+id,
		strings.NewReader(`{"enabled":false}`))
	patch.Host = "localhost"
	patch.Header.Set("Content-Type", "application/json")
	pres, _ := http.DefaultClient.Do(patch)
	var patched map[string]any
	_ = json.NewDecoder(pres.Body).Decode(&patched)
	if patched["enabled"] != false || patched["next_due"] != nil {
		t.Fatalf("pause %+v", patched)
	}

	rn, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/schedules/"+id+"/run", nil)
	rn.Host = "localhost"
	rnRes, _ := http.DefaultClient.Do(rn)
	if rnRes.StatusCode != http.StatusAccepted {
		t.Fatalf("run now %d", rnRes.StatusCode)
	}

	del, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/schedules/"+id, nil)
	del.Host = "localhost"
	dres, _ := http.DefaultClient.Do(del)
	if dres.StatusCode != http.StatusNoContent {
		t.Fatalf("delete %d", dres.StatusCode)
	}
}

func TestScheduleCapAndValidation(t *testing.T) {
	_, _, mURL, fURL := fakeollama.Pair(t, []string{"qwen2.5:14b"}, []string{"llama3.1:8b"})
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	bad, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/schedules", strings.NewReader(
		`{"name":"bad","rule":{"every":"hour","at":"02:00"},"prompt":"x","machine":"firmino","model":"qwen2.5:14b"}`))
	bad.Host = "localhost"
	bad.Header.Set("Content-Type", "application/json")
	bres, _ := http.DefaultClient.Do(bad)
	if bres.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad rule %d", bres.StatusCode)
	}
}
