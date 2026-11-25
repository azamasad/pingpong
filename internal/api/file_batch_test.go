package api_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func postMultipart(t *testing.T, url string, fields map[string]string, fileName string, fileContent []byte) (*http.Response, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if fileContent != nil {
		fw, _ := w.CreateFormFile("file", fileName)
		_, _ = fw.Write(fileContent)
	}
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, url, &buf)
	req.Host = "localhost"
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = decodeJSON(res, &out)
	return res, out
}

func TestFileBatchDryRunAndSubmit(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		Reply: func(call fakeollama.Call) []fakeollama.Step {
			return []fakeollama.Step{{Content: "chunk-out"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts := newHarnessJobs(t, hTs.URL, fTs.URL)
	defer ts.Close()

	text := "One.\n\nTwo.\n\nThree."
	res, body := postMultipart(t, ts.URL+"/api/batches/from-file", map[string]string{
		"instruction": "Summarise: {{chunk}}",
		"dry_run":     "1",
	}, "doc.txt", []byte(text))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("dry_run status %d %+v", res.StatusCode, body)
	}
	if body["chunks"].(float64) != 1 {
		t.Fatalf("chunks %+v", body)
	}
	res2, created := postMultipart(t, ts.URL+"/api/batches/from-file", map[string]string{
		"instruction": "Summarise: {{chunk}}",
	}, "doc.txt", []byte(text))
	if res2.StatusCode != http.StatusAccepted {
		t.Fatalf("submit %d %+v", res2.StatusCode, created)
	}
	id, _ := created["id"].(string)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+id, nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var view map[string]any
		_ = decodeJSON(gr, &view)
		if view["pending"] == float64(0) {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("file batch did not finish")
}

func TestFileBatchRejections(t *testing.T) {
	_, _, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	ts := newHarnessJobs(t, mURL, fURL)
	defer ts.Close()

	res, _ := postMultipart(t, ts.URL+"/api/batches/from-file", map[string]string{
		"instruction": "no blank",
	}, "x.txt", []byte("hi"))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}

	res, body := postMultipart(t, ts.URL+"/api/batches/from-file", map[string]string{
		"instruction": "{{chunk}}",
	}, "x.pdf", []byte("%PDF"))
	if res.StatusCode != 415 {
		t.Fatalf("pdf status %d", res.StatusCode)
	}
	if errObj, ok := body["error"].(map[string]any); !ok || !strings.Contains(errObj["message"].(string), "PDF") {
		t.Fatalf("%+v", body)
	}
}

func TestFileBatchReduce(t *testing.T) {
	replyN := 0
	heavy := &fakeollama.Server{Models: []string{"qwen2.5:14b"}}
	heavy.Reply = func(call fakeollama.Call) []fakeollama.Step {
		replyN++
		if replyN <= 2 {
			return []fakeollama.Step{{Content: fmt.Sprintf("part-%d", replyN)}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
		}
		return []fakeollama.Step{{Content: "combined"}, {Done: &fakeollama.DoneStats{EvalCount: 2}}}
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts := newHarnessJobs(t, hTs.URL, fTs.URL)
	defer ts.Close()

	para := strings.Repeat("word ", 120)
	text := para + ".\n\n" + para + "."
	reduce := `{"instruction":"Combine:\n{{results}}","machine":"firmino","model":"qwen2.5:14b"}`
	res, created := postMultipart(t, ts.URL+"/api/batches/from-file", map[string]string{
		"instruction": "Go: {{chunk}}",
		"reduce":      reduce,
		"chunk_chars": "500",
	}, "doc.md", []byte(text))
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("%d %+v", res.StatusCode, created)
	}
	id := created["id"].(string)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+id, nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var view map[string]any
		_ = decodeJSON(gr, &view)
		rd, _ := view["reduce"].(map[string]any)
		if rd != nil && rd["status"] == "done" {
			if len(heavy.Calls()) < 3 {
				t.Fatalf("expected reduce call, got %d", len(heavy.Calls()))
			}
			last := heavy.Calls()[len(heavy.Calls())-1]
			if !strings.Contains(last.Messages[0].Content, "### Part 1") {
				t.Fatal("missing parts in reduce prompt")
			}
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("reduce did not complete")
}
