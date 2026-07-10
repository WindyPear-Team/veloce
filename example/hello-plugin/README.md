# Hello Plugin

This is a minimal single-file Veloce WASM plugin example.

The plugin exports:

- `plugin_manifest`: writes plugin metadata, frontend sidebar entries, routes, and settings schema as JSON to stdout.
- `plugin_init`: called after installation/startup load.
- `plugin_handle_action`: receives JSON from stdin and writes JSON to stdout.

## Build

Install TinyGo, then run from the `community` repository root:

```bash
tinygo build -target=wasi -o data/plugins/hello-plugin.wasm ./example/hello-plugin
```

Restart Veloce or upload `data/plugins/hello-plugin.wasm` from the Plugins page.

On startup, Veloce scans:

```text
data/plugins/*.wasm
```

Each WASM file must provide `plugin_manifest`; no external `plugin.json` is needed.
