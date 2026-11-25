package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func runBatch(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pingpong batch submit|get|cancel --url http://127.0.0.1:8787 ...")
		return 2
	}
	switch args[0] {
	case "submit":
		return batchSubmit(args[1:])
	case "get":
		return batchGet(args[1:])
	case "cancel":
		return batchCancel(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown batch command %q\n", args[0])
		return 2
	}
}

func batchSubmit(args []string) int {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	url := fs.String("url", "http://127.0.0.1:8787", "router base URL")
	file := fs.String("file", "", "JSON file with {\"items\":[...]}")
	splitSpec := fs.String("split", "", "comma-separated machine:model pairs for a split batch")
	assign := fs.String("assign", "round_robin", "split assign mode: round_robin or weights")
	weights := fs.String("weights", "", "comma-separated weights for each split target (with --assign weights)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var body []byte
	var err error
	if *file == "" || *file == "-" {
		body, err = io.ReadAll(os.Stdin)
	} else {
		body, err = os.ReadFile(*file)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		return 1
	}
	if *splitSpec != "" {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			fmt.Fprintf(os.Stderr, "json: %v\n", err)
			return 1
		}
		targets, err := parseSplitTargets(*splitSpec, *assign, *weights)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 2
		}
		payload["split"] = map[string]any{"assign": *assign, "targets": targets}
		body, err = json.Marshal(payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "json: %v\n", err)
			return 1
		}
	}
	res, err := http.Post(*url+"/api/batches", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "post: %v\n", err)
		return 1
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	fmt.Println(string(out))
	if res.StatusCode >= 300 {
		return 1
	}
	return 0
}

func parseSplitTargets(spec, assign, weights string) ([]map[string]any, error) {
	parts := strings.Split(spec, ",")
	out := make([]map[string]any, 0, len(parts))
	wparts := []string{}
	if weights != "" {
		wparts = strings.Split(weights, ",")
		if assign != "weights" {
			return nil, fmt.Errorf("--weights requires --assign weights")
		}
		if len(wparts) != len(parts) {
			return nil, fmt.Errorf("need one weight per split target")
		}
	}
	for i, p := range parts {
		p = strings.TrimSpace(p)
		idx := strings.Index(p, ":")
		if idx <= 0 {
			return nil, fmt.Errorf("split target %q: want machine:model", p)
		}
		t := map[string]any{
			"machine": strings.TrimSpace(p[:idx]),
			"model":   strings.TrimSpace(p[idx+1:]),
		}
		if assign == "weights" {
			w, err := strconv.Atoi(strings.TrimSpace(wparts[i]))
			if err != nil || w < 1 {
				return nil, fmt.Errorf("invalid weight %q", wparts[i])
			}
			t["weight"] = w
		}
		out = append(out, t)
	}
	return out, nil
}

func batchGet(args []string) int {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	url := fs.String("url", "http://127.0.0.1:8787", "router base URL")
	id := fs.String("id", "", "batch id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" && fs.NArg() > 0 {
		*id = fs.Arg(0)
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "batch id required")
		return 2
	}
	res, err := http.Get(*url + "/api/batches/" + *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get: %v\n", err)
		return 1
	}
	defer res.Body.Close()
	var pretty any
	if err := json.NewDecoder(res.Body).Decode(&pretty); err != nil {
		fmt.Fprintf(os.Stderr, "decode: %v\n", err)
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(pretty)
	if res.StatusCode >= 300 {
		return 1
	}
	return 0
}

func batchCancel(args []string) int {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	url := fs.String("url", "http://127.0.0.1:8787", "router base URL")
	id := fs.String("id", "", "batch id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" && fs.NArg() > 0 {
		*id = fs.Arg(0)
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "batch id required")
		return 2
	}
	res, err := http.Post(*url+"/api/batches/"+*id+"/cancel", "application/json", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "post: %v\n", err)
		return 1
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	fmt.Println(string(out))
	if res.StatusCode >= 300 {
		return 1
	}
	return 0
}
