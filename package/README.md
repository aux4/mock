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

An unknown or unresolved placeholder collapses to the empty string — the literal `${...}` is never left behind. The mock resolves each `${...}` **once** and splices the request value in verbatim, so a placeholder nested under a same-named key (e.g. `${body.channel}` inside a `channel` object) resolves cleanly instead of recursing. To emit a literal `${`, write `$${`. The stub author owns the surrounding JSON quoting.

**Supply `${body.*}` templates with `--bodyFile`, not inline `--body`.** The inline `--body` flag value passes through aux4's own recursive `${...}` variable resolution *before* the mock sees it, and aux4 has a variable literally named `body` (the flag). So an inline `${body.*}` placeholder can be mis-resolved against the flag value itself — for a nested case like `--body '{"channel":{"id":"${body.channel}"}}'` it blows up into `{"id":"{"id":"{"id":"…` before ever reaching the mock. Put such bodies in a file and pass `--bodyFile` (or pipe on stdin): file/stdin content is read straight from disk and reaches the mock's single-pass resolver untouched. Templates that only use `${path.*}`, `${query.*}` or `${header.*}` are safe inline.

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

**Note:** the literal-escape `$${` and any `${body.*}` template must be supplied with `--bodyFile` (or stdin), not inline `--body` — see the warning above. Templates using only `${path.*}`/`${query.*}`/`${header.*}` are safe inline (single-quote them so the shell does not expand them).

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

### Stateful effects (`--then`)

By default the mock is **static**: a stub always returns the same response, no matter what earlier requests did. `--then` lets a stub carry one or more follow-up stubs that get programmed into the store at the moment this stub serves a response — so a later request can observe the effect of an earlier one (a DELETE that makes a later GET 404, a PATCH that changes what a later GET returns, a POST that makes a later GET-by-id succeed).

```bash
aux4 mock stub --port 8080 --method GET --path /events/evt-1 \
  --status 200 --body '{"id":"evt-1","status":"confirmed"}'

aux4 mock stub --port 8080 --method DELETE --path /events/{id} --status 204 \
  --thenFile delete-then.json
```

`delete-then.json`:

```json
[
  {
    "method": "GET",
    "path": "/events/${path.id}",
    "status": 404,
    "body": "{\"error\":{\"code\":404,\"message\":\"Not Found\"}}"
  }
]
```

```bash
curl -s http://localhost:8080/api/events/evt-1
# {"id":"evt-1","status":"confirmed"}

curl -s -X DELETE http://localhost:8080/api/events/evt-1

curl -s -w "\n%{http_code}" http://localhost:8080/api/events/evt-1
# {"error":{"code":404,"message":"Not Found"}}
# 404
```

Each entry in `--then`/`--thenFile` is a stub object (`method`/`path`/`status`/`headers`/`body`/`match`). Its `method`/`path`/`headers`/`body` are interpolated against the **triggering request** using the same `${path.*}`/`${query.*}`/`${header.*}`/`${body}`/`${body.<dotted.path>}` placeholders a response body supports — so `${path.id}` above resolves to the id the DELETE actually hit. The built stub is then upserted into the store with the same replace semantics as `mock stub` (same method + path + predicate set replaces in place), which is exactly how the follow-up GET above comes to replace the original `200` stub.

A PATCH/PUT can merge part of the request into a response shaped like the updated resource:

```json
[
  {
    "method": "GET",
    "path": "/events/${path.id}",
    "status": 200,
    "body": "{\"id\":\"${path.id}\",\"start\":{\"dateTime\":\"${body.start.dateTime}\"}}"
  }
]
```

A field the request didn't send (e.g. `${body.summary}` when the PATCH only touched `start`) resolves to an empty string, same as any other unresolved placeholder — build the then-stub's body from exactly the fields you expect the request to carry.

**Use `--thenFile`, not inline `--then`, whenever the JSON contains placeholders.** The placeholder names (`path`, `body`, `query`, `header`) collide with this same command's own flags, so an inline `--then` value risks being expanded by aux4's own `${...}` variable resolution before the mock ever sees it — the identical reason `--bodyFile` exists for response bodies (see the warning above). `--thenFile` content is read straight off disk, untouched.

A then-stub can itself carry `${...}` placeholders meant to survive for **its own** later responses (once it becomes an ordinary stored stub) — escape them as `$${...}`. This first interpolation pass (against the triggering request) collapses `$${` to a literal `${`, and the stub serves that literal text verbatim until a later request interpolates it again:

```json
[
  { "method": "POST", "path": "/events/${path.id}/echo", "status": 200,
    "body": "{\"echoedLater\":\"$${body.text}\"}" }
]
```

The `match` predicates and any nested `then` on a then-stub are stored as-is, not interpolated — they describe how/whether that stub fires later, not data pulled from the current request.

