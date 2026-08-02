# aux4 mock service presets

Proves that `aux4 mock preset <service>` stands up a real service's auth dance
(token endpoint + bearer-gated userinfo/identity + unauth error envelopes) with a
single command, and that a preset layers cleanly under a later business `mock stub`.

The server is started inside the first test's `execute` block (not `beforeAll`,
which the harness would kill) and torn down in `afterAll` on a unique port. It is
a detached process, so it survives across the sibling `##` scenarios; each
scenario clears the stub store first so the scenarios stay independent. All
checks use plain `curl` with no external network.

```afterAll
aux4 mock stop --port 18775 2>/dev/null
pkill -f "18775" 2>/dev/null
true
```

## google preset: token endpoint + bearer-gated userinfo

### should start the server and register the google preset

```execute
aux4 mock start --port 18775
sleep 1
aux4 mock preset google --port 18775
```

```expect:partial
preset google -> 5 stub(s)
```

### should return an OAuth token from the token endpoint

```execute
curl -s -X POST http://localhost:18775/api/token
```

```expect
{"access_token":"mock-access-token","expires_in":3599,"refresh_token":"mock-refresh-token","scope":"https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile","token_type":"Bearer"}
```

### should return the identity when a bearer token is present

```execute
curl -s -w "\n%{http_code}" http://localhost:18775/api/oauth2/v2/userinfo -H 'Authorization: Bearer sally-token'
```

```expect
{"email":"sally@example.com","email_verified":true,"family_name":"Example","given_name":"Sally","name":"Sally","picture":"https://example.com/photo.jpg","sub":"1234567890"}
200
```

### should return a 401 Google error envelope without a bearer token

```execute
curl -s -w "\n%{http_code}" http://localhost:18775/api/oauth2/v2/userinfo
```

```expect
{"error":{"code":401,"message":"Request is missing required authentication credential. Expected OAuth 2 access token, login cookie or other valid authentication credential.","status":"UNAUTHENTICATED"}}
401
```

### should gate the v3 userinfo endpoint the same way

```execute
curl -s -w "\n%{http_code}" http://localhost:18775/api/oauth2/v3/userinfo
```

```expect
{"error":{"code":401,"message":"Request is missing required authentication credential. Expected OAuth 2 access token, login cookie or other valid authentication credential.","status":"UNAUTHENTICATED"}}
401
```

## x preset: token endpoint + bearer-gated users/me

### should register the x preset

```execute
aux4 mock reset --port 18775 --stubs true
aux4 mock preset x --port 18775
```

```expect:partial
preset x -> 3 stub(s)
```

### should return the user when a bearer token is present

```execute
curl -s -w "\n%{http_code}" http://localhost:18775/api/2/users/me -H 'Authorization: Bearer sally-token'
```

```expect
{"data":{"id":"1234567890","name":"Sally","username":"sally"}}
200
```

### should return a 401 X error body without a bearer token

```execute
curl -s -w "\n%{http_code}" http://localhost:18775/api/2/users/me
```

```expect
{"status":401,"title":"Unauthorized","type":"about:blank"}
401
```

## overrides: pin a known access token and identity

### should register the oauth preset with overrides

```execute
aux4 mock reset --port 18775 --stubs true
aux4 mock preset oauth --port 18775 --accessToken TOK123 --email sally@corp.io --user "Sally Ride"
```

```expect:partial
preset oauth -> 3 stub(s)
```

### should return the overridden token

```execute
curl -s -X POST http://localhost:18775/api/token
```

```expect
{"access_token":"TOK123","expires_in":3600,"refresh_token":"mock-refresh-token","scope":"openid profile email","token_type":"Bearer"}
```

### should return the overridden identity with a bearer token

```execute
curl -s http://localhost:18775/api/userinfo -H 'Authorization: Bearer sally-token'
```

```expect
{"email":"sally@corp.io","name":"Sally Ride","sub":"1234567890"}
```

## layering: a preset coexists with a later business stub

### should register the google preset then a gmail business stub

```execute
aux4 mock reset --port 18775 --stubs true
aux4 mock preset google --port 18775
aux4 mock stub --port 18775 --method POST --path /gmail/v1/users/me/messages/send --status 200 --body '{"id":"msg_1","labelIds":["SENT"]}'
```

```expect:partial
stub POST /gmail/v1/users/me/messages/send -> 200
```

### should still serve the preset token endpoint

```execute
curl -s -X POST http://localhost:18775/api/token
```

```expect
{"access_token":"mock-access-token","expires_in":3599,"refresh_token":"mock-refresh-token","scope":"https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile","token_type":"Bearer"}
```

### should serve the layered gmail business endpoint

```execute
curl -s -w "\n%{http_code}" -X POST http://localhost:18775/api/gmail/v1/users/me/messages/send -H 'Authorization: Bearer sally-token'
```

```expect
{"id":"msg_1","labelIds":["SENT"]}
200
```
