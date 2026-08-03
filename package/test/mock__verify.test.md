# aux4 mock verify path matching

`mock verify --path` accepts the same `{name}` / `{name...}` tokens the stub
matcher uses (MOCK-012), so a test can assert "some request to `/2/tweets/{id}`"
without knowing the id. A concrete `--path` still matches exactly (back-compat).

The server is started inside the first test's `execute` block (not `beforeAll`,
which the harness would kill) and torn down in `afterAll` on a unique port.

```afterAll
aux4 mock stop --port 18775 2>/dev/null
pkill -f "18775" 2>/dev/null
true
```

## parametric --path matches a recorded concrete path

### should start the server, stub a parametric endpoint, and record two ids

```execute
aux4 mock start --port 18775
sleep 1
aux4 mock stub --port 18775 --method GET --path /2/tweets/{id} --body '{"id":"${path.id}"}'
curl -s -o /dev/null http://localhost:18775/api/2/tweets/999
curl -s -o /dev/null http://localhost:18775/api/2/tweets/111
echo recorded
```

```expect:partial
recorded
```

### should match a recorded /2/tweets/999 via --path /2/tweets/{id}

```execute
aux4 mock verify --port 18775 --method GET --path /2/tweets/{id} --times 2
```

```expect:partial
verify ok: 2 match(es)
```

### should still match a concrete --path exactly (back-compat)

```execute
aux4 mock verify --port 18775 --method GET --path /2/tweets/999 --times 1
```

```expect:partial
verify ok: 1 match(es)
```

### should not match a concrete path that was never recorded

```execute
aux4 mock verify --port 18775 --method GET --path /2/tweets/555
```

```error:partial
verify failed: expected at least 1 match(es), found 0
```

## greedy {rest...} token matches across slashes

### should record a multi-segment path

```execute
aux4 mock reset --port 18775
aux4 mock stub --port 18775 --method GET --path /files/{rest...} --body '{"ok":true}'
curl -s -o /dev/null http://localhost:18775/api/files/a/b/c.txt
echo recorded
```

```expect:partial
recorded
```

### should match the recorded path via a greedy verify pattern

```execute
aux4 mock verify --port 18775 --method GET --path '/files/{rest...}'
```

```expect:partial
verify ok: 1 match(es)
```

### should combine a parametric --path with --body-contains

```execute
aux4 mock reset --port 18775
aux4 mock stub --port 18775 --method POST --path /2/tweets/{id}/like --body '{"ok":true}'
curl -s -o /dev/null -X POST http://localhost:18775/api/2/tweets/42/like -H 'Content-Type: application/json' -d '{"user":"sally"}'
aux4 mock verify --port 18775 --method POST --path /2/tweets/{id}/like --body-contains sally
```

```expect:partial
verify ok: 1 match(es)
```
