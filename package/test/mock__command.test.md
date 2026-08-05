# aux4 mock command

Stubbing an aux4 **command** rather than an HTTP endpoint.

HTTP stubbing only reaches packages that expose an API URL override; most do not. A command
stub uses aux4's `replace` hook, so any command can be stood in for — including one from
another package — and the call is recorded with the arguments it was given.

No server is needed: stubs live in a state directory and aux4 loads them through
`AUX4_AUX4_FILES`, so nothing is written to the user's own `.aux4` and the working directory
does not matter.

```file:target.aux4
{
  "profiles": [
    {
      "name": "main",
      "commands": [
        {
          "name": "deploy",
          "execute": [
            "echo REAL DEPLOY RAN"
          ],
          "help": {
            "text": "deploy the app",
            "variables": [
              { "name": "env", "text": "environment", "default": "" }
            ]
          }
        }
      ]
    }
  ]
}
```

## stubbing a command

### should return the stubbed output instead of running the command

```execute
aux4 mock command --stateDir ${TMPDIR:-/tmp}/aux4-mock-cmdtest --command "main/deploy" --output "stubbed deploy" > /dev/null
AUX4_AUX4_FILES="$(pwd)/target.aux4:${TMPDIR:-/tmp}/aux4-mock-cmdtest/commands.aux4" aux4 deploy --env prod
```

```expect
stubbed deploy
```

### should record the invocation with the arguments as typed

```execute
aux4 mock invocations --stateDir ${TMPDIR:-/tmp}/aux4-mock-cmdtest
```

```expect:partial
*"command": "main/deploy"*
```

```expect:partial
*--env 'prod'*
```

### should verify the command was called

```execute
aux4 mock verify-command --stateDir ${TMPDIR:-/tmp}/aux4-mock-cmdtest --command "main/deploy" --params-contains "prod"
```

```expect:partial
*verify ok*
```

### should fail verification for a command that was never called

```execute
aux4 mock verify-command --stateDir ${TMPDIR:-/tmp}/aux4-mock-cmdtest --command "main/release" 2>&1 || true
```

```expect:partial
*verify failed*
```

## only stubbing matching arguments

A stub can be narrowed with `--param`, so the real command still runs for other arguments.

### should stub only the matching parameter value

```execute
aux4 mock reset-commands --stateDir ${TMPDIR:-/tmp}/aux4-mock-cmdtest > /dev/null
aux4 mock command --stateDir ${TMPDIR:-/tmp}/aux4-mock-cmdtest --command "main/deploy" --param env=staging --output "stubbed staging" > /dev/null
AUX4_AUX4_FILES="$(pwd)/target.aux4:${TMPDIR:-/tmp}/aux4-mock-cmdtest/commands.aux4" aux4 deploy --env staging
```

```expect
stubbed staging
```

### should let the real command run when the parameter does not match

```execute
AUX4_AUX4_FILES="$(pwd)/target.aux4:${TMPDIR:-/tmp}/aux4-mock-cmdtest/commands.aux4" aux4 deploy --env prod
```

```expect
REAL DEPLOY RAN
```

## resetting

A leftover stub would keep a real command from running, so `reset-commands` retires them.
Command stubs need no server, so they are reset without one.

### should stop stubbing after reset

```execute
aux4 mock reset-commands --stateDir ${TMPDIR:-/tmp}/aux4-mock-cmdtest > /dev/null
AUX4_AUX4_FILES="$(pwd)/target.aux4:${TMPDIR:-/tmp}/aux4-mock-cmdtest/commands.aux4" aux4 deploy --env staging
```

```expect
REAL DEPLOY RAN
```
