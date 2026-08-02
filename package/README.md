# aux4/mock

A programmable HTTP mock server for tests, built on top of `aux4/api`. Use it to stand in for real external APIs — Google, Microsoft, X, AWS, Stripe, and anything else your commands talk to — without touching the network.

`aux4/mock` follows the WireMock/nock model: one long-lived server whose stubs are reprogrammed at runtime. Each test rewrites the stubs and clears the request journal while the **same server keeps running** — no restart per test.

`aux4/api` stays a pure HTTP-to-CLI bridge and never returns canned bodies. Instead, the mock server mounts a single catch-all route that runs `aux4 mock respond`, which reads a programmable stub store (a JSON state file), records the incoming request, and prints the matching response.

## Installation

```bash
aux4 aux4 pkger install aux4/mock
```

## Quick Start

```bash
# 1. Start a mock server on port 8080
aux4 mock start --port 8080
# http://localhost:8080

# 2. Program a stub: POST /messages returns 201 with a JSON body
aux4 mock stub --port 8080 --method POST --path /messages --status 201 --body '{"id":"msg_1"}'

# 3. Point your code at the mock and make a request
curl -s -X POST http://localhost:8080/api/messages -d '{"text":"hello sally"}'
# {"id":"msg_1"}

# 4. Assert the mock received the expected request
aux4 mock verify --port 8080 --method POST --path /messages --times 1 --body-contains "hello sally"
# verify ok: 1 match(es)

# 5. Tear the server down
aux4 mock stop --port 8080
```

## Mocking a Real API (Gmail send)

Suppose a command sends mail through the Gmail REST API and you want to assert the payload it builds — without hitting Google. Point the client's base URL at the mock, stub the Gmail send endpoint, exercise your command, then verify the request it made.

```bash
aux4 mock start --port 8080

# Stub the Gmail "send message" endpoint
aux4 mock stub --port 8080 \
  --method POST \
  --path /gmail/v1/users/me/messages/send \
  --status 200 \
  --body '{"id":"18f...","labelIds":["SENT"]}'

# Run whatever command hits http://localhost:8080/api/gmail/v1/users/me/messages/send
curl -s -X POST http://localhost:8080/api/gmail/v1/users/me/messages/send \
  -H 'Authorization: Bearer test-token' \
  -d '{"raw":"VG86IHNhbGx5QGF1eDQuaW8..."}'
# {"id":"18f...","labelIds":["SENT"]}

# Assert the outgoing payload and auth header
aux4 mock verify --port 8080 \
  --method POST \
  --path /gmail/v1/users/me/messages/send \
  --body-contains '"raw"' \
  --header Authorization="Bearer test-token"

aux4 mock stop --port 8080
```

**Note:** requests arrive at the mock under the `/api` prefix (`http://localhost:<port>/api/<your/path>`). The `/api` prefix is stripped before matching, so stub and verify paths are written without it (`/gmail/v1/users/me/messages/send`).

## How Requests Match

A stub matches a request when:

- the method matches (`ANY` matches every method),
- the path matches exactly (after normalizing to a single leading slash), and
- every request-matching predicate on the stub (if any) holds.

Multiple stubs can share the same method+path, distinguished by their predicates. Matching is **most-specific-first**: a stub with more predicates is evaluated before one with fewer, and a stub with no predicates is the lowest-priority fallback. The first stub whose method, path and predicates all hold wins — priority is by predicate count, not definition order. When nothing matches, the server responds `404` with `{"error":"no matching stub","method":...,"path":...}` and still records the request, so you can verify calls that were meant to fail.

### Request-matching predicates

Gate a stub on properties of the incoming request with these optional `mock stub` flags. When present, **all** predicates must hold:

- `--when-header key=value` (repeatable) — the request must carry this header. The header name is compared case-insensitively, and a trailing `*` on the value is a wildcard prefix (e.g. `authorization=Bearer *`).
- `--when-query key=value` (repeatable) — the named query parameter must equal `value` (also supports the trailing `*` wildcard).
- `--when-body-contains <substring>` — the request body must contain the substring.

The predicates are stored in the stub's `match` object: `{ "headers": { … }, "query": { … }, "bodyContains": "…" }`. Defining a stub replaces an existing one only when the method, path **and** predicate set are identical; stubs that differ by predicate coexist.

### Fallthrough example: auth present → 200, absent → 401

Program two stubs on the same `POST /login` — one gated on the `Authorization` header, one bare fallback:

