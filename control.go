package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// stringList collects repeatable flag values (e.g. --header a --header b).
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func parseHeaderPairs(pairs []string) map[string]string {
	out := map[string]string{}
	for _, p := range pairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			out[strings.TrimSpace(kv[0])] = kv[1]
		}
	}
	return out
}

// runStub defines or replaces a stub, matched by (method, path).
func runStub(args []string) {
	fs := flag.NewFlagSet("stub", flag.ExitOnError)
	port := fs.String("port", "7070", "server port (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	method := fs.String("method", "ANY", "HTTP method to match (ANY = any)")
	path := fs.String("path", "", "request path to match (required)")
	status := fs.Int("status", 200, "response status code")
	body := fs.String("body", "", "inline response body")
	bodyFile := fs.String("bodyFile", "", "file to read response body from")
	contentType := fs.String("contentType", "", "response Content-Type")
	var headers stringList
	fs.Var(&headers, "header", "response header key=value (repeatable)")
	_ = fs.Parse(args)

	if *path == "" {
		fmt.Fprintln(os.Stderr, "Error: --path is required")
		os.Exit(1)
	}

	dir := resolveStateDir(*stateDir, *port)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create state dir: %v\n", err)
		os.Exit(1)
	}

	// Resolve body: inline > file > stdin.
	resolvedBody := *body
	bodyProvided := *body != ""
	if !bodyProvided && *bodyFile != "" {
		data, err := os.ReadFile(*bodyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading --bodyFile: %v\n", err)
			os.Exit(1)
		}
		resolvedBody = string(data)
		bodyProvided = true
	}
	if !bodyProvided {
		if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			data, _ := readAllStdin()
			if len(data) > 0 {
				resolvedBody = string(data)
			}
		}
	}

	respHeaders := parseHeaderPairs(headers)
	ct := *contentType
	if ct == "" && looksLikeJSON(resolvedBody) {
		ct = "application/json"
	}
	if ct != "" {
		respHeaders["Content-Type"] = ct
	}

	stub := Stub{
		Method:  strings.ToUpper(*method),
		Path:    normalizePath(*path),
		Status:  *status,
		Headers: respHeaders,
		Body:    resolvedBody,
	}

	unlock, err := acquireLock(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error acquiring lock: %v\n", err)
		os.Exit(1)
	}
	defer unlock()

	stubs := loadStubs(dir)
	replaced := false
	for i, s := range stubs {
		if s.Method == stub.Method && s.Path == stub.Path {
			stubs[i] = stub
			replaced = true
			break
		}
	}
	if !replaced {
		stubs = append(stubs, stub)
	}
	if err := saveStubs(dir, stubs); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving stub: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("stub %s %s -> %d\n", stub.Method, stub.Path, stub.Status)
}

// runReset clears the stub store and/or the request journal.
func runReset(args []string) {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	port := fs.String("port", "7070", "server port (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	stubs := fs.String("stubs", "", "clear stubs only (true)")
	requests := fs.String("requests", "", "clear requests only (true)")
	_ = fs.Parse(args)

	dir := resolveStateDir(*stateDir, *port)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create state dir: %v\n", err)
		os.Exit(1)
	}

	clearStubs := *stubs == "true"
	clearRequests := *requests == "true"
	if !clearStubs && !clearRequests {
		clearStubs = true
		clearRequests = true
	}

	unlock, err := acquireLock(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error acquiring lock: %v\n", err)
		os.Exit(1)
	}
	defer unlock()

	if clearStubs {
		_ = saveStubs(dir, []Stub{})
	}
	if clearRequests {
		_ = saveRequests(dir, []RecordedRequest{})
	}
	fmt.Println("reset ok")
}

// runRequests prints recorded requests as a JSON array, optionally filtered.
func runRequests(args []string) {
	fs := flag.NewFlagSet("requests", flag.ExitOnError)
	port := fs.String("port", "7070", "server port (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	method := fs.String("method", "", "filter by method")
	path := fs.String("path", "", "filter by path")
	_ = fs.Parse(args)

	dir := resolveStateDir(*stateDir, *port)
	reqs := loadRequests(dir)

	filtered := make([]RecordedRequest, 0, len(reqs))
	for _, r := range reqs {
		if *method != "" && !strings.EqualFold(r.Method, *method) {
			continue
		}
		if *path != "" && r.Path != normalizePath(*path) {
			continue
		}
		filtered = append(filtered, r)
	}

	out, _ := json.MarshalIndent(filtered, "", "  ")
	fmt.Println(string(out))
}

// runVerify asserts that a matching request was recorded. Exit 0 if the
// assertion holds, non-zero with a diagnostic on stderr otherwise.
func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	port := fs.String("port", "7070", "server port (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	method := fs.String("method", "", "match by method")
	path := fs.String("path", "", "match by path")
	timesStr := fs.String("times", "", "exact number of matches (omit = at least 1)")
	bodyContains := fs.String("body-contains", "", "require request body substring")
	var headers stringList
	fs.Var(&headers, "header", "require request header key=value (repeatable)")
	_ = fs.Parse(args)

	dir := resolveStateDir(*stateDir, *port)
	reqs := loadRequests(dir)
	wantHeaders := parseHeaderPairs(headers)

	times := -1
	if strings.TrimSpace(*timesStr) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(*timesStr))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: --times must be an integer, got %q\n", *timesStr)
			os.Exit(2)
		}
		times = n
	}

	matches := 0
	for _, r := range reqs {
		if *method != "" && !strings.EqualFold(r.Method, *method) {
			continue
		}
		if *path != "" && r.Path != normalizePath(*path) {
			continue
		}
		if *bodyContains != "" && !strings.Contains(r.Body, *bodyContains) {
			continue
		}
		if !headersContain(r.Headers, wantHeaders) {
			continue
		}
		matches++
	}

	ok := false
	if times >= 0 {
		ok = matches == times
	} else {
		ok = matches >= 1
	}

	if ok {
		fmt.Printf("verify ok: %d match(es)\n", matches)
		return
	}

	expected := "at least 1"
	if times >= 0 {
		expected = fmt.Sprintf("exactly %d", times)
	}
	fmt.Fprintf(os.Stderr, "verify failed: expected %s match(es), found %d\n", expected, matches)
	fmt.Fprintf(os.Stderr, "criteria: method=%q path=%q body-contains=%q headers=%v\n",
		*method, *path, *bodyContains, wantHeaders)
	fmt.Fprintf(os.Stderr, "recorded %d request(s):\n", len(reqs))
	for _, r := range reqs {
		fmt.Fprintf(os.Stderr, "  %s %s body=%s\n", r.Method, r.Path, r.Body)
	}
	os.Exit(1)
}

func headersContain(have, want map[string]string) bool {
	for k, v := range want {
		found := false
		for hk, hv := range have {
			if strings.EqualFold(hk, k) && hv == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func looksLikeJSON(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	return (strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}")) ||
		(strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]"))
}
