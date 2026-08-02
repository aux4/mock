package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Stub is a single programmed response. The optional Match field is reserved
// for future request-matching predicates (header/query/body) and is not used
// by the current exact method+path matcher.
type Stub struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body"`
	Match   *StubMatch        `json:"match,omitempty"`
}

// StubMatch is a reserved placeholder for MOCK-003 predicates.
type StubMatch struct {
	Headers map[string]string `json:"headers,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
	Body    string            `json:"body,omitempty"`
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

func stubsPath(stateDir string) string    { return filepath.Join(stateDir, "stubs.json") }
func requestsPath(stateDir string) string { return filepath.Join(stateDir, "requests.json") }
func pidPath(stateDir string) string      { return filepath.Join(stateDir, "server.pid") }
func lockPath(stateDir string) string     { return filepath.Join(stateDir, ".lock") }

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
