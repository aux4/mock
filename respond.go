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
	// Compile each stub path once so we can both match parametric paths and rank
	// stubs by specificity.
	ranked := make([]rankedStub, len(stubs))
	for i, s := range stubs {
		ranked[i] = rankedStub{stub: s, pat: compilePathPattern(s.Path)}
	}
	// Evaluate most-specific-first. Specificity is, in order:
	//   1. more STATIC path segments wins (an exact /2/tweets/123 beats /2/tweets/{id}),
	//   2. a path without a greedy {name...} wildcard beats one with it
	//      (/2/tweets/{id} beats /2/tweets/{rest...}),
	//   3. more request-matching predicates wins (MOCK-003),
	//   4. declaration order (the sort is stable), preserving the original
	//      first-match-wins behavior for otherwise-equal stubs.
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.pat.staticSegs != b.pat.staticSegs {
			return a.pat.staticSegs > b.pat.staticSegs
		}
		if a.pat.hasGreedy != b.pat.hasGreedy {
			return !a.pat.hasGreedy // a non-greedy path is more specific
		}
		return a.stub.Match.predicateCount() > b.stub.Match.predicateCount()
	})
	for _, r := range ranked {
		ok, params := matchStub(r.stub, r.pat, method, path, event)
		if !ok {
			continue
		}
		status := r.stub.Status
		if status == 0 {
			status = 200
		}
		// Interpolate request data into the response body and header values.
		headers := map[string]string{}
		for k, v := range r.stub.Headers {
			headers[k] = interpolate(v, params, event, path)
		}
		// Apply side effects (MOCK-STATE-001) before responding: a stub with
		// `then` upserts one or more follow-up stubs into the store, built by
		// interpolating THIS request into each one, so a later request (e.g. the
		// GET after a DELETE) observes the change. The triggering response itself
		// is unaffected by this.
		if len(r.stub.Then) > 0 {
			applyThen(stateDir, r.stub.Then, params, event, path)
		}
		writeResponse(apiGatewayResponse{
			StatusCode: status,
			Headers:    headers,
			Body:       interpolate(r.stub.Body, params, event, path),
		})
		return
	}

	// No match: a diagnostic 404 (the request was still recorded above). The
	// body lists the registered stubs and, when possible, a "did you mean" hint
	// — most importantly the /api-prefix gotcha. The near-miss is also logged.
	body, hint := buildNoMatchBody(stubs, method, path)
	logNearMiss(stateDir, method, path, hint)
	writeResponse(apiGatewayResponse{
		StatusCode: 404,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       body,
	})
}

// rankedStub pairs a stub with its compiled path pattern so respond can match
// and rank it without recompiling.
type rankedStub struct {
	stub Stub
	pat  *pathPattern
}

// matchStub reports whether a stub should serve this request and returns any
// captured path params. Method and path (which may contain {name}/{name...}
// tokens) must match, and every predicate in the stub's match (if any) must
// hold. A stub with no match matches any request to its method+path.
func matchStub(stub Stub, pat *pathPattern, method, path string, event apiGatewayEvent) (bool, map[string]string) {
	if !(stub.Method == "" || stub.Method == "ANY" || stub.Method == method) {
		return false, nil
	}
	ok, params := pat.matchPath(path)
	if !ok {
		return false, nil
	}
	if !matchPredicates(stub.Match, event) {
		return false, nil
	}
	return true, params
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

// applyThen builds and upserts the follow-up stubs attached to the stub that
// just served this request. Each then-stub's method/path/headers/body is
// interpolated against the SAME request (params/event/path) that triggered it
// — the identical ${path.x}/${query.x}/${header.x}/${body}/${body.a.b}
// placeholders a response body supports. A then-stub that itself carries
// `${...}` placeholders meant to survive for ITS OWN future responses (once it
// becomes an ordinary stored stub) must write them as `$${...}`: this first
// interpolation pass collapses `$${` to a literal `${`, and the stub serves
// that literal text verbatim until a later request interpolates it again. The
// `match` predicates and any nested `then` of a then-stub are stored as-is
// (not interpolated) — they describe how/whether THAT stub fires later, not
// data from the current request. Upsert reuses the normal method+path+predicate
// replace key, so a then-stub naturally replaces a prior stub standing at the
// same slot (e.g. the GET that previously returned the now-deleted resource).
func applyThen(stateDir string, thenStubs []Stub, params map[string]string, event apiGatewayEvent, fullPath string) {
	built := make([]Stub, 0, len(thenStubs))
	for _, t := range thenStubs {
		method := strings.ToUpper(strings.TrimSpace(interpolate(t.Method, params, event, fullPath)))
		path := normalizePath(interpolate(t.Path, params, event, fullPath))
		body := interpolate(t.Body, params, event, fullPath)

		var headers map[string]string
		if len(t.Headers) > 0 {
			headers = map[string]string{}
			for k, v := range t.Headers {
				headers[k] = interpolate(v, params, event, fullPath)
			}
		}
		if (headers == nil || headers["Content-Type"] == "") && looksLikeJSON(body) {
			if headers == nil {
				headers = map[string]string{}
			}
			if _, ok := headers["Content-Type"]; !ok {
				headers["Content-Type"] = "application/json"
			}
		}

		built = append(built, Stub{
			Method:  method,
			Path:    path,
			Status:  t.Status,
			Headers: headers,
			Body:    body,
			Match:   t.Match,
			Then:    t.Then,
		})
	}
	_ = registerStubs(stateDir, built)
}
