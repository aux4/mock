#### Description

The `stop` command terminates a running mock server and cleans up its state directory. It reads the server pidfile and kills the whole process group, with a `pkill` fallback keyed on the port and state directory in case the pidfile is missing. It then removes the state directory (stubs, request journal, pidfile, and log).

#### Usage

```bash
aux4 mock stop [--port <port>] [--stateDir <dir>]
```

--port      Port of the mock server to stop (default: 7070)
--stateDir  Explicit state directory (default: ${TMPDIR:-/tmp}/aux4-mock/<port>)

#### Example

```bash
aux4 mock stop --port 8080
```

```text
stopped mock server on port 8080
```
