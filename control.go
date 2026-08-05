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
	name := fs.String("name", "", "stable handle of the server (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	method := fs.String("method", "ANY", "HTTP method to match (ANY = any)")
	path := fs.String("path", "", "request path to match (required)")
	status := fs.Int("status", 200, "response status code")
	body := fs.String("body", "", "inline response body")
	bodyFile := fs.String("bodyFile", "", "file to read response body from")
	contentType := fs.String("contentType", "", "response Content-Type")
	var headers stringList
	fs.Var(&headers, "header", "response header key=value (repeatable)")
	var whenHeaders stringList
	fs.Var(&whenHeaders, "when-header", "request header predicate key=value, trailing * wildcard (repeatable)")
	var whenQueries stringList
	fs.Var(&whenQueries, "when-query", "request query predicate key=value (repeatable)")
	whenBodyContains := fs.String("when-body-contains", "", "request body substring predicate")
	_ = fs.Parse(args)

	if *path == "" {
		fmt.Fprintln(os.Stderr, "Error: --path is required")
		os.Exit(1)
	}

	dir := resolveStateDir(*stateDir, *name, *port)
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

	// Build the request-matching predicates. A stub with no predicates keeps a
	// nil match and acts as the fallback for its method+path.
	match := &StubMatch{
		Headers:      parseHeaderPairs(whenHeaders),
		Query:        parseHeaderPairs(whenQueries),
		BodyContains: *whenBodyContains,
	}
	if match.predicateCount() == 0 {
		match = nil
	}

	stub := Stub{
		Method:  strings.ToUpper(*method),
		Path:    normalizePath(*path),
		Status:  *status,
		Headers: respHeaders,
		Body:    resolvedBody,
		Match:   match,
	}

	unlock, err := acquireLock(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error acquiring lock: %v\n", err)
		os.Exit(1)
	}
	defer unlock()

	stubs := upsertStub(loadStubs(dir), stub)
	if err := saveStubs(dir, stubs); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving stub: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("stub %s %s -> %d\n", stub.Method, stub.Path, stub.Status)
}

// upsertStub inserts stub into stubs, replacing an existing stub that shares the
// same replace key (method + path + predicate set) or appending it otherwise.
// Replace only when method, path AND the predicate set are identical; otherwise
// stubs on the same method+path coexist (e.g. one gated on a header, one bare
// fallback).
func upsertStub(stubs []Stub, stub Stub) []Stub {
	sig := matchSignature(stub.Match)
	for i, s := range stubs {
		if s.Method == stub.Method && s.Path == stub.Path && matchSignature(s.Match) == sig {
			stubs[i] = stub
			return stubs
		}
	}
	return append(stubs, stub)
}

// registerStubs applies one or more stubs to the store under a single lock,
// reusing the same replace-by-key semantics as `mock stub`. It never clears
// existing stubs, so presets and later `mock stub` calls layer on top of each
// other.
func registerStubs(dir string, newStubs []Stub) error {
	unlock, err := acquireLock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	stubs := loadStubs(dir)
	for _, s := range newStubs {
		stubs = upsertStub(stubs, s)
	}
	return saveStubs(dir, stubs)
}

// runReset clears the stub store and/or the request journal.
func runReset(args []string) {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	port := fs.String("port", "7070", "server port (derives stateDir)")
	name := fs.String("name", "", "stable handle of the server (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	stubs := fs.String("stubs", "", "clear stubs only (true)")
	requests := fs.String("requests", "", "clear requests only (true)")
	_ = fs.Parse(args)

	dir := resolveStateDir(*stateDir, *name, *port)
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
		// Command stubs are cleared with the HTTP ones: a leftover `replace` hook keeps a
		// real command from running, so a stub must never outlive the reset that retires it.
		_ = saveCommandStubs(dir, []CommandStub{})
		_ = writeCommandsAux4(dir, []CommandStub{})
	}
	if clearRequests {
		_ = saveRequests(dir, []RecordedRequest{})
		_ = saveInvocations(dir, []RecordedInvocation{})
	}
	fmt.Println("reset ok")
}

