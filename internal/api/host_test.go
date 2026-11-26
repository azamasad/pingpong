package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHostEndpoint(t *testing.T) {
	h, f := fakePair(t)
	ts, _ := newHarness(t, h, f)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/host", nil)
	req.Host = "localhost"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body struct {
		Machines []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
			Sample struct {
				OK bool `json:"ok"`
			} `json:"sample"`
		} `json:"machines"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Machines) != 2 {
		t.Fatalf("machines %+v", body.Machines)
	}
	for _, m := range body.Machines {
		if m.Source != "local" || !m.Sample.OK {
			t.Fatalf("expected a local reading for %s: %+v", m.Name, m)
		}
	}
}
