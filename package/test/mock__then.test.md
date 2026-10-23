# aux4 mock stateful effects (--then)

Proves that a stub's `--then`/`--thenFile` follow-up stubs are interpolated
against the request that triggered them and upserted into the store, so a
later request observes the effect of an earlier one (e.g. a DELETE makes a
later GET 404; a PATCH changes what a later GET returns).

The server is started inside the first test's `execute` block (not
`beforeAll`, which the harness would kill) and torn down in `afterAll` on a
unique port. Each scenario resets the stub store first so the scenarios are
independent.

```afterAll
aux4 mock stop --port 18774 2>/dev/null
pkill -f "18774" 2>/dev/null
true
```

## delete-then-404: a DELETE makes a later GET of the same resource 404

`${path.id}` in the then-stub's path must resolve to the id captured from
THIS DELETE request, not stay a literal placeholder.

```file:delete-then.json
[
  {
    "method": "GET",
    "path": "/events/${path.id}",
    "status": 404,
    "body": "{\"error\":\"not found\"}"
  }
]
```

### should start the server and program the stub with --thenFile

```execute
aux4 mock start --port 18774
sleep 1
aux4 mock stub --port 18774 --method GET --path /events/evt-1 --status 200 --body '{"id":"evt-1","status":"confirmed"}'
aux4 mock stub --port 18774 --method DELETE --path /events/{id} --status 200 --body '{"deleted":true}' --thenFile delete-then.json
```

```expect:partial
stub DELETE /events/{id} -> 200
```

### should return the original event before the delete

```execute
curl -s -w "\n%{http_code}" http://localhost:18774/api/events/evt-1
```

```expect
{"id":"evt-1","status":"confirmed"}
200
```

### should delete the event

```execute
curl -s -w "\n%{http_code}" -X DELETE http://localhost:18774/api/events/evt-1
```

```expect
{"deleted":true}
200
```

### should now 404 on the same event

```execute
curl -s -w "\n%{http_code}" http://localhost:18774/api/events/evt-1
```

```expect
{"error":"not found"}
404
```

## patch-then-updated: a PATCH changes what a later GET returns

The then-stub's body merges the request's `start.dateTime` into an object
shaped like the resource, using `${body.start.dateTime}` (nested dotted
access).

```file:patch-then.json
[
  {
    "method": "GET",
    "path": "/events/${path.id}",
    "status": 200,
    "body": "{\"id\":\"${path.id}\",\"start\":{\"dateTime\":\"${body.start.dateTime}\"}}"
  }
]
```

### should program the original event and a PATCH that updates it

```execute
aux4 mock reset --port 18774 --stubs true
aux4 mock stub --port 18774 --method GET --path /events/evt-2 --status 200 --body '{"id":"evt-2","start":{"dateTime":"2026-10-23T09:00:00Z"}}'
aux4 mock stub --port 18774 --method PATCH --path /events/{id} --status 200 --body '{"id":"${path.id}","updated":true}' --thenFile patch-then.json
```

```expect:partial
stub PATCH /events/{id} -> 200
```

### should return the original start time before the patch

```execute
curl -s http://localhost:18774/api/events/evt-2
```

```expect
{"id":"evt-2","start":{"dateTime":"2026-10-23T09:00:00Z"}}
```

### should patch the event's start time

```execute
curl -s -X PATCH http://localhost:18774/api/events/evt-2 -H 'Content-Type: application/json' -d '{"start":{"dateTime":"2026-10-24T10:00:00Z"}}'
```

```expect
{"id":"evt-2","updated":true}
```

### should now return the updated start time

```execute
curl -s http://localhost:18774/api/events/evt-2
```

```expect
{"id":"evt-2","start":{"dateTime":"2026-10-24T10:00:00Z"}}
```

## a stub without `then` behaves exactly as before

Regression: adding `then` support must not change the response of a stub
that never uses it.

### should program a plain stub and serve it unchanged

```execute
aux4 mock reset --port 18774 --stubs true
aux4 mock stub --port 18774 --method GET --path /health --status 200 --body '{"ok":true}'
curl -s -w "\n%{http_code}" http://localhost:18774/api/health
```

```expect:partial
stub GET /health -> 200**{"ok":true}
200
```

## escaping: $${...} in a then-stub body survives for its own later response

The then-stub's body echoes a posted field back once IT is hit later. Writing
`$${body.text}` keeps a literal `${body.text}` in the stored stub (this first
interpolation pass only resolves it against the triggering DELETE, which has
no body); the literal is then resolved normally the next time that stub
itself serves a response.

```file:escape-then.json
[
  {
    "method": "POST",
    "path": "/events/${path.id}/echo",
    "status": 200,
    "body": "{\"echoedLater\":\"$${body.text}\"}"
  }
]
```

### should program a stub whose then-stub escapes one placeholder

```execute
aux4 mock reset --port 18774 --stubs true
aux4 mock stub --port 18774 --method DELETE --path /events/{id} --status 200 --body '{"deleted":true}' --thenFile escape-then.json
curl -s -w "\n%{http_code}" -X DELETE http://localhost:18774/api/events/evt-3
```

```expect
reset ok
stub DELETE /events/{id} -> 200
{"deleted":true}
200
```

### should resolve the escaped placeholder against the later request, not the delete

```execute
curl -s -X POST http://localhost:18774/api/events/evt-3/echo -H 'Content-Type: application/json' -d '{"text":"hi sally"}'
```

```expect
{"echoedLater":"hi sally"}
```
