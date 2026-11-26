package hoststats

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/azamasad/pingpong/internal/config"
)

// Report is one machine's hardware reading for the page.
type Report struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Sample Sample `json:"sample"`
}

// Collector reads this host and optional remote stats URLs.
type Collector struct {
	Sampler *Sampler
	Fetch   func(ctx context.Context, rawURL string) Sample
}

// Collect returns one report per configured machine, in file order.
func (c Collector) Collect(ctx context.Context, machines []config.Machine) []Report {
	if c.Fetch == nil {
		c.Fetch = Fetch
	}
	out := make([]Report, len(machines))
	type got struct {
		i int
		r Report
	}
	ch := make(chan got, len(machines))
	pending := 0
	var local *Sample
	for i, m := range machines {
		if m.StatsAddress != "" {
			pending++
			go func(i int, name, rawURL string) {
				ch <- got{i, Report{Name: name, Source: "remote", Sample: c.Fetch(ctx, rawURL)}}
			}(i, m.Name, m.StatsAddress)
			continue
		}
		if LocalMachine(m.Address) {
			if local == nil {
				s := c.Sampler.Sample()
				local = &s
			}
			out[i] = Report{Name: m.Name, Source: "local", Sample: *local}
			continue
		}
		out[i] = Report{Name: m.Name, Source: "none", Sample: Sample{OK: false, Error: "not_configured"}}
	}
	gotN := 0
	for gotN < pending {
		select {
		case item := <-ch:
			out[item.i] = item.r
			gotN++
		case <-ctx.Done():
			for j := range machines {
				if machines[j].StatsAddress != "" && out[j].Name == "" {
					out[j] = Report{Name: machines[j].Name, Source: "remote", Sample: Sample{OK: false, Error: "unreachable"}}
				}
			}
			return out
		}
	}
	return out
}

// Fetch GETs a stats URL. Redirects and oversized or unexpected bodies are a failed report.
// The error string never includes the URL.
func Fetch(ctx context.Context, rawURL string) Sample {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Sample{OK: false, Error: "unreachable"}
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return Sample{OK: false, Error: "unreachable"}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Sample{OK: false, Error: "unreachable"}
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8192))
	if err != nil {
		return Sample{OK: false, Error: "bad report"}
	}
	var s Sample
	if err := json.Unmarshal(body, &s); err != nil || !s.OK {
		if s.Error != "" {
			return Sample{OK: false, Error: s.Error}
		}
		return Sample{OK: false, Error: "bad report"}
	}
	return s
}
