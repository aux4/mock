#### Description

The `stub` command defines or replaces a stub response in the running mock server's stub store. A stub is matched against incoming requests by method and exact path, plus any optional request-matching predicates it carries. Defining a stub for the same method+path+predicate set overwrites it, so tests can freely redefine behavior at runtime.

The response body may be provided three ways, in precedence order:

1. `--body` inline,
2. `--bodyFile <file>`, or
3. piped on stdin.

When the resolved body looks like JSON (starts/ends with `{}` or `[]`) and no `--contentType` is given, the response `Content-Type` defaults to `application/json`. Additional response headers can be set with repeatable `--header key=value` flags.

#### Path parameters

A stub `--path` may contain parameter tokens, mirroring `aux4/api`:

- **`{name}`** — matches exactly one path segment (no `/`). For example `--path /2/tweets/{id}` matches `/2/tweets/999` but not `/2/tweets/999/likes`.
- **`{name...}`** — greedy: matches the rest of the path including any `/`. For example `--path /files/{rest...}` matches `/files/a/b/c.txt`.

An exact (static) path keeps matching exactly as before. Captured params are exposed to response templating as `${path.<name>}` (see below).

When several stubs could match the same concrete request, the **most specific** one wins, ranked by:

1. more STATIC path segments (an exact `/2/tweets/123` beats `/2/tweets/{id}`),
2. no greedy `{name...}` wildcard (`/2/tweets/{id}` beats `/2/tweets/{rest...}`),
3. more request-matching predicates (see below),
4. declaration order (stable) as the final tie-breaker.

#### Templated responses

The response body and response header values are interpolated against the incoming request at respond time. Interpolation is always on. Placeholders use `${...}`:

- **`${path.<name>}`** — a named path param captured by a `{name}`/`{name...}` token.
- **`${query.<name>}`** — a query-string parameter.
- **`${header.<name>}`** — a request header (the name is matched case-insensitively).
- **`${body.<dotted.path>}`** — a field of the request body when it parses as JSON; nested fields use dots, e.g. `${body.text}` or `${body.user.id}`.
- **`${body}`** — the whole raw request body. **`${path}`** — the whole request path.

An unknown or unresolved placeholder collapses to the empty string — the literal `${...}` is never left behind. To emit a literal `${`, write `$${`. The stub author owns the surrounding JSON quoting, e.g. `--body '{"echo":"${body.text}"}'`.

**Note:** an inline `--body '{"echo":"${body.text}"}'` normally reaches the stub store verbatim (single-quote it so the shell does not expand it), and the mock resolves each `${...}` **once**, splicing the request value in without re-scanning it — so a nested placeholder never recurses inside the mock itself.

**Important — supply `${body.*}` templates with `--bodyFile`, not inline `--body`.** The value of the inline `--body` flag passes through aux4's own recursive `${...}` variable resolution before it reaches the mock, and aux4's variable namespace includes a variable literally named `body` (the flag itself). So an inline `--body '{"channel":{"id":"${body.channel}"}}'` is expanded by **aux4** — `${body.channel}` resolves against the self-referential `body` flag value and blows up into `{"id":"{"id":"{"id":"…`. The same applies to `${body}` and to the literal-escape `$${`. When a response references the `${body.*}` namespace (or needs a literal `${`), put the body in a file and pass `--bodyFile` (or pipe it on stdin): file/stdin content is read straight from disk by the mock and reaches the single-pass resolver untouched. Templates that only reference `${path.*}`, `${query.*}` or `${header.*}` are safe inline.

#### Request-matching predicates

A stub can be gated on properties of the incoming request. When present, **every** predicate must hold for the stub to match:

- **`--when-header key=value`** (repeatable) — the request must carry this header. The header name is compared case-insensitively. A trailing `*` on the value is a wildcard prefix match, e.g. `--when-header 'authorization=Bearer *'` matches any `Bearer …` token.
- **`--when-query key=value`** (repeatable) — the named query parameter must equal `value` (also supports the trailing `*` wildcard).
- **`--when-body-contains <substring>`** — the request body must contain the substring.

This lets several stubs coexist on the same method+path. Matching is **most-specific-first**: a stub with more predicates is evaluated before one with fewer, and a stub with **no** predicates is the lowest-priority fallback. The first stub whose method, path and predicates all hold wins; if none match, the server responds `404`. Priority is by predicate count, not definition order — a predicated stub beats a bare fallback even if the fallback was defined first.

The predicates are stored in the stub's `match` object with the shape `{ "headers": { … }, "query": { … }, "bodyContains": "…" }`. A stub with no predicates omits `match` entirely.

#### Stateful effects (`--then`/`--thenFile`)

By default the mock is static: a stub always returns the same response regardless of earlier requests. `--then` (inline JSON array) or `--thenFile` (JSON array read from a file) attaches one or more follow-up stubs that get upserted into the store at the moment THIS stub serves a response — so a later request can observe what the earlier one did (a DELETE that makes a later GET 404, a PATCH that changes what a later GET returns, a POST that makes a later GET-by-id succeed).

