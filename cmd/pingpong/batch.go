package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

func runBatch(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pingpong batch submit|get --url http://127.0.0.1:8787 ...")
		return 2
	}
	switch args[0] {
	case "submit":
		return batchSubmit(args[1:])
	case "get":
		return batchGet(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown batch command %q\n", args[0])
		return 2
	}
}

func batchSubmit(args []string) int {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	url := fs.String("url", "http://127.0.0.1:8787", "router base URL")
	file := fs.String("file", "", "JSON file with {\"items\":[...]}")
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
