# aux4 mock request-matching predicates

Programs several stubs on the *same* method+path and proves that the
request-matching predicates (`--when-header`, `--when-query`,
`--when-body-contains`) select the right one, with a bare (no-predicate) stub
acting as the fallback. Predicated stubs always beat the fallback regardless of
the order they were defined in.

The server is started inside the first test's `execute` block (not `beforeAll`,
which the harness would kill) and torn down in `afterAll` on a unique port. Each
scenario resets the stub store first so the scenarios are independent.

```afterAll
aux4 mock stop --port 18772 2>/dev/null
pkill -f "18772" 2>/dev/null
true
```

## header predicate: auth present -> 200, absent -> 401

The bare `401` fallback is defined **before** the predicated `200`, proving the
predicated stub wins on priority, not definition order.

### should start the server and program a fallback + a header-gated stub

```execute
aux4 mock start --port 18772
sleep 1
aux4 mock stub --port 18772 --method POST --path /login --status 401 --body '{"error":"unauthorized"}'
aux4 mock stub --port 18772 --method POST --path /login --status 200 --body '{"token":"ok"}' --when-header 'authorization=Bearer *'
```

```expect:partial
stub POST /login -> 200
```

### should return 200 when the Authorization header is present

```execute
curl -s -w "\n%{http_code}" -X POST http://localhost:18772/api/login -H 'Authorization: Bearer sally-token'
```

```expect
{"token":"ok"}
200
```

### should fall through to 401 when the Authorization header is absent

```execute
curl -s -w "\n%{http_code}" -X POST http://localhost:18772/api/login
```

```expect
{"error":"unauthorized"}
401
```

## query predicate: selects between two stubs on the same path

### should program a fallback + a query-gated stub

```execute
aux4 mock reset --port 18772 --stubs true
aux4 mock stub --port 18772 --method GET --path /search --status 200 --body '{"tier":"free"}'
aux4 mock stub --port 18772 --method GET --path /search --status 200 --body '{"tier":"pro"}' --when-query 'plan=pro'
```

```expect:partial
stub GET /search -> 200
```

### should serve the pro stub when plan=pro is present

```execute
curl -s -w "\n%{http_code}" "http://localhost:18772/api/search?plan=pro"
```

```expect
{"tier":"pro"}
200
```

### should fall through to the free stub when the query is absent

```execute
curl -s -w "\n%{http_code}" "http://localhost:18772/api/search"
```

```expect
{"tier":"free"}
200
```

## body-contains predicate: selects on a request-body substring

### should program a fallback + a body-gated stub

```execute
aux4 mock reset --port 18772 --stubs true
aux4 mock stub --port 18772 --method POST --path /orders --status 200 --body '{"ok":true}'
aux4 mock stub --port 18772 --method POST --path /orders --status 422 --body '{"error":"bad sku"}' --when-body-contains 'INVALID'
```

```expect:partial
stub POST /orders -> 422
```

### should serve the 422 stub when the body contains the substring

```execute
curl -s -w "\n%{http_code}" -X POST http://localhost:18772/api/orders -H 'Content-Type: application/json' -d '{"sku":"INVALID"}'
```

```expect
{"error":"bad sku"}
422
```

### should fall through to the ok stub when the substring is absent

```execute
curl -s -w "\n%{http_code}" -X POST http://localhost:18772/api/orders -H 'Content-Type: application/json' -d '{"sku":"OK123"}'
```

```expect
{"ok":true}
200
```

## a predicated stub beats a bare fallback regardless of definition order

Defines the header-gated stub **first**, then the bare fallback, and confirms the
predicated stub still wins when its predicate holds.

### should program the predicated stub before the fallback

```execute
aux4 mock reset --port 18772 --stubs true
aux4 mock stub --port 18772 --method GET --path /account --status 200 --body '{"user":"sally"}' --when-header 'authorization=Bearer *'
aux4 mock stub --port 18772 --method GET --path /account --status 401 --body '{"error":"unauthorized"}'
```

```expect:partial
stub GET /account -> 401
```

### should serve the predicated stub when its predicate holds

```execute
curl -s -w "\n%{http_code}" http://localhost:18772/api/account -H 'Authorization: Bearer sally-token'
```

```expect
{"user":"sally"}
200
```
