# aux4 mock path parameters and response templating

Proves the two request-driven features: parametric path matching
(`--path /2/tweets/{id}`, `{rest...}`) and response templating that
interpolates request data (`${path.*}`, `${query.*}`, `${header.*}`,
`${body.*}`) into the stubbed body.

The server is started inside the first test's `execute` block (not `beforeAll`,
which the harness would kill) and torn down in `afterAll` on a unique port. Each
scenario resets the stub store first so the scenarios are independent.

```afterAll
aux4 mock stop --port 18773 2>/dev/null
pkill -f "18773" 2>/dev/null
true
```

## path parameters: {id} matches one segment

### should start the server and program a parametric stub

```execute
aux4 mock start --port 18773
sleep 1
aux4 mock stub --port 18773 --method GET --path /2/tweets/{id} --body '{"data":{"id":"${path.id}","text":"tweet ${path.id}"}}'
```

```expect:partial
stub GET /2/tweets/{id} -> 200
```

### should match a concrete path and echo the captured param

```execute
curl -s -w "\n%{http_code}" http://localhost:18773/api/2/tweets/999
```

```expect
{"data":{"id":"999","text":"tweet 999"}}
200
```

### should not match a path with an extra segment

`{id}` matches a single segment only, so `/2/tweets/999/likes` falls through to
the 404.

```execute
curl -s -w "\n%{http_code}" http://localhost:18773/api/2/tweets/999/likes
```

```expect:partial
no matching stub**404
```

## greedy {rest...} matches the remainder of the path

### should program a greedy stub

```execute
aux4 mock reset --port 18773 --stubs true
aux4 mock stub --port 18773 --method GET --path /files/{rest...} --body '{"file":"${path.rest}"}'
```

```expect:partial
stub GET /files/{rest...} -> 200
```

### should capture the whole remaining path including slashes

```execute
curl -s -w "\n%{http_code}" http://localhost:18773/api/files/a/b/c.txt
```

```expect
{"file":"a/b/c.txt"}
200
```

## body echo: interpolate a posted JSON field back into the response

This is the exact behavior the community/x Python mock did that aux4/mock
previously could not.

### should program a stub that echoes the posted text

```execute
aux4 mock reset --port 18773 --stubs true
aux4 mock stub --port 18773 --method POST --path /2/tweets --status 201 --body '{"data":{"id":"1","text":"${body.text}"}}'
```

```expect:partial
stub POST /2/tweets -> 201
```

### should echo the posted text back in the response body

```execute
curl -s -w "\n%{http_code}" -X POST http://localhost:18773/api/2/tweets -H 'Content-Type: application/json' -d '{"text":"hello sally"}'
```

```expect
{"data":{"id":"1","text":"hello sally"}}
201
```

## nested and absent body fields

### should program a stub that reads a nested body field

```execute
aux4 mock reset --port 18773 --stubs true
aux4 mock stub --port 18773 --method POST --path /users --body '{"echo":"${body.user.name}"}'
```

```expect:partial
stub POST /users -> 200
```

### should resolve a nested body field with dot access

```execute
curl -s http://localhost:18773/api/users -H 'Content-Type: application/json' -d '{"user":{"name":"Sally"}}'
```

```expect
{"echo":"Sally"}
```

### should resolve an absent body field to an empty string

```execute
curl -s http://localhost:18773/api/users -H 'Content-Type: application/json' -d '{"user":{"id":7}}'
```

```expect
{"echo":""}
```

## query interpolation

### should program a stub that echoes a query param

```execute
aux4 mock reset --port 18773 --stubs true
aux4 mock stub --port 18773 --method GET --path /2/search --body '{"query":"${query.q}"}'
```

```expect:partial
stub GET /2/search -> 200
```

### should echo a query-string parameter into the body

```execute
curl -s "http://localhost:18773/api/2/search?q=sally"
```

```expect
{"query":"sally"}
```

## header interpolation

### should program a stub that echoes a request header

```execute
aux4 mock reset --port 18773 --stubs true
aux4 mock stub --port 18773 --method GET --path /2/me --body '{"auth":"${header.authorization}"}'
```

```expect:partial
stub GET /2/me -> 200
```

### should echo a request header into the body (case-insensitive name)

```execute
curl -s http://localhost:18773/api/2/me -H 'Authorization: Bearer sally-token'
```

```expect
{"auth":"Bearer sally-token"}
```

## specificity: an exact path beats a {id} stub regardless of order

The parametric `{id}` stub is defined **first**, then the exact
`/2/tweets/123`. A request for `/2/tweets/123` must be served by the exact stub;
`/2/tweets/999` still falls through to the parametric one.

### should program the {id} stub before the exact stub

```execute
aux4 mock reset --port 18773 --stubs true
aux4 mock stub --port 18773 --method GET --path /2/tweets/{id} --body '{"which":"param","id":"${path.id}"}'
aux4 mock stub --port 18773 --method GET --path /2/tweets/123 --body '{"which":"exact"}'
```

```expect:partial
stub GET /2/tweets/123 -> 200
```

### should serve the exact stub for the concrete path

```execute
curl -s http://localhost:18773/api/2/tweets/123
```

```expect
{"which":"exact"}
```

### should serve the parametric stub for any other id

```execute
curl -s http://localhost:18773/api/2/tweets/999
```

```expect
{"which":"param","id":"999"}
```

## escaping a literal ${ with $${

A body is loaded from a file so the literal `$${` reaches the store unmodified;
`$${body.text}` renders as the literal `${body.text}` while `${body.text}`
interpolates.

```file:esc-body.json
{"literal":"$${body.text}","echo":"${body.text}"}
```

### should program a stub whose body escapes one placeholder

```execute
aux4 mock reset --port 18773 --stubs true
aux4 mock stub --port 18773 --method POST --path /esc --contentType application/json --bodyFile esc-body.json
```

```expect:partial
stub POST /esc -> 200
```

### should render $${ as a literal ${ and still interpolate siblings

```execute
curl -s -X POST http://localhost:18773/api/esc -H 'Content-Type: application/json' -d '{"text":"hi sally"}'
```

```expect
{"literal":"${body.text}","echo":"hi sally"}
```
