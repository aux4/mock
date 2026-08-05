#### Description

The `preset` command is the extension point for **service presets** — canned stub sets that stand up a real service's auth dance (OAuth token endpoint, bearer-gated userinfo/identity, and the matching `401` error envelope) in one command, so a test only has to stub the business endpoints it cares about.

The core mock engine ships **no presets** and reserves **no service names**. `preset` is a pure routing command into the `mock:preset` profile, which is empty in core. Each preset is an installable package named `community/mock-preset-<service>` that declares a `<service>` command under `mock:preset`; installing it makes `aux4 mock preset <service>` available purely through aux4's `global.aux4` profile merge — the core is never modified and knows nothing about the service.

This means:

- **Nothing is bundled, nothing is reserved.** You can ship (or override) your own `google`, `stripe`, `github`, … preset without the core claiming the name.
- **Presets are ordinary aux4 commands.** A preset applies its stubs by calling `aux4 mock stub --port <port> ...` for each canned endpoint — no special core machinery.
- **List what is installed** with `aux4 mock presets`.

To see what a given preset does and which flags it accepts, install it and run `aux4 mock preset <service> --help`.

#### Usage

```bash
aux4 aux4 pkger install community/mock-preset-<service>
aux4 mock preset <service> [--port <port>] [preset-specific flags]
```

With nothing installed, `aux4 mock preset <service>` reports `Command not found: <service>` — install the matching `community/mock-preset-<service>` package first.

#### Writing your own preset package

A preset package is a pure-`.aux4` package that depends on `aux4/mock`, re-declares the `mock` profile with a `preset` routing command, and adds a `<service>` command under `mock:preset` whose `execute` registers stubs via `aux4 mock stub`. See the README section "Writing your own preset package" and the reference implementation `community/mock-preset-google`.

#### Example

```bash
aux4 aux4 pkger install community/mock-preset-google
aux4 mock start --port 8080
aux4 mock preset google --port 8080
```

```text
stub POST /token -> 200
stub GET /oauth2/v2/userinfo -> 200
stub GET /oauth2/v2/userinfo -> 401
stub GET /oauth2/v3/userinfo -> 200
stub GET /oauth2/v3/userinfo -> 401
```

A request with a bearer token gets the identity; without one it gets Google's `401` envelope:

```bash
curl -s http://localhost:8080/api/oauth2/v2/userinfo -H 'Authorization: Bearer x'
# {"email":"sally@example.com","email_verified":true,...,"sub":"1234567890"}

curl -s http://localhost:8080/api/oauth2/v2/userinfo
# {"error":{"code":401,"message":"Request is missing required authentication credential...","status":"UNAUTHENTICATED"}}
```
