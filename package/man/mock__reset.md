#### Description

The `reset` command clears the mock server's stub store and/or its recorded request journal without stopping the server. This is the per-test workhorse: call it in a per-test hook so each test starts from a clean slate while the same server keeps running.

By default it clears both stubs and requests. Pass `--stubs true` to clear only the stubs, or `--requests true` to clear only the recorded requests.

#### Usage

```bash
aux4 mock reset [--stubs true] [--requests true] [--port <port>] [--name <handle>] [--stateDir <dir>]
```

--stubs     Clear only stubs when set to true
--requests  Clear only recorded requests when set to true
--port      Port of the mock server, derives stateDir (default: 7070)
--name      Address the server by handle instead of port (stateDir ${TMPDIR:-/tmp}/aux4-mock/name-<handle>)
--stateDir  Explicit state directory

#### Example

```bash
aux4 mock reset --port 8080
```

```text
reset ok
```
