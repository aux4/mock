package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// apiGatewayEvent is the subset of the AWS-API-Gateway event that aux4/api
// delivers on stdin to a mounted command.
type apiGatewayEvent struct {
	HTTPMethod     string            `json:"httpMethod"`
	PathParameters map[string]string `json:"pathParameters"`
	Query          map[string]string `json:"queryStringParameters"`
	Headers        map[string]string `json:"headers"`
	Body           string            `json:"body"`
}

// apiGatewayResponse is the shape aux4/api turns into an HTTP response.
type apiGatewayResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

func writeResponse(resp apiGatewayResponse) {
	if resp.Headers == nil {
		resp.Headers = map[string]string{}
	}
	out, _ := json.Marshal(resp)
	fmt.Println(string(out))
}

// runRespond is the mounted catch-all handler. It reads the event from stdin,
// records the request, finds the first matching stub, and prints the response.
// It must never exit non-zero on bad state — that would surface as a 500.
func runRespond(stateDir string) {
	if stateDir == "" {
		writeResponse(apiGatewayResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"mock respond: --stateDir is required"}`,
		})
		return
	}

	raw, _ := io.ReadAll(os.Stdin)
	var event apiGatewayEvent
	_ = json.Unmarshal(raw, &event)

	method := event.HTTPMethod
	if method == "" {
		method = "GET"
	}
	path := "/"
	if event.PathParameters != nil {
		if p, ok := event.PathParameters["path"]; ok {
			path = normalizePath(p)
		}
	}

	// Record the request first so verify sees even unmatched calls.
	_ = appendRequest(stateDir, RecordedRequest{
		Method:  method,
		Path:    path,
		Query:   event.Query,
		Headers: event.Headers,
		Body:    event.Body,
		Ts:      nowTs(),
	})

	stubs := loadStubs(stateDir)
	// Evaluate most-specific-first: a stub with more predicates outranks one
	// with fewer, and a no-predicate stub is the lowest-priority fallback. The
	// sort is stable, so equally specific stubs keep their definition order
	// (preserving the original first-match-wins behavior for plain stubs).
	sort.SliceStable(stubs, func(i, j int) bool {
		return stubs[i].Match.predicateCount() > stubs[j].Match.predicateCount()
	})
	for _, stub := range stubs {
		if stubMatches(stub, method, path, event) {
			headers := stub.Headers
			if headers == nil {
				headers = map[string]string{}
			}
			status := stub.Status
			if status == 0 {
				status = 200
			}
			writeResponse(apiGatewayResponse{
				StatusCode: status,
				Headers:    headers,
				Body:       stub.Body,
			})
			return
		}
	}

	// No match: 404, but the request was still recorded above.
	body, _ := json.Marshal(map[string]string{
		"error":  "no matching stub",
		"method": method,
		"path":   path,
	})
	writeResponse(apiGatewayResponse{
		StatusCode: 404,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	})
}

// stubMatches reports whether a stub should serve this request. Method and path
// must match, and every predicate in the stub's match (if any) must hold. A
// stub with no match matches any request to its method+path.
func stubMatches(stub Stub, method, path string, event apiGatewayEvent) bool {
	if stub.Path != path {
		return false
	}
	if !(stub.Method == "" || stub.Method == "ANY" || stub.Method == method) {
		return false
	}
	return matchPredicates(stub.Match, event)
}

// matchPredicates evaluates a stub's request-matching predicates against the
// incoming event. A nil match always holds (fallback stub).
func matchPredicates(m *StubMatch, event apiGatewayEvent) bool {
	if m == nil {
		return true
	}
	for k, v := range m.Headers {
		hv, ok := lookupHeader(event.Headers, k)
		if !ok || !valueMatches(v, hv) {
			return false
		}
	}
	for k, v := range m.Query {
		qv, ok := event.Query[k]
		if !ok || !valueMatches(v, qv) {
			return false
		}
	}
	if m.BodyContains != "" && !strings.Contains(event.Body, m.BodyContains) {
		return false
	}
	return true
}

// lookupHeader finds a header by name case-insensitively (HTTP header names are
// case-insensitive and aux4/api may deliver them in any case).
func lookupHeader(headers map[string]string, key string) (string, bool) {
	for hk, hv := range headers {
		if strings.EqualFold(hk, key) {
			return hv, true
		}
	}
	return "", false
}

// valueMatches compares a predicate value against an actual value. A trailing
// "*" in the pattern is a wildcard: everything before it must be a prefix of the
// actual value (e.g. "Bearer *" matches "Bearer abc123"). Otherwise it is exact
// equality.
func valueMatches(pattern, actual string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(actual, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == actual
}
