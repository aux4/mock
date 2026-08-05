# aux4 mock preset / presets

The core mock engine is **preset-agnostic**: it ships no service knowledge. `mock preset`
is an empty routing point (`profile:mock:preset`) that installable
`community/mock-preset-<service>` packages extend by merging their own `<service>` commands
into it. `mock presets` lists whatever is installed. With nothing installed, the core knows
no services — it points the user at installing one.

## presets lists what is installed

`presets` reports whatever `community/mock-preset-*` packages are present, and points at
installing one when there are none. Which of the two appears depends on the machine, so assert
that it reports one of them rather than pinning the empty case — a developer with presets
installed should not see a failure here.

### should report the installed presets, or how to install one

```execute
aux4 mock presets | grep -qE "No presets installed\.|^Installed presets" && echo "REPORTED"
```

```expect:partial
REPORTED
```

## preset routing is empty in core

### should show the preset help describing installable presets

```execute
aux4 mock preset --help
```

```expect:partial
Apply a service preset to a running mock. Presets are installable packages
```

### should error cleanly on an uninstalled service

Named so it can never be installed — using a real service name fails on a machine that has
that preset.

```execute
aux4 mock preset not-a-real-service
```

```error:partial
Command not found: not-a-real-service
```
