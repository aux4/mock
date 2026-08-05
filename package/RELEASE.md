# Release notes

## Mock aux4 commands, not just HTTP

HTTP stubbing only reaches packages that expose an API URL override. Most do not, so a test
could not stand in for an arbitrary command.

`aux4 mock command` registers a stand-in for a command itself, using aux4 core's `replace`
hook. No server and no port are involved — command stubs are not HTTP.

```bash
aux4 mock command --stateDir ./state \
  --command "google:gmail/list" --output '{"messages":[]}'

export AUX4_AUX4_FILES=./state/commands.aux4
aux4 google gmail list --query is:unread     # returns the stub; the real command never runs

aux4 mock verify-command --stateDir ./state \
  --command "google:gmail/list" --params-contains "is:unread"
```

Calls are recorded with the arguments as they were typed, so a test can assert **what was
passed**, not merely that something ran. `--param key=value` narrows a stub to matching
arguments, leaving the real command to run for everything else, and `--times 0` asserts a
command was *not* called.

New commands: `command`, `invocations`, `verify-command`, `reset-commands`.

### Notes

- The command pattern is `profile/command`, with nested profiles keeping their `:` separator —
  `aux4 google gmail list` is `google:gmail/list`. Check any command with
  `aux4 <command> --whereIsIt`.
- Stub **leaf** commands. A pattern like `google:calendar/*` also matches the `events` command
  that *routes* into `google:calendar:events`, which would replace the routing step and stop
  the descent into `create`.
- A stub is not a safety boundary. A command with no matching stub still runs for real — use
  permissions to bound what may execute.
- Requires an aux4 core with the `replace` hook phase.
