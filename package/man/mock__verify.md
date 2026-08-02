#### Description

The `verify` command asserts that the mock server recorded a request matching the given criteria. It is meant for use in tests: it exits `0` when the assertion holds and exits non-zero with a diagnostic on stderr — showing expected vs. actual counts, the missing body substrings, and every recorded request — when it does not.

Criteria are combined with AND:

- `--method` — match the HTTP method (case-insensitive)
- `--path` — match the exact normalized path
- `--body-contains` — require the request body to contain a substring. **Repeatable**: pass `--body-contains a --body-contains b` to require that a single recorded request's body contains **both** substrings (AND). When the assertion fails, the diagnostic lists exactly which substrings no candidate request body contained (`missing body substring(s): [...]`).
- `--header key=value` — require a request header (repeatable, AND; header name case-insensitive)

Count semantics: omitting `--times` asserts at least one match; `--times <n>` asserts exactly `n` matches.

The server is addressed by `--stateDir` > `--name` > `--port` (same precedence as every `mock` command), so a `--name`-addressed server needs no `--port`.

#### Usage

```bash
aux4 mock verify [--method <method>] [--path <path>] [--times <n>] \
  [--body-contains <str> ...] [--header key=value ...] \
  [--name <handle>] [--port <port>] [--stateDir <dir>]
```

--method         Match recorded requests by method
--path           Match recorded requests by path
--times          Exact number of matches expected (omit = at least 1)
--body-contains  Require the request body to contain this substring (repeatable; all must be present, AND)
--header         Require a request header as key=value (repeatable, AND)
--name           Stable handle of the server, derives stateDir (no --port needed)
--port           Port of the mock server, derives stateDir (default: 7070)
--stateDir       Explicit state directory

#### Example

Require two substrings in one recorded body:

```bash
aux4 mock verify --name gateway \
  --method POST \
  --path /orders \
  --body-contains "ABC" \
  --body-contains "gift"
```

```text
verify ok: 1 match(es)
```

When one substring is absent the command fails and names it:

```bash
aux4 mock verify --name gateway --method POST --path /orders \
  --body-contains "ABC" --body-contains "WIDGET"
```

```text
verify failed: expected at least 1 match(es), found 0
criteria: method="POST" path="/orders" body-contains=[ABC WIDGET] headers=map[]
missing body substring(s): [WIDGET]
recorded 1 request(s):
  POST /orders body={"sku":"ABC","note":"gift for sally"}
```
