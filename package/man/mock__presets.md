#### Description

The `presets` command lists the service presets currently available under `mock preset`. Presets are contributed by installed `community/mock-preset-<service>` packages and merged into the `mock:preset` profile via `global.aux4` — the core ships none, so with nothing installed this prints an install hint instead of a list.

It reflects exactly what is installed by inspecting the merged `mock preset` profile, with zero built-in service knowledge: installing a preset package makes it appear here automatically; uninstalling removes it.

#### Usage

```bash
aux4 mock presets
```

#### Example

With nothing installed:

```bash
aux4 mock presets
```

```text
No presets installed.
Install one, e.g.: aux4 aux4 pkger install community/mock-preset-google
Then apply it with: aux4 mock preset google --port <port>
```

After installing one or more preset packages:

```bash
aux4 aux4 pkger install community/mock-preset-google
aux4 mock presets
```

```text
Installed presets (apply with: aux4 mock preset <service> --port <port>):
  google
```