## Mocking real services (presets)

Most real APIs make you mock the same boilerplate before you can test anything interesting: an OAuth token endpoint, a userinfo/identity endpoint that returns `401` without a bearer token, and the service's specific error envelope. A **service preset** registers all of that in one command, so your test only has to stub the business endpoints it actually cares about.

Presets are **installable packages**, not built into the core. The core mock engine ships no presets and reserves no service names — so you (or anyone) can ship, and even override, a preset for any service without the core claiming the name. Each preset is a package named `community/mock-preset-<service>` that adds a `<service>` command under `mock preset` through aux4's `global.aux4` profile merge. Install it and the command appears; uninstall it and it is gone. The core never changes.

```bash
aux4 aux4 pkger install community/mock-preset-google
aux4 mock start --port 8080
aux4 mock preset google --port 8080
# stub POST /token -> 200
# stub GET /oauth2/v2/userinfo -> 200
# stub GET /oauth2/v2/userinfo -> 401
# stub GET /oauth2/v3/userinfo -> 200
# stub GET /oauth2/v3/userinfo -> 401
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

Because a preset is just a sequence of `mock stub` calls, it is **additive** — it never clears existing stubs, so you layer business endpoints on top:

```bash
aux4 mock preset google --port 8080
aux4 mock stub --port 8080 --method POST \
  --path /gmail/v1/users/me/messages/send \
  --status 200 --body '{"id":"msg_1","labelIds":["SENT"]}'
# both the preset auth stubs and the Gmail send stub now serve
```

### Finding presets

List the presets available on your machine (i.e. the ones you have installed) with:

```bash
aux4 mock presets
```

```text
Installed presets (apply with: aux4 mock preset <service> --port <port>):
  google
```

With nothing installed it points you at installing one. Each preset accepts its own flags — see `aux4 mock preset <service> --help` after installing it. The reference preset `community/mock-preset-google` accepts `--accessToken`, `--email`, and `--user` so a test can assert a known identity:

```bash
aux4 mock preset google --port 8080 \
  --accessToken TOK123 --email sally@corp.io --user "Sally"
```

**Base URLs and paths.** A preset stubs each service's real path (`/token`, `/oauth2/v2/userinfo`, …). Point the code-under-test's base URL at `http://localhost:<port>/api` and let it append the real path — the `/api` mount prefix is stripped before matching.

### Writing your own preset package

A preset package is a **pure `.aux4` package** (no binary, no bundle) that depends on `aux4/mock` and contributes one `<service>` command under the `mock:preset` profile. When installed, aux4 merges that command into `mock preset` so `aux4 mock preset <service>` just works — the core is never touched.

The package `.aux4` needs three things:

1. `"dependencies": ["aux4/mock"]` so installing it pulls in the mock engine.
2. A re-declaration of the `mock` profile carrying the `preset` routing command (so `mock preset` resolves to the `mock:preset` profile the same way the core declares it). Merging is by name, so this is harmless.
3. A `mock:preset` profile with your `<service>` command, whose `execute` registers the canned stubs by calling `aux4 mock stub --port param(port) ...` once per endpoint.

A minimal `github` preset (one bearer-gated identity endpoint) looks like this:

```json
{
  "scope": "community",
  "name": "mock-preset-github",
  "version": "0.0.1",
  "description": "GitHub service preset for aux4/mock",
  "dependencies": [
    "aux4/mock"
  ],
  "profiles": [
    {
      "name": "main",
      "commands": [
        {
          "name": "mock",
          "execute": [
            "profile:mock"
          ],
          "help": {
            "text": "Programmable HTTP mock server for tests"
          }
        }
      ]
    },
    {
      "name": "mock",
      "commands": [
        {
          "name": "preset",
          "execute": [
            "profile:mock:preset"
          ],
          "help": {
            "text": "Apply a service preset to a running mock"
          }
        }
      ]
    },
    {
      "name": "mock:preset",
      "commands": [
        {
          "name": "github",
          "execute": [
            "aux4 mock stub param(port) --method GET --path /user --status 200 --when-header 'authorization=Bearer *' --body value(identity)",
            "aux4 mock stub param(port) --method GET --path /user --status 401 --body '{\"message\":\"Requires authentication\",\"status\":\"401\"}'"
          ],
          "help": {
            "text": "Register a GitHub auth preset (bearer-gated GET /user)",
            "variables": [
              {
                "name": "port",
                "text": "Port of the running mock server",
                "default": "7070"
              },
              {
                "name": "user",
                "text": "GitHub login returned by GET /user",
                "default": "sally"
              },
              {
                "name": "identity",
                "text": "Identity JSON (derived from --user)",
                "default": "{\"login\":\"${user}\",\"id\":1,\"type\":\"User\"}"
              }
            ]
          }
        }
      ]
    }
  ]
}
```

