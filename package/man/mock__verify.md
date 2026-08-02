#### Description

The `verify` command asserts that the mock server recorded a request matching the given criteria. It is meant for use in tests: it exits `0` when the assertion holds and exits non-zero with a diagnostic on stderr — showing expected vs. actual counts and listing every recorded request — when it does not.

Criteria are combined with AND:

- `--method` — match the HTTP method (case-insensitive)
- `--path` — match the exact normalized path
- `--body-contains` — require the request body to contain a substring
- `--header key=value` — require a request header (repeatable, header name case-insensitive)

Count semantics: omitting `--times` asserts at least one match; `--times <n>` asserts exactly `n` matches.

#### Usage

```bash
aux4 mock verify [--method <method>] [--path <path>] [--times <n>] \
  [--body-contains <str>] [--header key=value ...] \
  [--port <port>] [--stateDir <dir>]
```

--method         Match recorded requests by method
--path           Match recorded requests by path
--times          Exact number of matches expected (omit = at least 1)
--body-contains  Require the request body to contain this substring
--header         Require a request header as key=value (repeatable)
--port           Port of the mock server, derives stateDir (default: 7070)
--stateDir       Explicit state directory

#### Example

```bash
aux4 mock verify --port 8080 \
  --method POST \
  --path /messages \
  --times 1 \
  --body-contains "hello sally"
```

```text
verify ok: 1 match(es)
```
