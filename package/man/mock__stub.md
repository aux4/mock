#### Description

The `stub` command defines or replaces a stub response in the running mock server's stub store. A stub is matched against incoming requests by method and exact path; the first matching stub wins. Defining a stub for a method+path that already exists overwrites it, so tests can freely redefine behavior at runtime.

The response body may be provided three ways, in precedence order:

1. `--body` inline,
2. `--bodyFile <file>`, or
3. piped on stdin.

When the resolved body looks like JSON (starts/ends with `{}` or `[]`) and no `--contentType` is given, the response `Content-Type` defaults to `application/json`. Additional response headers can be set with repeatable `--header key=value` flags.

The stored stub carries an optional reserved `match` field for future header/query/body predicates; it is not evaluated by the current matcher.

#### Usage

```bash
aux4 mock stub --path <path> [--method <method>] [--status <code>] \
  [--body <body> | --bodyFile <file>] [--header key=value ...] \
  [--contentType <type>] [--port <port>] [--stateDir <dir>]
```

--path         Request path to match (required)
--method       HTTP method to match; ANY matches any method (default: ANY)
--status       Response status code (default: 200)
--body         Inline response body (else --bodyFile, else stdin)
--bodyFile     File to read the response body from
--header       Response header as key=value (repeatable)
--contentType  Response Content-Type (JSON body defaults to application/json)
--port         Port of the mock server, derives stateDir (default: 7070)
--stateDir     Explicit state directory

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