```bash
# bare fallback (defined first — order does not matter)
aux4 mock stub --port 8080 --method POST --path /login \
  --status 401 --body '{"error":"unauthorized"}'

# header-gated stub: any bearer token
aux4 mock stub --port 8080 --method POST --path /login \
  --status 200 --body '{"token":"ok"}' \
  --when-header 'authorization=Bearer *'

# with the header → 200 (predicated stub wins)
curl -s -X POST http://localhost:8080/api/login -H 'Authorization: Bearer sally-token'
# {"token":"ok"}

# without it → 401 (falls through to the bare stub)
curl -s -X POST http://localhost:8080/api/login
# {"error":"unauthorized"}
```

`--when-query` and `--when-body-contains` select between stubs on the same path the same way.

## Commands

| Command | Purpose |
|---------|---------|
| `mock start` | Start a detached mock server (launches `aux4/api` with a catch-all route) |
| `mock stop` | Stop the server and clean up its state directory |
| `mock stub` | Define or replace a stub response |
| `mock reset` | Clear stubs and/or the request journal (the per-test workhorse) |
| `mock requests` | Print recorded requests as a JSON array |
| `mock verify` | Assert a matching request was recorded (exit non-zero on failure) |

### mock start

```bash
aux4 mock start [--port 7070] [--stateDir <dir>] [--timeout 15000]
```

Resolves the state directory, recreates it fresh (clearing old stubs and the journal), launches `aux4/api` in the background, writes a pidfile, and prints the base URL `http://localhost:<port>`.

### mock stop

```bash
aux4 mock stop [--port 7070] [--stateDir <dir>]
```

Stops the server via its pidfile (killing the whole process group), with a `pkill` fallback, then removes the state directory.

### mock stub

```bash
aux4 mock stub --path <path> [--method ANY] [--status 200] \
  [--body <json> | --bodyFile <file> | <stdin>] \
  [--header key=value ...] [--contentType <type>] \
  [--when-header key=value ...] [--when-query key=value ...] \
  [--when-body-contains <substring>] [--port 7070]
```

Defines or replaces a stub, matched by (method, path, predicate set). The body may be given inline with `--body`, read from `--bodyFile`, or piped on stdin. When the body looks like JSON and no `--contentType` is given, `Content-Type: application/json` is set automatically. Add `--when-header`, `--when-query`, or `--when-body-contains` predicates to gate the stub on the incoming request — see [Request-matching predicates](#request-matching-predicates).

### mock reset

```bash
aux4 mock reset [--stubs true] [--requests true] [--port 7070]
```

Clears both the stub store and the request journal by default. Pass `--stubs true` or `--requests true` to clear only one. Call this in a per-test hook so each test starts clean while the server stays up.

### mock requests

```bash
aux4 mock requests [--method <m>] [--path <p>] [--port 7070]
```

Prints the recorded requests as a JSON array, optionally filtered by method and/or path.

### mock verify

```bash
aux4 mock verify [--method <m>] [--path <p>] [--times <n>] \
  [--body-contains <str>] [--header key=value ...] [--port 7070]
```

Asserts that a matching request was recorded. Exits `0` when the assertion holds; exits non-zero with a diagnostic on stderr (expected vs. actual, plus every recorded request) otherwise. Omitting `--times` asserts at least one match; `--times <n>` asserts exactly `n`.

## State Directory

Every command resolves to the same state directory for a given port, so the control plane and the running server agree on where stubs and requests live:

- explicit `--stateDir <dir>`, or
- derived from the port as `${TMPDIR:-/tmp}/aux4-mock/<port>`.

The directory contains:

- `stubs.json` — the programmable stub store
- `requests.json` — the recorded request journal
- `server.pid` — the server process id
- `server.log` — the server's stdout/stderr

## Using in Tests

Because the server is reprogrammable at runtime, a test suite starts one server, then in a per-test hook resets it and defines just the stubs that test needs:

```bash
# once, before the suite (inside the first test's execute block — the test
# harness kills processes spawned in beforeAll)
aux4 mock start --port 18080

# before each test
aux4 mock reset --port 18080
aux4 mock stub --port 18080 --method POST --path /messages --status 201 --body '{"id":"msg_1"}'

# the test exercises the code, then asserts
aux4 mock verify --port 18080 --method POST --path /messages --times 1

# after the suite
aux4 mock stop --port 18080
```
