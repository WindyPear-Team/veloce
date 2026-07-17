# Embedding model plugin package

Copy an embedding plugin package to `DATA_PATH/embedding-models/<plugin-id>/`
(by default, `data/embedding-models/<plugin-id>/`) and restart the server. The
server discovers it through the normal WASM plugin system. Wazero is a pure-Go
WebAssembly runtime: it does not load CGO code, native shared libraries, ONNX
Runtime, or llama.cpp.

Required package files:

```text
<model-id>/
  model.wasm
  tokenizer.json       # optional; available to the WASM module at /model/tokenizer.json
  model-weights.bin    # optional; available to the WASM module at /model/model-weights.bin
```

The module's `plugin_manifest` export must print a normal plugin manifest, for
example:

```json
{
  "id": "my-embedding-plugin",
  "name": "My embedding model",
  "version": "1.0.0",
  "description": "Local WASI embedding model",
  "embedding": {
    "dimensions": 384,
    "entrypoint": "embedding",
    "timeout_seconds": 60
  }
}
```

The WASM module must import WASI, export `embedding` (or the configured
entrypoint) with no parameters, receive `{"input":["text"]}` on standard
input, and write one JSON object to standard output. The output may be either
`{"embeddings":[[...]]}` or OpenAI-style
`{"data":[{"index":0,"embedding":[...]}]}`. All vectors must have the exact
dimension declared in the plugin manifest. Package assets are mounted read-only
at `/model`. The API and knowledge-base selector refer to this package as
`plugin:<plugin-id>`. Plugin enable/disable controls apply to embedding models
as well.

Authenticated users can invoke the local service at
`POST /api/user/advanced-chat/embeddings` with the OpenAI-compatible body
`{"model":"plugin:<plugin-id>","input":"text"}` (or an array of strings).
