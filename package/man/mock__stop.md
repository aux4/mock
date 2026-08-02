#### Description

The `stop` command terminates a running mock server and cleans up its state directory. It reads the server pidfile and kills the whole process group, with a `pkill` fallback keyed on the port and state directory in case the pidfile is missing. It then removes the state directory (stubs, request journal, pidfile, and log).

The server is addressed by `--stateDir` > `--name` > `--port`. When stopping a `--name`-addressed server (which may have bound an auto-selected free port), `stop` reads the persisted port from `<stateDir>/port` so the `pkill` fallback targets the right process without the caller tracking the chosen port.

#### Usage

```bash
aux4 mock stop [--port <port>] [--name <handle>] [--stateDir <dir>]
```

--port      Port of the mock server to stop (default: 7070)
--name      Stable handle of the server to stop (reads its persisted port)
--stateDir  Explicit state directory (default: ${TMPDIR:-/tmp}/aux4-mock/name-<name> or /<port>)

#### Example

```bash
aux4 mock stop --port 8080
```

```text
stopped mock server on port 8080
```

Stop a server started on a free port, addressed by name:

```bash
aux4 mock stop --name gateway
```

```text
stopped mock server on port 62485
```
