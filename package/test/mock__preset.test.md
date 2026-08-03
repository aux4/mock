# aux4 mock preset / presets

The core mock engine is **preset-agnostic**: it ships no service knowledge. `mock preset`
is an empty routing point (`profile:mock:preset`) that installable
`community/mock-preset-<service>` packages extend by merging their own `<service>` commands
into it. `mock presets` lists whatever is installed. With nothing installed, the core knows
no services — it points the user at installing one.

## presets with nothing installed

### should report no presets and point at install

```execute
aux4 mock presets
```

```expect
No presets installed.
Install one, e.g.: aux4 aux4 pkger install community/mock-preset-google
Then apply it with: aux4 mock preset google --port <port>
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

```execute
aux4 mock preset google
```

```error:partial
Command not found: google
```
