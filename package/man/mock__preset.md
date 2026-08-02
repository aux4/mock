#### Description

The `preset` command registers a canned set of stubs for a well-known service in one shot: its OAuth token endpoint, its bearer-gated userinfo/identity endpoint(s), and the matching unauthenticated error envelope. This lets a test stand up a service's auth dance with a single command and then stub only the specific business endpoints it actually cares about with `mock stub`.

Presets are **additive** — they never clear existing stubs. You can register a preset and then layer business stubs on top (and vice versa); later `mock stub` calls for the service's real endpoints coexist with the preset. Re-running the same preset simply refreshes its own stubs (same replace-by-key semantics as `mock stub`).

Each identity endpoint is registered as **two** stubs on the same method+path: a happy-path stub gated on `Authorization: Bearer *` (any token) and a bare fallback that returns the service's `401` error envelope. So a request with a bearer token gets the identity; one without falls through to the realistic unauthenticated error.

#### Services

- **`google`** — `POST /token`, and `GET /oauth2/v2/userinfo` + `GET /oauth2/v3/userinfo` gated on a bearer token (unauth → `401` `{"error":{"code":401,"message":"…","status":"UNAUTHENTICATED"}}`). Registers 5 stubs.
- **`microsoft`** — `POST /common/oauth2/v2.0/token` (concrete `common` tenant path), and Graph `GET /me` gated on a bearer token (unauth → `401` `{"error":{"code":"InvalidAuthenticationToken","message":"…"}}`). Registers 3 stubs.
- **`x`** (Twitter) — `POST /2/oauth2/token`, and `GET /2/users/me` gated on a bearer token → `{"data":{"id","name","username"}}` (unauth → `401` `{"title","type","status":401}`). Registers 3 stubs.
- **`aws`** — **minimal, clearly-labeled.** AWS does not use OAuth bearer auth (it uses SigV4 + per-service XML/JSON), so this preset only ships the STS `GetCallerIdentity` happy path: `POST /` whose request body contains `GetCallerIdentity` → an identity JSON. Full AWS mocking is out of scope for presets — stub the specific endpoints you need with `mock stub`. Registers 1 stub.
- **`oauth`** (generic) — a provider-agnostic `POST /token` + bearer-gated `GET /userinfo` (unauth → `401` `{"error":"invalid_token",…}`), for services without a dedicated preset. Registers 3 stubs.

#### Paths and base URLs

The mock matches the request path exactly as aux4/api delivers it (no `/api` prefix, single leading slash). Presets stub each service's **real** path (`/token`, `/oauth2/v2/userinfo`, `/2/users/me`, …). Point the code-under-test's base URL at `http://localhost:<port>/api` and let it append the real path — the `/api` mount prefix is stripped before matching.

**Microsoft tenant segment:** the real token path is `/{tenant}/oauth2/v2.0/token`. The preset stubs the concrete `/common/oauth2/v2.0/token`. Point the code-under-test's authority/base URL so the tenant lands on `common` (the multi-tenant default), or register a tenant-specific token path yourself with `mock stub`.

#### Overrides

All overrides are optional with sane defaults; only a couple of common ones are exposed:

--accessToken   Token value returned by the token endpoint (default: `mock-access-token`)
--email         Email in the userinfo/identity response (default: `sally@example.com`)
--user          Display name in the userinfo/identity response (default: `Sally`; X derives the handle from it)

#### Usage

```bash
aux4 mock preset <service> [--port <port>] [--name <handle>] [--stateDir <dir>] \
  [--accessToken <token>] [--email <email>] [--user <name>]
```

<service>       One of: google, microsoft, x, aws, oauth (required)
--port          Port of the running mock server, derives stateDir (default: 7070)
--name          Address the server by handle instead of port (stateDir ${TMPDIR:-/tmp}/aux4-mock/name-<handle>)
--stateDir      Explicit state directory
--accessToken   Token returned by the token endpoint (default: mock-access-token)
--email         Email in the userinfo/identity response (default: sally@example.com)
--user          Display name in the userinfo/identity response (default: Sally)

#### Example

Stand up Google's auth dance, then layer only the Gmail send endpoint the test exercises:

```bash
aux4 mock start --port 8080
aux4 mock preset google --port 8080
aux4 mock stub --port 8080 --method POST \
  --path /gmail/v1/users/me/messages/send \
  --status 200 --body '{"id":"msg_1","labelIds":["SENT"]}'
```

```text
preset google -> 5 stub(s)
  POST /token -> 200
  GET /oauth2/v2/userinfo -> 200 (gated)
  GET /oauth2/v2/userinfo -> 401
  GET /oauth2/v3/userinfo -> 200 (gated)
  GET /oauth2/v3/userinfo -> 401
stub POST /gmail/v1/users/me/messages/send -> 200
```

A request with a bearer token gets the identity; without one it gets Google's `401` envelope:

```bash
curl -s http://localhost:8080/api/oauth2/v2/userinfo -H 'Authorization: Bearer x'
# {"email":"sally@example.com","email_verified":true,...,"sub":"1234567890"}

curl -s http://localhost:8080/api/oauth2/v2/userinfo
# {"error":{"code":401,"message":"Request is missing required authentication credential...","status":"UNAUTHENTICATED"}}
```
