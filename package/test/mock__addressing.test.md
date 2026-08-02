# aux4 mock addressing

Address a mock server by a stable `--name` handle and let the OS pick a free
port with `--port 0`, so a test never hand-picks a port or manages collisions.
Servers are started inside the first `execute` blocks (not `beforeAll`, which
the harness would kill), addressed purely by `--name` afterwards, and torn down
by name in `afterAll`. A legacy fixed `--port` server is exercised too to prove
backward compatibility.

```afterAll
aux4 mock stop --name addr-a 2>/dev/null
aux4 mock stop --name addr-b 2>/dev/null
aux4 mock stop --port 18781 2>/dev/null
aux4 mock stop --port 18782 2>/dev/null
pkill -f "aux4-mock/name-addr-" 2>/dev/null
true
```

## free-port + name addressing

### should serve a name-addressed server on an OS-assigned free port

```execute
URL=$(aux4 mock start --name addr-a --port 0 | head -1)
sleep 1
aux4 mock stub --name addr-a --method POST --path /messages --status 201 --body '{"server":"a"}' >/dev/null
curl -s -o /dev/null "$URL/api/messages" -X POST -H 'Content-Type: application/json' -d '{"text":"hi sally"}'
aux4 mock verify --name addr-a --method POST --path /messages --body-contains "hi sally"
```

```expect:partial
verify ok: 1 match(es)
```

### should start a second named server on a different free port without collision

```execute
URLB=$(aux4 mock start --name addr-b --port 0 | head -1)
sleep 1
PA=$(cat "${TMPDIR:-/tmp}/aux4-mock/name-addr-a/port")
PB=$(cat "${TMPDIR:-/tmp}/aux4-mock/name-addr-b/port")
aux4 mock stub --name addr-b --method GET --path /ping --status 200 --body '{"server":"b"}' >/dev/null
curl -s "$URLB/api/ping"
if [ "$PA" != "$PB" ]; then echo " [distinct]"; else echo " [COLLISION $PA $PB]"; fi
```

```expect
{"server":"b"} [distinct]
```

### should reset a server addressed only by name

```execute
aux4 mock reset --name addr-b --stubs true >/dev/null
curl -s -w "\n%{http_code}" "$(cat "${TMPDIR:-/tmp}/aux4-mock/name-addr-b/port" | sed 's#^#http://localhost:#')/api/ping"
```

```expect:partial
no matching stub**404
```

## backward compatibility: fixed --port

### should print the base URL and the bare chosen port on start

```execute
aux4 mock start --port 18782
```

```expect
http://localhost:18782
18782
```

### should stub and serve on a legacy fixed port unchanged

```execute
aux4 mock start --port 18781 >/dev/null
sleep 1
aux4 mock stub --port 18781 --method GET --path /legacy --status 200 --body '{"ok":true}' >/dev/null
curl -s "http://localhost:18781/api/legacy"
```

```expect
{"ok":true}
```
