# Release notes

## Stateful effects: `--then`/`--thenFile` on `mock stub`

The mock was purely static: a stub always returned the same response, no matter what earlier
requests did. A verifier that checks outcomes by reading state back after acting (list → delete
→ list again) could not observe that an action actually took effect, because a GET after a
DELETE still returned the original resource.

`mock stub` now accepts `--then`/`--thenFile`: a JSON array of follow-up stubs that get built and
upserted into the store at the moment this stub serves a response. Each follow-up stub's
`method`/`path`/`headers`/`body` are interpolated against the request that triggered it, using
the same `${path.*}`/`${query.*}`/`${header.*}`/`${body}`/`${body.<dotted.path>}` placeholders a
response body already supports, then upserted with the same replace semantics as `mock stub`
(same method + path + predicate set replaces in place).

```bash
aux4 mock stub --port 8080 --method GET --path /events/evt-1 \
  --status 200 --body '{"id":"evt-1","status":"confirmed"}'

aux4 mock stub --port 8080 --method DELETE --path /events/{id} --status 204 \
  --thenFile delete-then.json
```

`delete-then.json`:

```json
[
  { "method": "GET", "path": "/events/${path.id}", "status": 404,
    "body": "{\"error\":{\"code\":404,\"message\":\"Not Found\"}}" }
]
```

```bash
curl -s -X DELETE http://localhost:8080/api/events/evt-1
curl -s http://localhost:8080/api/events/evt-1
# {"error":{"code":404,"message":"Not Found"}}
```

### Notes

- `--thenFile` is preferred over inline `--then` whenever the JSON contains `${path.*}`/
  `${body.*}` placeholders: those names collide with this command's own flags, so passing them
  inline risks aux4's own `${...}` resolution eating them before the mock sees them — the same
  reason `--bodyFile` exists for response bodies.
- A then-stub can escape a placeholder as `$${...}` to keep it literal in the stored stub, so it
  resolves against a LATER request once that stub itself serves a response.
- `match` predicates and any nested `then` on a then-stub are copied as-is, not interpolated.
- A stub without `then` behaves byte-identically to before — this is fully backward compatible.
