package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Stub is a single programmed response. The optional Match field holds
// request-matching predicates (header/query/body substring). A stub with no
// Match is a fallback that matches any request to its method+path; a stub with
// predicates only matches when every predicate holds and is evaluated with
// higher priority than fallbacks.
type Stub struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body"`
	Match   *StubMatch        `json:"match,omitempty"`
}

// StubMatch holds the request-matching predicates for a stub (MOCK-003).
// Headers/Query are exact key=value equality (case-insensitive header name,
// trailing "*" wildcard on the value). BodyContains is a request-body
// substring. All present predicates must hold for the stub to match.
type StubMatch struct {
	Headers      map[string]string `json:"headers,omitempty"`
	Query        map[string]string `json:"query,omitempty"`
	BodyContains string            `json:"bodyContains,omitempty"`
}

// predicateCount reports how many predicates this match specifies. More
// predicates == more specific == higher priority when a request could match
// several stubs. A nil match (fallback) has zero predicates.
func (m *StubMatch) predicateCount() int {
	if m == nil {
		return 0
	}
	n := len(m.Headers) + len(m.Query)
	if m.BodyContains != "" {
		n++
	}
	return n
}

// matchSignature is a stable serialization of a stub's predicates, used as part
// of the replace key so stubs on the same method+path but different predicates
// coexist while an identical predicate set replaces in place. A nil match
// yields the empty signature (the fallback slot).
func matchSignature(m *StubMatch) string {
	if m == nil || m.predicateCount() == 0 {
		return ""
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// RecordedRequest is one request the server observed.
type RecordedRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   map[string]string `json:"query,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body"`
	Ts      string            `json:"ts"`
}

func nowTs() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// resolveStateDir returns the state directory for the given explicit stateDir
// or, when empty, derives it deterministically from the port so every command
// (control plane and data plane) agrees on the same location.
func resolveStateDir(stateDir, port string) string {
	if stateDir != "" {
		abs, err := filepath.Abs(stateDir)
		if err == nil {
			return abs
		}
		return stateDir
	}
	base := os.Getenv("TMPDIR")
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(base, "aux4-mock", port)
}

func stubsPath(stateDir string) string     { return filepath.Join(stateDir, "stubs.json") }
func requestsPath(stateDir string) string  { return filepath.Join(stateDir, "requests.json") }
func pidPath(stateDir string) string       { return filepath.Join(stateDir, "server.pid") }
func lockPath(stateDir string) string      { return filepath.Join(stateDir, ".lock") }
func serverLogPath(stateDir string) string { return filepath.Join(stateDir, "server.log") }

// normalizePath collapses any leading slashes to exactly one so that an
// incoming event path ("gmail/v1/x", no leading slash) and a stub path
// ("/gmail/v1/x") compare equal.
func normalizePath(p string) string {
	p = strings.TrimLeft(p, "/")
	return "/" + p
}

// acquireLock is a portable spin-lock using an exclusive-create lock file.
// It avoids syscall.Flock so the binary cross-compiles to Windows unchanged.
func acquireLock(stateDir string) (func(), error) {
	lp := lockPath(stateDir)
	deadline := time.Now().Add(3 * time.Second)
	for {
		f, err := os.OpenFile(lp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			f.Close()
			return func() { os.Remove(lp) }, nil
		}
		if time.Now().After(deadline) {
			// Stale lock recovery: take it over rather than deadlock forever.
			os.Remove(lp)
			continue
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// loadStubs reads stubs.json. A missing or malformed file yields an empty
// slice and no error, so a corrupt state file never 500s the server.
func loadStubs(stateDir string) []Stub {
	data, err := os.ReadFile(stubsPath(stateDir))
	if err != nil {
		return []Stub{}
	}
	var stubs []Stub
	if err := json.Unmarshal(data, &stubs); err != nil {
		return []Stub{}
	}
	return stubs
}

func saveStubs(stateDir string, stubs []Stub) error {
	data, err := json.MarshalIndent(stubs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stubsPath(stateDir), data, 0600)
}

// loadRequests reads requests.json. Missing or malformed yields an empty slice.
func loadRequests(stateDir string) []RecordedRequest {
	data, err := os.ReadFile(requestsPath(stateDir))
	if err != nil {
		return []RecordedRequest{}
	}
	var reqs []RecordedRequest
	if err := json.Unmarshal(data, &reqs); err != nil {
		return []RecordedRequest{}
	}
	return reqs
}

func saveRequests(stateDir string, reqs []RecordedRequest) error {
	data, err := json.MarshalIndent(reqs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(requestsPath(stateDir), data, 0600)
}

// appendRequest records one request under a lock so concurrent respond
// processes don't clobber each other's writes.
func appendRequest(stateDir string, req RecordedRequest) error {
	unlock, err := acquireLock(stateDir)
	if err != nil {
		return err
	}
	defer unlock()
	reqs := loadRequests(stateDir)
	reqs = append(reqs, req)
	return saveRequests(stateDir, reqs)
}
