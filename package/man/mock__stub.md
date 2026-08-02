#### Description

The `stub` command defines or replaces a stub response in the running mock server's stub store. A stub is matched against incoming requests by method and exact path, plus any optional request-matching predicates it carries. Defining a stub for the same method+path+predicate set overwrites it, so tests can freely redefine behavior at runtime.

The response body may be provided three ways, in precedence order:

1. `--body` inline,
2. `--bodyFile <file>`, or
3. piped on stdin.

When the resolved body looks like JSON (starts/ends with `{}` or `[]`) and no `--contentType` is given, the response `Content-Type` defaults to `application/json`. Additional response headers can be set with repeatable `--header key=value` flags.

#### Request-matching predicates

A stub can be gated on properties of the incoming request. When present, **every** predicate must hold for the stub to match:

- **`--when-header key=value`** (repeatable) — the request must carry this header. The header name is compared case-insensitively. A trailing `*` on the value is a wildcard prefix match, e.g. `--when-header 'authorization=Bearer *'` matches any `Bearer …` token.
- **`--when-query key=value`** (repeatable) — the named query parameter must equal `value` (also supports the trailing `*` wildcard).
- **`--when-body-contains <substring>`** — the request body must contain the substring.

This lets several stubs coexist on the same method+path. Matching is **most-specific-first**: a stub with more predicates is evaluated before one with fewer, and a stub with **no** predicates is the lowest-priority fallback. The first stub whose method, path and predicates all hold wins; if none match, the server responds `404`. Priority is by predicate count, not definition order — a predicated stub beats a bare fallback even if the fallback was defined first.

The predicates are stored in the stub's `match` object with the shape `{ "headers": { … }, "query": { … }, "bodyContains": "…" }`. A stub with no predicates omits `match` entirely.

#### Usage

```bash
aux4 mock stub --path <path> [--method <method>] [--status <code>] \
  [--body <body> | --bodyFile <file>] [--header key=value ...] \
  [--contentType <type>] \
  [--when-header key=value ...] [--when-query key=value ...] \
  [--when-body-contains <substring>] \
  [--port <port>] [--stateDir <dir>]
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
--port                Port of the mock server, derives stateDir (default: 7070)
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
