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
- the path matches — either exactly (after normalizing to a single leading slash) or via `{name}`/`{name...}` parameter tokens (see [Path parameters](#path-parameters)), and
- every request-matching predicate on the stub (if any) holds.

Multiple stubs can match the same request. Matching is **most-specific-first**, ranked by:

1. more STATIC path segments (an exact `/2/tweets/123` beats `/2/tweets/{id}`),
2. no greedy `{name...}` wildcard (`/2/tweets/{id}` beats `/2/tweets/{rest...}`),
3. more request-matching predicates (a stub with predicates beats a bare fallback),
4. declaration order (stable) as the final tie-breaker.

The first stub whose method, path and predicates all hold wins — priority is by specificity, not definition order.

When nothing matches, the server responds `404` and still records the request (so you can verify calls that were meant to fail). The 404 body is a diagnostic: it keeps the `no matching stub` marker, echoes the `method`/`path` as received, lists every `registeredStubs` entry, and — when it can — adds a `hint`:

```json
{
  "error": "no matching stub",
  "method": "POST",
  "path": "/api/2/tweets",
  "registeredStubs": ["POST /2/tweets"],
  "hint": "did you mean POST /2/tweets? (you requested POST /api/2/tweets — the mock serves under /api, so the stub --path should omit the /api prefix)"
}
```

The hint specifically calls out the most common mistake — the mock serves under `/api`, so `--path` must omit it — and also flags a same-path method mismatch or the closest registered path. Every unmatched request is also appended to `server.log` in the state directory, so a test author can `tail` it.

### Path parameters

A stub `--path` may contain parameter tokens, mirroring `aux4/api`:

- **`{name}`** — matches exactly one path segment (no `/`). `--path /2/tweets/{id}` matches `/2/tweets/999` but not `/2/tweets/999/likes`.
- **`{name...}`** — greedy: matches the rest of the path including `/`. `--path /files/{rest...}` matches `/files/a/b/c.txt`.

A static path keeps matching exactly as before. Captured params are available to response templating as `${path.<name>}`.

### Templated responses

The response body and response header values are interpolated against the incoming request at respond time (always on). Placeholders use `${...}`:

- **`${path.<name>}`** — a named path param captured by a `{name}`/`{name...}` token.
- **`${query.<name>}`** — a query-string parameter.
- **`${header.<name>}`** — a request header (name matched case-insensitively).
- **`${body.<dotted.path>}`** — a field of the JSON request body; nested via dots, e.g. `${body.text}`, `${body.user.id}`.
- **`${body}`** — the whole raw request body. **`${path}`** — the whole request path.

An unknown or unresolved placeholder collapses to the empty string — the literal `${...}` is never left behind. To emit a literal `${`, write `$${`. The stub author owns the surrounding JSON quoting.

```bash
# echo a posted field back into the response (the x/create-tweet pattern)
aux4 mock stub --port 8080 --method POST --path /2/tweets --status 201 \
  --body '{"data":{"id":"1","text":"${body.text}"}}'
curl -s -X POST http://localhost:8080/api/2/tweets \
  -H 'Content-Type: application/json' -d '{"text":"hello sally"}'
# {"data":{"id":"1","text":"hello sally"}}

# capture the id from the path
aux4 mock stub --port 8080 --method GET --path /2/tweets/{id} \
  --body '{"data":{"id":"${path.id}"}}'
curl -s http://localhost:8080/api/2/tweets/999
# {"data":{"id":"999"}}
```

**Note:** an inline `--body '{"echo":"${body.text}"}'` reaches the store verbatim (single-quote it). The literal-escape `$${` is the exception — aux4's own `$$` handling mangles it on the command line — so when you need a literal `${...}` in the response, supply the body with `--bodyFile` instead of inline `--body`.

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

## Mocking real services (presets)

Most real APIs make you mock the same boilerplate before you can test anything interesting: an OAuth token endpoint, a userinfo/identity endpoint that returns `401` without a bearer token, and the service's specific error envelope. `mock preset <service>` registers all of that in one command, so your test only has to stub the business endpoints it actually cares about.

```bash
aux4 mock start --port 8080
aux4 mock preset google --port 8080
# preset google -> 5 stub(s)
#   POST /token -> 200
#   GET /oauth2/v2/userinfo -> 200 (gated)
#   GET /oauth2/v2/userinfo -> 401
#   GET /oauth2/v3/userinfo -> 200 (gated)
#   GET /oauth2/v3/userinfo -> 401
```

Each identity endpoint is two stubs on the same path: a happy path gated on `Authorization: Bearer *` and a bare fallback returning the real `401` envelope. So a bearer token gets the identity; its absence gets Google's error shape:

```bash
# with a bearer token → 200 identity
curl -s http://localhost:8080/api/oauth2/v2/userinfo -H 'Authorization: Bearer x'
# {"email":"sally@example.com","email_verified":true,...,"sub":"1234567890"}

# without one → 401 Google error envelope
curl -s http://localhost:8080/api/oauth2/v2/userinfo
# {"error":{"code":401,"message":"Request is missing required authentication credential...","status":"UNAUTHENTICATED"}}

# the token endpoint returns a standard OAuth token response
curl -s -X POST http://localhost:8080/api/token
# {"access_token":"mock-access-token","expires_in":3599,"refresh_token":"mock-refresh-token","scope":"...","token_type":"Bearer"}
```

Presets are **additive** — they never clear existing stubs, so you layer business endpoints on top:

```bash
aux4 mock preset google --port 8080
aux4 mock stub --port 8080 --method POST \
  --path /gmail/v1/users/me/messages/send \
  --status 200 --body '{"id":"msg_1","labelIds":["SENT"]}'
# both the preset auth stubs and the Gmail send stub now serve
```

### Available presets

| Service | Token endpoint | Identity endpoint (bearer-gated) | Unauthenticated envelope |
|---------|----------------|----------------------------------|--------------------------|
| `google` | `POST /token` | `GET /oauth2/v2/userinfo`, `GET /oauth2/v3/userinfo` | `{"error":{"code":401,"message":"…","status":"UNAUTHENTICATED"}}` |
| `microsoft` | `POST /common/oauth2/v2.0/token` | `GET /me` (Graph) | `{"error":{"code":"InvalidAuthenticationToken","message":"…"}}` |
| `x` | `POST /2/oauth2/token` | `GET /2/users/me` → `{"data":{…}}` | `{"title":"Unauthorized","type":"about:blank","status":401}` |
| `aws` | — | — | — |
| `oauth` | `POST /token` | `GET /userinfo` | `{"error":"invalid_token",…}` |

**`aws` is intentionally minimal.** AWS does not use OAuth bearer auth (it uses SigV4 request signing with per-service XML/JSON payloads), so the preset ships only the STS `GetCallerIdentity` happy path — `POST /` whose request body contains `GetCallerIdentity` returns an identity JSON. For anything else, stub the specific endpoints with `mock stub`.

**Base URLs and paths.** Presets stub each service's real path (`/token`, `/oauth2/v2/userinfo`, `/2/users/me`, …). Point the code-under-test's base URL at `http://localhost:<port>/api` and let it append the real path. For Microsoft, whose real token path has a `/{tenant}` segment, the preset stubs the concrete `/common/oauth2/v2.0/token`; point the client so the tenant lands on `common`, or add a tenant-specific token path with `mock stub`.

### Overrides

All overrides are optional with sane defaults. Only a couple of common ones are exposed so a test can assert a known identity:

```bash
aux4 mock preset oauth --port 8080 \
  --accessToken TOK123 --email sally@corp.io --user "Sally Ride"
```

- `--accessToken` — token returned by the token endpoint (default `mock-access-token`)
- `--email` — email in the userinfo/identity response (default `sally@example.com`)
- `--user` — display name in the identity response (default `Sally`; `x` derives the handle from it)

## Commands

| Command | Purpose |
|---------|---------|
| `mock start` | Start a detached mock server (launches `aux4/api` with a catch-all route) |
| `mock stop` | Stop the server and clean up its state directory |
| `mock stub` | Define or replace a stub response |
| `mock preset` | Register a canned service preset (auth token + userinfo + error envelopes) |
| `mock reset` | Clear stubs and/or the request journal (the per-test workhorse) |
| `mock requests` | Print recorded requests as a JSON array |
| `mock verify` | Assert a matching request was recorded (exit non-zero on failure) |

### mock start

```bash
aux4 mock start [--port <port|0|auto>] [--name <handle>] [--stateDir <dir>] [--timeout 15000]
```

Resolves the state directory, recreates it fresh (clearing old stubs and the journal), launches `aux4/api` in the background, writes a pidfile, and prints the base URL `http://localhost:<port>` on line 1 followed by the bare chosen port on line 2.

Pass `--port 0` (or `--port auto`) to bind a free OS-assigned port instead of hand-picking one; the chosen port is printed and persisted to `<stateDir>/port`. Give the server a stable `--name <handle>` and every other command can address it by name without knowing the port — see [Addressing servers](#addressing-servers). With no `--name` and no `--port`, the server binds the default `7070`.

### mock stop

```bash
aux4 mock stop [--port 7070] [--name <handle>] [--stateDir <dir>]
```

Stops the server via its pidfile (killing the whole process group), with a `pkill` fallback, then removes the state directory. When stopping a `--name`-addressed server it reads the persisted port so the fallback targets the right process.

### mock stub

```bash
aux4 mock stub --path <path> [--method ANY] [--status 200] \
  [--body <json> | --bodyFile <file> | <stdin>] \
  [--header key=value ...] [--contentType <type>] \
  [--when-header key=value ...] [--when-query key=value ...] \
  [--when-body-contains <substring>] [--port 7070]
```

Defines or replaces a stub, matched by (method, path, predicate set). The body may be given inline with `--body`, read from `--bodyFile`, or piped on stdin. When the body looks like JSON and no `--contentType` is given, `Content-Type: application/json` is set automatically. Add `--when-header`, `--when-query`, or `--when-body-contains` predicates to gate the stub on the incoming request — see [Request-matching predicates](#request-matching-predicates).

### mock preset

```bash
aux4 mock preset <google|microsoft|x|aws|oauth> [--port 7070] \
  [--accessToken <token>] [--email <email>] [--user <name>]
```

Registers a canned set of stubs for a well-known service — its OAuth token endpoint, its bearer-gated identity endpoint(s), and the matching `401` error envelope — so a test can stand up the auth dance with one command and stub only its business endpoints. Presets are additive (they never clear existing stubs), so they layer with `mock stub`. See [Mocking real services (presets)](#mocking-real-services-presets).

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
  [--body-contains <str> ...] [--header key=value ...] [--name <handle>] [--port 7070]
```

Asserts that a matching request was recorded. Exits `0` when the assertion holds; exits non-zero with a diagnostic on stderr (expected vs. actual, the missing body substrings, plus every recorded request) otherwise. Omitting `--times` asserts at least one match; `--times <n>` asserts exactly `n`.

`--body-contains` is **repeatable with AND semantics**: `--body-contains a --body-contains b` passes only when a single recorded request's body contains **both** substrings. On failure the diagnostic names exactly which substrings were missing (`missing body substring(s): [...]`). `--header` is likewise repeatable (AND).

## Addressing servers

Every command resolves which server it targets with a fixed precedence: `--stateDir` > `--name` > `--port`.

- **`--port <n>`** — the classic form. State lives at `${TMPDIR:-/tmp}/aux4-mock/<port>`. Fully backward compatible.
- **`--name <handle>`** — address a server by a stable handle instead of a port. State lives at `${TMPDIR:-/tmp}/aux4-mock/name-<handle>`. Combine with `--port 0` on `start` so the OS assigns a free port and no test ever hand-picks one:

  ```bash
  URL=$(aux4 mock start --name gateway --port 0 | head -1)   # line 1 = base URL, line 2 = bare port
  aux4 mock stub   --name gateway --method GET --path /health --status 200 --body '{"ok":true}'
  # ... point the code under test at "$URL" ...
  aux4 mock verify --name gateway --method GET --path /health
  aux4 mock stop   --name gateway
  ```

  Two named servers started with `--port 0` get distinct free ports and never collide. The chosen port is also readable from `<stateDir>/port` (e.g. `${TMPDIR:-/tmp}/aux4-mock/name-gateway/port`).
- **`--stateDir <dir>`** — pin the directory explicitly (highest precedence).

The state directory contains:

- `stubs.json` — the programmable stub store
- `requests.json` — the recorded request journal
- `server.pid` — the server process id
- `port` — the port the server actually bound (persisted so `--name` commands can recover it)
- `server.log` — the server's stdout/stderr

## Using in Tests

`aux4/mock` is a plain CLI, so it drops straight into an [`aux4/test`](https://hub.aux4.io/package/aux4/test) `.test.md` with `execute` blocks — **no test-framework plugin or extra dependency required**. Because the server is reprogrammable at runtime, the pattern is: start one server, then before each test reset it and define only the stubs that test needs.

Two conventions matter:

- **Start the server inside the first test's `execute` block, not `beforeAll`** — the test harness kills processes spawned in `beforeAll`, so the server would die before your tests run.
- **Tear it down in `afterAll`** (`mock stop` + a `pkill` fallback), so a failed test can't leak the server.

To avoid hand-picking a collision-free port per test file, start with a stable name and a free port, capture the base URL, and address every later command by name:

```bash
URL=$(aux4 mock start --name x-post --port 0 | head -1)
# point the code under test at "$URL", then stub/verify --name x-post; stop --name x-post in afterAll
```

A complete `.test.md` that mocks Google's userinfo endpoint and asserts the outgoing call:

````markdown
# gmail send against a mocked Google

```afterAll
aux4 mock stop --port 18080 2>/dev/null
pkill -f "18080" 2>/dev/null
true
```

## sends the message and hits Google with a bearer token

### start the mock and program the Google auth dance + the send endpoint

```execute
aux4 mock start --port 18080
sleep 1
aux4 mock preset google --port 18080
aux4 mock stub --port 18080 --method POST \
  --path /gmail/v1/users/me/messages/send --status 200 \
  --body '{"id":"18f...","labelIds":["SENT"]}'
```

```expect:partial
stub POST /gmail/v1/users/me/messages/send -> 200
```

### exercise the code under test, pointed at the mock

```execute
GOOGLE_API_URL=http://localhost:18080/api \
  aux4 gmail send --to sally@example.com --subject Hi --body "hello"
```

```expect:partial
"labelIds":["SENT"]
```

### assert the outgoing request carried the recipient

```execute
aux4 mock verify --port 18080 --method POST \
  --path /gmail/v1/users/me/messages/send \
  --times 1 --body-contains "sally@example.com"
```

```expect:partial
verify ok: 1 match(es)
```
````

> **Note:** `verify --header key=value` matches the header value **exactly** — pass a concrete token (e.g. `--header "authorization=Bearer test-token"` when the code sends a known one). The trailing-`*` wildcard is only for **stub** gating (`mock stub --when-header authorization="Bearer *"`), not for `verify`.

To redefine behavior per test, add a `reset` at the top of each test's `execute` block (`aux4 mock reset --port 18080`) and re-stub — the same running server serves the new behavior immediately.

## Mocking aux4 commands

HTTP stubs only reach packages that expose an API URL override; most do not. A **command
stub** stands in for an aux4 command itself, using aux4 core's `replace` hook, so any command
can be mocked — including one from another package — without that package supporting an
override of its own.

```
> aux4 mock command --stateDir ./state --command "google:gmail/list" --output '{"messages":[]}'
stubbed command google:gmail/list
AUX4_AUX4_FILES=/path/to/state/commands.aux4
```

Stubs are written as hooks into a generated `.aux4` in the state directory. aux4 picks them up
through `AUX4_AUX4_FILES`, so your own `.aux4` is never modified and the working directory does
not matter:

```
> export AUX4_AUX4_FILES=./state/commands.aux4
> aux4 google gmail list --query is:unread
{"messages":[]}
```

The command pattern is `profile/command`; nested profiles keep their `:` separator, so
`aux4 google gmail list` is `google:gmail/list`. Use `aux4 <command> --whereIsIt` to check a
command's profile. `*` is a per-segment wildcard.

### Narrowing a stub

`--param` stubs only matching arguments, leaving the real command to run for everything else:

```
> aux4 mock command --stateDir ./state --command "main/deploy" --param env=staging --output "stubbed"
```

### Verifying calls

Every stubbed call is recorded with the arguments as typed:

```
> aux4 mock invocations --stateDir ./state
[ { "command": "main/deploy", "commandLine": "aux4 deploy", "params": "--env 'staging'", ... } ]

> aux4 mock verify-command --stateDir ./state --command "main/deploy" --params-contains "staging"
verify ok: 1 match(es)
```

`verify-command` exits non-zero when the expectation fails, so it works directly as a test
assertion. `--times 0` asserts a command was **not** called.

### Lifetime

A stub keeps the real command from running, so `aux4 mock reset` clears command stubs along
with HTTP stubs. Keep patterns narrow — prefer an exact `profile/command` over a broad
wildcard — and reset between tests.