// runRequests prints recorded requests as a JSON array, optionally filtered.
func runRequests(args []string) {
	fs := flag.NewFlagSet("requests", flag.ExitOnError)
	port := fs.String("port", "7070", "server port (derives stateDir)")
	name := fs.String("name", "", "stable handle of the server (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	method := fs.String("method", "", "filter by method")
	path := fs.String("path", "", "filter by path")
	_ = fs.Parse(args)

	dir := resolveStateDir(*stateDir, *name, *port)
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
	name := fs.String("name", "", "stable handle of the server (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	method := fs.String("method", "", "match by method")
	path := fs.String("path", "", "match by path")
	timesStr := fs.String("times", "", "exact number of matches (omit = at least 1)")
	var bodyContains stringList
	fs.Var(&bodyContains, "body-contains", "require request body substring (repeatable, AND)")
	var headers stringList
	fs.Var(&headers, "header", "require request header key=value (repeatable, AND)")
	_ = fs.Parse(args)

	dir := resolveStateDir(*stateDir, *name, *port)
	reqs := loadRequests(dir)
	wantHeaders := parseHeaderPairs(headers)
	wantBody := nonEmpty(bodyContains)

	// Compile the --path once so verify accepts the same {name}/{name...} tokens
	// the stub matcher uses. A concrete path compiles to an anchored literal and
	// still matches exactly (back-compat); a parametric path (e.g. /2/tweets/{id})
	// matches any recorded path in that shape.
	var pathPat *pathPattern
	if *path != "" {
		pathPat = compilePathPattern(normalizePath(*path))
	}

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
		if pathPat != nil {
			if ok, _ := pathPat.matchPath(r.Path); !ok {
				continue
			}
		}
		if !bodyContainsAll(r.Body, wantBody) {
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
	fmt.Fprintf(os.Stderr, "criteria: method=%q path=%q body-contains=%v headers=%v\n",
		*method, *path, wantBody, wantHeaders)

	// Point at the substrings that no candidate request body contained. Candidate
	// = a recorded request that matched the method/path/header criteria, so the
	// hint stays focused on why the body predicate failed rather than every stub.
	if missing := missingBodySubstrings(reqs, *method, pathPat, wantHeaders, wantBody); len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "missing body substring(s): %v\n", missing)
	}

	fmt.Fprintf(os.Stderr, "recorded %d request(s):\n", len(reqs))
	for _, r := range reqs {
		fmt.Fprintf(os.Stderr, "  %s %s body=%s\n", r.Method, r.Path, r.Body)
	}
	os.Exit(1)
}

// nonEmpty drops blank entries so an empty --body-contains passed by aux4 (the
// variable default) does not become a predicate that can never match.
func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// bodyContainsAll reports whether body contains every wanted substring (AND).
func bodyContainsAll(body string, want []string) bool {
	for _, sub := range want {
		if !strings.Contains(body, sub) {
			return false
		}
	}
	return true
}

// missingBodySubstrings returns the wanted substrings that no candidate request
// body contained. A candidate is a recorded request that satisfied the
// method/path/header criteria, so the diagnostic names only the body substrings
// that actually blocked the match.
func missingBodySubstrings(reqs []RecordedRequest, method string, pathPat *pathPattern, wantHeaders map[string]string, wantBody []string) []string {
	missing := make([]string, 0, len(wantBody))
	for _, sub := range wantBody {
		found := false
		for _, r := range reqs {
			if method != "" && !strings.EqualFold(r.Method, method) {
				continue
			}
			if pathPat != nil {
				if ok, _ := pathPat.matchPath(r.Path); !ok {
					continue
				}
			}
			if !headersContain(r.Headers, wantHeaders) {
				continue
			}
			if strings.Contains(r.Body, sub) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, sub)
		}
	}
	return missing
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
