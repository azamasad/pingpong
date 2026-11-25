package hoststats

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/config"
)

func TestCollectLocalRemoteAndMissing(t *testing.T) {
	s := &Sampler{
		now:  time.Now,
		read: func() (raw, error) { return raw{RAMTotal: 10, RAMUsed: 4}, nil },
	}
	c := Collector{
		Sampler: s,
		Fetch: func(context.Context, string) Sample {
			pct := 12.0
			return Sample{OK: true, CPUPercent: &pct}
		},
	}
	machines := []config.Machine{
		{Name: "mane", Address: "http://127.0.0.1:11434"},
		{Name: "firmino", Address: "http://10.0.0.8:11434", StatsAddress: "http://10.0.0.8:8788"},
		{Name: "other", Address: "http://10.0.0.9:11434"},
	}
	got := c.Collect(context.Background(), machines)
	if len(got) != 3 || got[0].Source != "local" || !got[0].Sample.OK {
		t.Fatalf("local %+v", got[0])
	}
	if got[1].Source != "remote" || got[1].Sample.CPUPercent == nil {
		t.Fatalf("remote %+v", got[1])
	}
	if got[2].Source != "none" || got[2].Sample.Error != "not_configured" {
		t.Fatalf("missing %+v", got[2])
	}
}

func TestFetchRejectsRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/elsewhere", http.StatusFound)
	}))
	defer srv.Close()
	s := Fetch(context.Background(), srv.URL)
	if s.OK || s.Error != "unreachable" {
		t.Fatalf("%+v", s)
	}
}

func TestHandlerSample(t *testing.T) {
	s := &Sampler{now: time.Now, read: func() (raw, error) {
		return raw{RAMTotal: 100, RAMUsed: 40}, nil
	}}
	ts := httptest.NewServer(NewHandler(s))
	defer ts.Close()
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
}