Key points, in the order they bite you:

- **Two stubs per gated endpoint.** Register the happy path with `--when-header 'authorization=Bearer *'` (the trailing `*` is a wildcard, so any token matches) and a bare fallback with no predicate that returns the service's real `401` envelope. The gated stub wins when a bearer token is present; the request falls through to the `401` otherwise.
- **`param(x)` already includes the flag — never prefix it.** `param(port)` expands to `--port '7070'`, so write `aux4 mock stub param(port) param(name) param(stateDir) --method ...`, **not** `--port param(port)`. Prefixing produces `--port --port '7070'`, which silently mis-resolves the state directory so the stubs are written where the server isn't (the calls still print success). Pass the *pass-through* flags (port/name/stateDir) with bare `param(...)` — empty ones drop out automatically — and the *literal* flags you're setting (`--method`, `--path`, `--status`) as plain flags with `value(...)` for their values.
- **Derive identity from flags with a defaulted variable.** Expose `--user`/`--email`/`--accessToken` as variables and interpolate them into a defaulted `identity`/`body` variable (`"default": "…${user}…"`) so overrides flow into the stub body. Pass it with `value(identity)`.
- **Pure aux4, no code.** The whole preset is `mock stub` calls — no Go, no JS, no `lib/`, no `dist/`.

See `community/mock-preset-google` for the fully worked reference (OAuth token endpoint + both userinfo versions + the Google `401` envelope, with `--accessToken`/`--email`/`--user` overrides).

## Commands

| Command | Purpose |
|---------|---------|
| `mock start` | Start a detached mock server (launches `aux4/api` with a catch-all route) |
| `mock stop` | Stop the server and clean up its state directory |
| `mock stub` | Define or replace a stub response |
| `mock preset` | Apply an installed service preset (routing point extended by `community/mock-preset-<service>` packages) |
| `mock presets` | List the service presets installed on this machine |
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
  [--when-body-contains <substring>] \
  [--then <json> | --thenFile <file>] [--port 7070]
```

Defines or replaces a stub, matched by (method, path, predicate set). The body may be given inline with `--body`, read from `--bodyFile`, or piped on stdin. When the body looks like JSON and no `--contentType` is given, `Content-Type: application/json` is set automatically. Add `--when-header`, `--when-query`, or `--when-body-contains` predicates to gate the stub on the incoming request — see [Request-matching predicates](#request-matching-predicates). Add `--then`/`--thenFile` to program follow-up stubs when this one serves a response — see [Stateful effects](#stateful-effects---then).

### mock preset

```bash
aux4 mock preset <service> [--port 7070] [preset-specific flags]
```

Routing point into the `mock:preset` profile, which the core ships **empty**. Installing a `community/mock-preset-<service>` package adds a `<service>` command here (via `global.aux4` profile merge), making `aux4 mock preset <service>` stand up that service's auth dance in one command. With nothing installed, `aux4 mock preset <service>` reports `Command not found: <service>`. Presets are additive (they only call `mock stub`), so they layer with your own stubs. See [Mocking real services (presets)](#mocking-real-services-presets) and [Writing your own preset package](#writing-your-own-preset-package).

### mock presets

```bash
aux4 mock presets
```

Lists the service presets available under `mock preset` — i.e. the `community/mock-preset-<service>` packages you have installed. With nothing installed it prints an install hint. Reflects exactly what is installed, with zero built-in service knowledge.

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

`--path` accepts the same `{name}`/`{name...}` parameter tokens as `mock stub`, so you can assert "some request to `/2/tweets/{id}`" without knowing the id (`{name}` matches one segment; `{name...}` matches the rest including `/`). A concrete `--path` still matches exactly.

`--body-contains` is **repeatable with AND semantics**: `--body-contains a --body-contains b` passes only when a single recorded request's body contains **both** substrings. On failure the diagnostic names exactly which substrings were missing (`missing body substring(s): [...]`). `--header` is likewise repeatable (AND).

**Quoting caveat:** a `--body-contains` value containing a single quote (`'`) cannot be passed reliably — aux4 single-quotes every argument to the mock binary without escaping an embedded `'`, so `--body-contains "\"raw\":'x'"` arrives as `"raw":x` and `--body-contains "it's"` errors with `unmatched '`. This is a core aux4 argument-quoting limitation, not a mock one. Assert on a distinctive **quote-free** substring instead.

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

A complete `.test.md` that mocks Google's userinfo endpoint and asserts the outgoing call. It uses the `community/mock-preset-google` preset package for the auth dance — declare it in the CI test job with `packages: aux4/mock,community/mock-preset-google` (or install it locally):

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

