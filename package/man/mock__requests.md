#### Description

The `requests` command prints the requests the mock server has recorded, as a JSON array. Each entry captures the method, normalized path, query parameters, headers, body, and a timestamp. Unmatched requests (those that produced a 404) are recorded too. The output can be filtered by method and/or path.

This is useful for debugging a failing test — inspect exactly what the code under test sent — and as a data source for custom assertions beyond what `mock verify` offers.

#### Usage

```bash
aux4 mock requests [--method <method>] [--path <path>] [--port <port>] [--name <handle>] [--stateDir <dir>]
```

--method    Filter recorded requests by method
--path      Filter recorded requests by path
--port      Port of the mock server, derives stateDir (default: 7070)
--name      Address the server by handle instead of port (stateDir ${TMPDIR:-/tmp}/aux4-mock/name-<handle>)
--stateDir  Explicit state directory

#### Example

```bash
aux4 mock requests --port 8080 --method POST --path /messages
```

```json
[
  {
    "method": "POST",
    "path": "/messages",
    "headers": {
      "content-type": "application/json"
    },
    "body": "{\"text\":\"hello sally\"}",
    "ts": "2026-08-02T09:33:08Z"
  }
]
```
