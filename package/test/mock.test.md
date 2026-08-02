# aux4 mock

End-to-end test of the programmable mock loop: start a server, stub an
endpoint, drive it with plain `curl`, verify the recorded request, then reset
and confirm the stub is gone. The server is started inside the first test's
`execute` block (not `beforeAll`, which the test harness would kill) and torn
down in `afterAll` on a unique port.

```afterAll
aux4 mock stop --port 18771 2>/dev/null
pkill -f "18771" 2>/dev/null
true
```

## programmable mock loop

### should start the server and program a stub

```execute
aux4 mock start --port 18771
sleep 1
aux4 mock stub --port 18771 --method POST --path /messages --status 201 --body '{"id":"msg_1"}'
```

```expect:partial
stub POST /messages -> 201
```

### should return the stubbed body and status for a matching request

```execute
curl -s -w "\n%{http_code}" -X POST http://localhost:18771/api/messages -d '{"text":"hello sally"}'
```

```expect
{"id":"msg_1"}
201
```

### should record exactly one matching request carrying the sent payload

```execute
aux4 mock verify --port 18771 --method POST --path /messages --times 1 --body-contains "hello sally"
```

```expect:partial
verify ok: 1 match(es)
```

### should fail verification when the expected count is wrong

```execute
aux4 mock verify --port 18771 --method POST --path /messages --times 2
```

```error:partial
verify failed: expected exactly 2 match(es), found 1
```

### should return 404 after the stubs are reset

```execute
aux4 mock reset --port 18771
curl -s -w "\n%{http_code}" -X POST http://localhost:18771/api/messages -d '{"text":"again"}'
```

```expect:partial
no matching stub**404
```
