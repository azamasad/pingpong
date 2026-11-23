package ollama_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/ollama"
)

func TestVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"0.5.1"}`))
	}))
	defer ts.Close()
	cli := ollama.NewClient(ts.URL, time.Second)
	v, err := cli.Version(context.Background())
	if err != nil || v != "0.5.1" {
		t.Fatalf("version %q err %v", v, err)
	}
}
