package main

import (
	"os"
	"path/filepath"
	"testing"
)

// newThenTestDir creates a throwaway state directory for a single test.
func newThenTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "aux4-mock-then-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestApplyThen_InterpolatesAgainstTriggeringRequest(t *testing.T) {
	dir := newThenTestDir(t)

	thenStubs := []Stub{
		{
			Method: "GET",
			Path:   "/calendars/${path.calendarId}/events/${path.eventId}",
			Status: 404,
			Body:   `{"error":{"code":404,"message":"Not Found"}}`,
		},
	}

	params := map[string]string{"calendarId": "primary", "eventId": "evt-1"}
	event := apiGatewayEvent{HTTPMethod: "DELETE"}

	applyThen(dir, thenStubs, params, event, "/calendars/primary/events/evt-1")

	stubs := loadStubs(dir)
	if len(stubs) != 1 {
		t.Fatalf("expected 1 stub to be upserted, got %d: %+v", len(stubs), stubs)
	}
	got := stubs[0]
	if got.Method != "GET" {
		t.Errorf("Method = %q, want GET", got.Method)
	}
	if got.Path != "/calendars/primary/events/evt-1" {
		t.Errorf("Path = %q, want /calendars/primary/events/evt-1", got.Path)
	}
	if got.Status != 404 {
		t.Errorf("Status = %d, want 404", got.Status)
	}
	if got.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type header = %q, want application/json (auto-set for JSON body)", got.Headers["Content-Type"])
	}
}

func TestApplyThen_BodyPlaceholderResolvesNestedField(t *testing.T) {
	dir := newThenTestDir(t)

	thenStubs := []Stub{
		{
			Method: "GET",
			Path:   "/events/${path.eventId}",
			Body:   `{"id":"${path.eventId}","start":{"dateTime":"${body.start.dateTime}"}}`,
		},
	}

	params := map[string]string{"eventId": "evt-1"}
	event := apiGatewayEvent{
		HTTPMethod: "PATCH",
		Body:       `{"start":{"dateTime":"2026-10-24T10:00:00Z"}}`,
	}

	applyThen(dir, thenStubs, params, event, "/events/evt-1")

	stubs := loadStubs(dir)
	if len(stubs) != 1 {
		t.Fatalf("expected 1 stub, got %d", len(stubs))
	}
	want := `{"id":"evt-1","start":{"dateTime":"2026-10-24T10:00:00Z"}}`
	if stubs[0].Body != want {
		t.Errorf("Body = %q, want %q", stubs[0].Body, want)
	}
}

func TestApplyThen_EscapedPlaceholderSurvivesForTheStoredStubsOwnFutureResponses(t *testing.T) {
	dir := newThenTestDir(t)

	// $${body.text} should collapse to a literal ${body.text} in the stored
	// stub, to be resolved later when THAT stub itself serves a response.
	thenStubs := []Stub{
		{
			Method: "GET",
			Path:   "/echo/${path.id}",
			Body:   `{"id":"${path.id}","echoLater":"$${body.text}"}`,
		},
	}

	params := map[string]string{"id": "1"}
	event := apiGatewayEvent{HTTPMethod: "POST"}

	applyThen(dir, thenStubs, params, event, "/echo/1")

	stubs := loadStubs(dir)
	if len(stubs) != 1 {
		t.Fatalf("expected 1 stub, got %d", len(stubs))
	}
	want := `{"id":"1","echoLater":"${body.text}"}`
	if stubs[0].Body != want {
		t.Errorf("Body = %q, want %q (literal ${ preserved for later interpolation)", stubs[0].Body, want)
	}

	// Simulate that later request: the stored stub's body is interpolated
	// again when IT serves a response, and now resolves against ITS request.
	laterEvent := apiGatewayEvent{HTTPMethod: "GET", Body: `{"text":"hi sally"}`}
	resolved := interpolate(stubs[0].Body, map[string]string{"id": "1"}, laterEvent, "/echo/1")
	wantResolved := `{"id":"1","echoLater":"hi sally"}`
	if resolved != wantResolved {
		t.Errorf("resolved later body = %q, want %q", resolved, wantResolved)
	}
}

