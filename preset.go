package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

// jsonContentType is the header every preset stub carries; preset bodies are
// always JSON, so we set it explicitly rather than relying on the sniffing that
// `mock stub` does.
var jsonContentType = map[string]string{"Content-Type": "application/json"}

// presetOverrides carries the small set of values a test may want to pin so it
// can assert a known identity. Everything has a sane default.
type presetOverrides struct {
	AccessToken string
	Email       string
	User        string
}

func (o presetOverrides) username() string {
	// Derive a handle from the display name (lowercase, no spaces) unless the
	// user override already looks like a handle.
	return strings.ToLower(strings.ReplaceAll(o.User, " ", ""))
}

// jsonBody marshals a map to a compact JSON string for a stub body.
func jsonBody(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// bearerGated returns the match predicate used for every happy-path identity
// endpoint: it only fires when an Authorization: Bearer <token> header is
// present (any token). The paired bare fallback then serves the 401.
func bearerGated() *StubMatch {
	return &StubMatch{Headers: map[string]string{"authorization": "Bearer *"}}
}

// presetBuilder produces the stub set for one service, given the resolved
// overrides. Presets are pure data: each builder just returns []Stub.
type presetBuilder func(o presetOverrides) []Stub

// presetBuilders is the embedded registry of service presets.
var presetBuilders = map[string]presetBuilder{
	"google":    googlePreset,
	"microsoft": microsoftPreset,
	"x":         xPreset,
	"aws":       awsPreset,
	"oauth":     oauthPreset,
}

func presetNames() []string {
	names := make([]string, 0, len(presetBuilders))
	for k := range presetBuilders {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// jsonStub is a small constructor for a JSON-bodied stub.
func jsonStub(method, path string, status int, body map[string]any, match *StubMatch) Stub {
	return Stub{
		Method:  strings.ToUpper(method),
		Path:    normalizePath(path),
		Status:  status,
		Headers: jsonContentType,
		Body:    jsonBody(body),
		Match:   match,
	}
}

func googlePreset(o presetOverrides) []Stub {
	token := map[string]any{
		"access_token":  o.AccessToken,
		"expires_in":    3599,
		"token_type":    "Bearer",
		"scope":         "https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile",
		"refresh_token": "mock-refresh-token",
	}
	identity := map[string]any{
		"sub":            "1234567890",
		"email":          o.Email,
		"email_verified": true,
		"name":           o.User,
		"given_name":     o.User,
		"family_name":    "Example",
		"picture":        "https://example.com/photo.jpg",
	}
	unauth := map[string]any{
		"error": map[string]any{
			"code":    401,
			"message": "Request is missing required authentication credential. Expected OAuth 2 access token, login cookie or other valid authentication credential.",
			"status":  "UNAUTHENTICATED",
		},
	}
	stubs := []Stub{jsonStub("POST", "/token", 200, token, nil)}
	// Google exposes userinfo at both v2 and v3.
	for _, p := range []string{"/oauth2/v2/userinfo", "/oauth2/v3/userinfo"} {
		stubs = append(stubs,
			jsonStub("GET", p, 200, identity, bearerGated()),
			jsonStub("GET", p, 401, unauth, nil),
		)
	}
	return stubs
}

func microsoftPreset(o presetOverrides) []Stub {
	token := map[string]any{
		"token_type":     "Bearer",
		"expires_in":     3599,
		"ext_expires_in": 3599,
		"access_token":   o.AccessToken,
		"scope":          "https://graph.microsoft.com/.default",
		"refresh_token":  "mock-refresh-token",
	}
	identity := map[string]any{
		"id":                "00000000-0000-0000-0000-000000000000",
		"displayName":       o.User,
		"mail":              o.Email,
		"userPrincipalName": o.Email,
	}
	unauth := map[string]any{
		"error": map[string]any{
			"code":    "InvalidAuthenticationToken",
			"message": "Access token is empty.",
		},
	}
	return []Stub{
		// Concrete /common tenant path; document that tests point their base URL
		// so the tenant segment lands on /common (or override with mock stub).
		jsonStub("POST", "/common/oauth2/v2.0/token", 200, token, nil),
		jsonStub("GET", "/me", 200, identity, bearerGated()),
		jsonStub("GET", "/me", 401, unauth, nil),
	}
}

func xPreset(o presetOverrides) []Stub {
	token := map[string]any{
		"token_type":    "bearer",
		"expires_in":    7200,
		"access_token":  o.AccessToken,
		"scope":         "tweet.read users.read offline.access",
		"refresh_token": "mock-refresh-token",
	}
	identity := map[string]any{
		"data": map[string]any{
			"id":       "1234567890",
			"name":     o.User,
			"username": o.username(),
		},
	}
	unauth := map[string]any{
		"title":  "Unauthorized",
		"type":   "about:blank",
		"status": 401,
	}
	return []Stub{
		jsonStub("POST", "/2/oauth2/token", 200, token, nil),
		jsonStub("GET", "/2/users/me", 200, identity, bearerGated()),
		jsonStub("GET", "/2/users/me", 401, unauth, nil),
	}
}

func awsPreset(o presetOverrides) []Stub {
	// AWS does NOT use OAuth bearer auth (SigV4 + per-service XML/JSON). This is
	// a deliberately minimal preset: only STS GetCallerIdentity happy-path,
	// matched on the request body. Everything else should be stubbed explicitly
	// with `mock stub`.
	//
	// The predicate matches the action NAME (not "Action=GetCallerIdentity")
	// because aux4/api re-stringifies a form-urlencoded STS request body to
	// {"Action":"GetCallerIdentity",...}; matching the bare token survives both
	// the raw form body and the api-parsed form.
	identity := map[string]any{
		"GetCallerIdentityResponse": map[string]any{
			"GetCallerIdentityResult": map[string]any{
				"Arn":     "arn:aws:iam::123456789012:user/" + firstNonEmpty(o.username(), "sally"),
				"UserId":  "AIDAEXAMPLESALLY",
				"Account": "123456789012",
			},
			"ResponseMetadata": map[string]any{
				"RequestId": "01234567-89ab-cdef-0123-456789abcdef",
			},
		},
	}
	return []Stub{
		{
			Method:  "POST",
			Path:    normalizePath("/"),
			Status:  200,
			Headers: jsonContentType,
			Body:    jsonBody(identity),
			Match:   &StubMatch{BodyContains: "GetCallerIdentity"},
		},
	}
}

func oauthPreset(o presetOverrides) []Stub {
	token := map[string]any{
		"access_token":  o.AccessToken,
		"expires_in":    3600,
		"token_type":    "Bearer",
		"scope":         "openid profile email",
		"refresh_token": "mock-refresh-token",
	}
	identity := map[string]any{
		"sub":   "1234567890",
		"email": o.Email,
		"name":  o.User,
	}
	unauth := map[string]any{
		"error":             "invalid_token",
		"error_description": "The access token is missing or invalid",
	}
	return []Stub{
		jsonStub("POST", "/token", 200, token, nil),
		jsonStub("GET", "/userinfo", 200, identity, bearerGated()),
		jsonStub("GET", "/userinfo", 401, unauth, nil),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// runPreset registers a canned set of stubs for a well-known service (its auth
// dance + error envelopes) so a test can stand the service up with one command
// and then stub only the business endpoints it cares about. Presets never clear
// existing stubs.
func runPreset(args []string) {
	fs := flag.NewFlagSet("preset", flag.ExitOnError)
	port := fs.String("port", "7070", "server port (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	accessToken := fs.String("access-token", "mock-access-token", "token returned by the token endpoint")
	email := fs.String("email", "sally@example.com", "email in the userinfo/identity response")
	user := fs.String("user", "Sally", "display name in the userinfo/identity response")
	_ = fs.Parse(args)

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintf(os.Stderr, "Error: a service is required (one of: %s)\n", strings.Join(presetNames(), ", "))
		os.Exit(1)
	}
	service := strings.ToLower(rest[0])
	build, ok := presetBuilders[service]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown preset %q (one of: %s)\n", service, strings.Join(presetNames(), ", "))
		os.Exit(1)
	}

	dir := resolveStateDir(*stateDir, *port)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create state dir: %v\n", err)
		os.Exit(1)
	}

	stubs := build(presetOverrides{
		AccessToken: *accessToken,
		Email:       *email,
		User:        *user,
	})
	if err := registerStubs(dir, stubs); err != nil {
		fmt.Fprintf(os.Stderr, "Error registering preset: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("preset %s -> %d stub(s)\n", service, len(stubs))
	for _, s := range stubs {
		gated := ""
		if s.Match.predicateCount() > 0 {
			gated = " (gated)"
		}
		fmt.Printf("  %s %s -> %d%s\n", s.Method, s.Path, s.Status, gated)
	}
}
