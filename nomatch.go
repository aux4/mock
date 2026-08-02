package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// buildNoMatchBody builds the diagnostic 404 body for an unmatched request. It
// lists every registered stub (method + path) and, when it can, adds a "hint"
// that points at the most likely mistake — most importantly the /api-prefix
// gotcha (the mock serves under /api, so stub --path must omit it). The returned
// hint string is also used for the server.log near-miss line.
func buildNoMatchBody(stubs []Stub, method, path string) (body string, hint string) {
	registered := make([]string, 0, len(stubs))
	for _, s := range stubs {
		registered = append(registered, methodLabel(s.Method)+" "+s.Path)
	}

	hint = computeHint(stubs, method, path)

	payload := map[string]interface{}{
		"error":           "no matching stub",
		"method":          method,
		"path":            path,
		"registeredStubs": registered,
	}
	if hint != "" {
		payload["hint"] = hint
	}
	b, _ := json.Marshal(payload)
	return string(b), hint
}

// computeHint returns a human-readable "did you mean" hint for an unmatched
// request, or "" when nothing plausible is close. Detection order (most useful
// first): the /api-prefix mistake on the requested path, the /api-prefix mistake
// on a stub path, a same-path method mismatch, then the closest path by edit
// distance.
func computeHint(stubs []Stub, method, path string) string {
	// 1. The requested path carries an /api prefix and the remainder matches a
	//    registered stub — the #1 gotcha (mock serves under /api).
	if rem, ok := stripAPIPrefix(path); ok {
		for _, s := range stubs {
			if matched, _ := compilePathPattern(s.Path).matchPath(rem); matched {
				return fmt.Sprintf(
					"did you mean %s %s? (you requested %s %s — the mock serves under /api, so the stub --path should omit the /api prefix)",
					methodLabel(s.Method), s.Path, method, path)
			}
		}
	}

	// 2. A stub path carries an /api prefix and its remainder matches the
	//    requested path — the author put /api into --path by mistake.
	for _, s := range stubs {
		if rem, ok := stripAPIPrefix(s.Path); ok {
			if matched, _ := compilePathPattern(rem).matchPath(path); matched {
				return fmt.Sprintf(
					"stub %s %s has an /api prefix — the mock already serves under /api, so define it as %s %s",
					methodLabel(s.Method), s.Path, methodLabel(s.Method), rem)
			}
		}
	}

	// 3. A stub's path matches but it expects a different method.
	for _, s := range stubs {
		if matched, _ := compilePathPattern(s.Path).matchPath(path); matched {
			if s.Method != "" && s.Method != "ANY" && !strings.EqualFold(s.Method, method) {
				return fmt.Sprintf(
					"did you mean %s %s? (a stub matches this path but expects the %s method, not %s)",
					s.Method, s.Path, s.Method, method)
			}
		}
	}

	// 4. Closest registered path by edit distance, when reasonably near.
	best, bestMethod, bestDist := "", "", -1
	for _, s := range stubs {
		d := levenshtein(path, s.Path)
		if bestDist < 0 || d < bestDist {
			bestDist, best, bestMethod = d, s.Path, methodLabel(s.Method)
		}
	}
	if best != "" && bestDist >= 0 && bestDist <= len(path)/2+2 {
		return fmt.Sprintf("did you mean %s %s?", bestMethod, best)
	}
	return ""
}

// methodLabel renders a stub method for display, defaulting a blank method to
// ANY (matches the on-the-wire semantics).
func methodLabel(m string) string {
	if m == "" {
		return "ANY"
	}
	return m
}

// stripAPIPrefix removes a leading /api segment from a normalized path. Returns
// the remainder (with its leading slash) and true when the prefix was present.
func stripAPIPrefix(p string) (string, bool) {
	if p == "/api" {
		return "/", true
	}
	if strings.HasPrefix(p, "/api/") {
		return p[len("/api"):], true
	}
	return "", false
}

// logNearMiss appends an unmatched-request line to the server's log so a test
// author can tail it. Matches never log (this is only called on the 404 path).
func logNearMiss(stateDir, method, path, hint string) {
	f, err := os.OpenFile(serverLogPath(stateDir), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	line := fmt.Sprintf("[%s] mock: no matching stub for %s %s", nowTs(), method, path)
	if hint != "" {
		line += " | hint: " + hint
	}
	fmt.Fprintln(f, line)
}

// levenshtein is the standard edit distance between two strings, used to pick
// the closest registered path for a "did you mean" hint.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur := make([]int, lb+1)
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