Each entry is a stub object (`method`/`path`/`status`/`headers`/`body`/`match`). Its `method`/`path`/`headers`/`body` are interpolated against the triggering request using the same `${path.*}`/`${query.*}`/`${header.*}`/`${body}`/`${body.<dotted.path>}` placeholders a response body supports. The built stub is then upserted with the same replace semantics as this command (same method + path + predicate set replaces in place) — so a then-stub naturally replaces whatever previously stood at that slot. A request field the then-stub's body doesn't reference, or one the request never sent, resolves to an empty string like any other unresolved placeholder.

**Prefer `--thenFile` over inline `--then`.** The placeholder names (`path`, `body`, `query`, `header`) are also this command's own flag names, so an inline `--then` JSON string containing `${path.*}`/`${body.*}` risks being expanded by aux4's own `${...}` resolution before the mock ever sees it — the same reason `--bodyFile` exists for response bodies. `--thenFile` content is read straight off disk.

A then-stub can carry `${...}` placeholders meant to survive for **its own** later responses (once it becomes an ordinary stored stub) by escaping them as `$${...}`: this first interpolation pass (against the triggering request) collapses `$${` to a literal `${`, and the stub serves that literal verbatim until a later request interpolates it again. The `match` predicates and any nested `then` on a then-stub are copied as-is, not interpolated — they describe how/whether that stub fires later, not data from the current request.

#### Usage

```bash
aux4 mock stub --path <path> [--method <method>] [--status <code>] \
  [--body <body> | --bodyFile <file>] [--header key=value ...] \
  [--contentType <type>] \
  [--when-header key=value ...] [--when-query key=value ...] \
  [--when-body-contains <substring>] \
  [--port <port>] [--name <handle>] [--stateDir <dir>]
```

--path                Request path to match (required)
--method              HTTP method to match; ANY matches any method (default: ANY)
--status              Response status code (default: 200)
--body                Inline response body (else --bodyFile, else stdin)
--bodyFile            File to read the response body from
--header              Response header as key=value (repeatable)
--contentType         Response Content-Type (JSON body defaults to application/json)
--when-header         Match only when a request header equals key=value; trailing * is a wildcard (repeatable)
--when-query          Match only when a request query param equals key=value (repeatable)
--when-body-contains  Match only when the request body contains this substring
--then                Inline JSON array of follow-up stubs to upsert when this stub serves a response
--thenFile            File to read the --then JSON array from (preferred when it contains ${path.*}/${body.*} placeholders)
--port                Port of the mock server, derives stateDir (default: 7070)
--name                Address the server by handle instead of port (stateDir ${TMPDIR:-/tmp}/aux4-mock/name-<handle>)
--stateDir            Explicit state directory

#### Example

```bash
aux4 mock stub --port 8080 \
  --method POST \
  --path /gmail/v1/users/me/messages/send \
  --status 200 \
  --header X-Trace=abc123 \
  --body '{"id":"18f...","labelIds":["SENT"]}'
```

```text
stub POST /gmail/v1/users/me/messages/send -> 200
```

#### Example: auth fallthrough on one path

Two stubs on the same `POST /login` — one gated on the `Authorization` header, one bare fallback. A request with a bearer token gets `200`; one without falls through to `401`. The predicated stub wins even though the fallback was defined first.

```bash
aux4 mock stub --port 8080 --method POST --path /login \
  --status 401 --body '{"error":"unauthorized"}'
aux4 mock stub --port 8080 --method POST --path /login \
  --status 200 --body '{"token":"ok"}' \
  --when-header 'authorization=Bearer *'
```

```text
stub POST /login -> 401
stub POST /login -> 200
```

#### Example: path param + echo a posted field

A read endpoint captures the id from the path and echoes it back; a create
endpoint echoes the posted `text` field into the response body.

```bash
aux4 mock stub --port 8080 --method GET --path /2/tweets/{id} \
  --body '{"data":{"id":"${path.id}","text":"tweet ${path.id}"}}'
aux4 mock stub --port 8080 --method POST --path /2/tweets --status 201 \
  --body '{"data":{"id":"1","text":"${body.text}"}}'
```

```bash
curl -s http://localhost:8080/api/2/tweets/999
# {"data":{"id":"999","text":"tweet 999"}}

curl -s -X POST http://localhost:8080/api/2/tweets \
  -H 'Content-Type: application/json' -d '{"text":"hello sally"}'
# {"data":{"id":"1","text":"hello sally"}}
```

#### Example: a DELETE that makes a later GET 404

```bash
aux4 mock stub --port 8080 --method GET --path /events/evt-1 \
  --status 200 --body '{"id":"evt-1","status":"confirmed"}'
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
aux4 mock stub --port 8080 --method DELETE --path /events/{id} --status 204 \
  --thenFile delete-then.json
```

```text
stub DELETE /events/{id} -> 204
```

```bash
curl -s -X DELETE http://localhost:8080/api/events/evt-1
curl -s -w "\n%{http_code}" http://localhost:8080/api/events/evt-1
# {"error":{"code":404,"message":"Not Found"}}
# 404
```
