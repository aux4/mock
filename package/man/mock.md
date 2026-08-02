#### Description

The `mock` command group is a programmable HTTP mock server for tests, built on top of `aux4/api`. It lets you stand in for real external APIs (Google, Microsoft, X, AWS, and others) without touching the network.

It follows the WireMock/nock model: one long-lived server whose stubs are reprogrammed at runtime. Each test rewrites the stubs and clears the request journal while the same server keeps running — no restart per test. `aux4/api` stays a pure HTTP-to-CLI bridge; the mock server mounts a single catch-all route that runs `aux4 mock respond`, which reads a JSON stub store, records the request, and prints the matching response.

Subcommands:

- **`start`** — start a detached mock server
- **`stop`** — stop the server and clean up its state
- **`stub`** — define or replace a stub response
- **`preset`** — register a canned service preset (auth token + userinfo + error envelopes)
- **`reset`** — clear stubs and/or the request journal
- **`requests`** — print recorded requests
- **`verify`** — assert a matching request was recorded

#### Usage

```bash
aux4 mock <command> [options]
```

#### Example

```bash
aux4 mock start --port 8080
aux4 mock stub --port 8080 --method POST --path /messages --status 201 --body '{"id":"msg_1"}'
curl -s -X POST http://localhost:8080/api/messages -d '{"text":"hello sally"}'
aux4 mock verify --port 8080 --method POST --path /messages --times 1
aux4 mock stop --port 8080
```