func TestApplyThen_ReplacesExistingStubAtSameMethodPathSlot(t *testing.T) {
	dir := newThenTestDir(t)

	// Seed an existing fallback stub at GET /events/evt-1 (the "original" state).
	original := Stub{Method: "GET", Path: "/events/evt-1", Status: 200, Body: `{"id":"evt-1","status":"confirmed"}`}
	if err := registerStubs(dir, []Stub{original}); err != nil {
		t.Fatalf("seed registerStubs: %v", err)
	}

	thenStubs := []Stub{
		{Method: "GET", Path: "/events/${path.eventId}", Status: 404, Body: `{"error":"not found"}`},
	}
	params := map[string]string{"eventId": "evt-1"}
	applyThen(dir, thenStubs, params, apiGatewayEvent{}, "/events/evt-1")

	stubs := loadStubs(dir)
	if len(stubs) != 1 {
		t.Fatalf("expected the then-stub to REPLACE the original (same method+path+predicate slot), got %d stubs: %+v", len(stubs), stubs)
	}
	if stubs[0].Status != 404 {
		t.Errorf("Status = %d, want 404 (replaced)", stubs[0].Status)
	}
}

func TestApplyThen_HeadersAreInterpolatedAndContentTypeAutoSetOnlyWhenAbsent(t *testing.T) {
	dir := newThenTestDir(t)

	thenStubs := []Stub{
		{
			Method:  "GET",
			Path:    "/widgets/${path.id}",
			Body:    `plain text, not json`,
			Headers: map[string]string{"X-Echo": "${path.id}"},
		},
	}
	params := map[string]string{"id": "w1"}
	applyThen(dir, thenStubs, params, apiGatewayEvent{}, "/widgets/w1")

	stubs := loadStubs(dir)
	if len(stubs) != 1 {
		t.Fatalf("expected 1 stub, got %d", len(stubs))
	}
	if stubs[0].Headers["X-Echo"] != "w1" {
		t.Errorf("X-Echo header = %q, want w1", stubs[0].Headers["X-Echo"])
	}
	if _, ok := stubs[0].Headers["Content-Type"]; ok {
		t.Errorf("Content-Type should not be auto-set for a non-JSON body, got %q", stubs[0].Headers["Content-Type"])
	}
}

func TestApplyThen_MultipleEntriesAllUpserted(t *testing.T) {
	dir := newThenTestDir(t)

	thenStubs := []Stub{
		{Method: "GET", Path: "/events/${path.eventId}", Status: 404, Body: `{"error":"not found"}`},
		{Method: "GET", Path: "/events", Status: 200, Body: `{"items":[]}`},
	}
	params := map[string]string{"eventId": "evt-1"}
	applyThen(dir, thenStubs, params, apiGatewayEvent{}, "/events/evt-1")

	stubs := loadStubs(dir)
	if len(stubs) != 2 {
		t.Fatalf("expected 2 stubs upserted, got %d: %+v", len(stubs), stubs)
	}
}

func TestStub_ThenJSONRoundTrip(t *testing.T) {
	// A stub carrying `then` must survive save/load through stubs.json (the
	// field is additive — stubs without `then` are untouched).
	dir := newThenTestDir(t)
	stub := Stub{
		Method: "DELETE",
		Path:   "/events/evt-1",
		Status: 204,
		Then: []Stub{
			{Method: "GET", Path: "/events/evt-1", Status: 404, Body: `{"error":"not found"}`},
		},
	}
	if err := saveStubs(dir, []Stub{stub}); err != nil {
		t.Fatalf("saveStubs: %v", err)
	}
	loaded := loadStubs(filepath.Dir(stubsPath(dir)))
	if len(loaded) != 1 || len(loaded[0].Then) != 1 {
		t.Fatalf("round-tripped stub lost its then entries: %+v", loaded)
	}
	if loaded[0].Then[0].Status != 404 {
		t.Errorf("then[0].Status = %d, want 404", loaded[0].Then[0].Status)
	}
}

func TestLoadStubs_WithoutThenFieldIsByteIdenticalBehavior(t *testing.T) {
	dir := newThenTestDir(t)
	stub := Stub{Method: "GET", Path: "/health", Status: 200, Body: `{"ok":true}`}
	if err := saveStubs(dir, []Stub{stub}); err != nil {
		t.Fatalf("saveStubs: %v", err)
	}
	loaded := loadStubs(dir)
	if len(loaded) != 1 || loaded[0].Then != nil {
		t.Fatalf("a stub without then must load with a nil Then, got %+v", loaded)
	}
}
