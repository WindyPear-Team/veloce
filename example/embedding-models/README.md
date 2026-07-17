# Built-in embedding model package

Copy a model package to `DATA_PATH/embedding-models/<model-id>/` (by default,
`data/embedding-models/<model-id>/`) and restart the server. The engine uses
Wazero, a pure-Go WebAssembly runtime: it does not load CGO code, native shared
libraries, ONNX Runtime, or llama.cpp.

Required package files:

```text
<model-id>/
  manifest.json
  model.wasm
  tokenizer.json       # optional; available to the WASM module at /model/tokenizer.json
  model-weights.bin    # optional; available to the WASM module at /model/model-weights.bin
```

`manifest.json` example:

```json
{
  "id": "my-embedding-model",
  "name": "My embedding model",
  "description": "Local WASI embedding model",
  "dimensions": 384,
  "wasm": "model.wasm",
  "entrypoint": "embed",
  "timeout_seconds": 60
}
```

The WASM module must import WASI, export `embed` (or the configured entrypoint)
with no parameters, receive `{"input":["text"]}` on standard input, and write
one JSON object to standard output. The output may be either
`{"embeddings":[[...]]}` or OpenAI-style
`{"data":[{"index":0,"embedding":[...]}]}`. All vectors must have the exact
dimension declared in the manifest. Package assets are mounted read-only at
`/model`. The API and knowledge-base selector refer to this package as
`builtin:<model-id>`.

Authenticated users can invoke the local service at
`POST /api/user/advanced-chat/embeddings` with the OpenAI-compatible body
`{"model":"builtin:<model-id>","input":"text"}` (or an array of strings).
