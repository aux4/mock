#### Description

The `start` command launches a mock HTTP server and returns immediately. It resolves the state directory, recreates it fresh (clearing any old stubs and request journal), launches an `aux4/api` server in the background whose single catch-all route runs `aux4 mock respond`, writes a pidfile into the state directory, and prints the base URL followed by the bare chosen port.

The state directory is resolved deterministically so every other `mock` command targeting the same server agrees on where stubs and recorded requests live. Precedence is `--stateDir` > `--name` > `--port`:

- explicit `--stateDir <dir>`, or
- derived from a `--name <handle>` as `${TMPDIR:-/tmp}/aux4-mock/name-<handle>`, or
- derived from the port as `${TMPDIR:-/tmp}/aux4-mock/<port>`.

**Free port selection.** Pass `--port 0` (or `--port auto`) to bind an unused ephemeral port chosen by the OS — no more hand-picking collision-free ports across test files. When `--name` is set and `--port` is omitted, a free port is chosen automatically. With no `--name` and no `--port`, the server binds the historical default `7070`. The chosen port is persisted to `<stateDir>/port` and printed, so `mock stub` / `mock verify` / `mock stop` can address the server by `--name` alone.

**Output.** Line 1 is the full base URL (`http://localhost:<port>`), line 2 is the bare port. Point the code under test at the base URL and hit paths under `/api` (e.g. `http://localhost:<port>/api/messages`).

The server keeps running across many tests; use `mock stub` / `mock reset` to reprogram it at runtime rather than restarting it.

#### Usage

```bash
aux4 mock start [--port <port|0|auto>] [--name <handle>] [--stateDir <dir>] [--timeout <ms>]
```

--port      Port to run the mock server on. `0`/`auto` picks a free port; empty picks a free port when `--name` is set, otherwise `7070`
--name      Stable handle to address this server later without knowing its port
--stateDir  Explicit state directory (default: `${TMPDIR:-/tmp}/aux4-mock/name-<name>` or `/<port>`)
--timeout   Per-request handler timeout in milliseconds (default: 15000)

#### Example

Fixed port (backward compatible):

```bash
aux4 mock start --port 8080
```

```text
http://localhost:8080
8080
```

Free port addressed by name — capture the URL or the bare port in a `.test.md`:

```bash
URL=$(aux4 mock start --name gateway --port 0 | head -1)
# ... point the code under test at "$URL", then:
aux4 mock stub --name gateway --method GET --path /health --status 200 --body '{"ok":true}'
aux4 mock verify --name gateway --method GET --path /health
```

The bare port is the second line, so `PORT=$(aux4 mock start --name gateway --port 0 | tail -1)` captures it directly; it is also readable from `<stateDir>/port`.
