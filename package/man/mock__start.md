#### Description

The `start` command launches a mock HTTP server and returns immediately. It resolves the state directory, recreates it fresh (clearing any old stubs and request journal), launches an `aux4/api` server in the background whose single catch-all route runs `aux4 mock respond`, writes a pidfile into the state directory, and prints the base URL.

The state directory is resolved deterministically so every other `mock` command targeting the same port agrees on where stubs and recorded requests live:

- explicit `--stateDir <dir>`, or
- derived from the port as `${TMPDIR:-/tmp}/aux4-mock/<port>`.

The server keeps running across many tests; use `mock stub` / `mock reset` to reprogram it at runtime rather than restarting it.

#### Usage

```bash
aux4 mock start [--port <port>] [--stateDir <dir>] [--timeout <ms>]
```

--port      Port to run the mock server on (default: 7070)
--stateDir  Explicit state directory (default: ${TMPDIR:-/tmp}/aux4-mock/<port>)
--timeout   Per-request handler timeout in milliseconds (default: 15000)

#### Example

```bash
aux4 mock start --port 8080
```

```text
http://localhost:8080
```
