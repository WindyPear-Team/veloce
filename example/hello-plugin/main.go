package main

import (
	"encoding/json"
	"io"
	"os"
)

func main() {}

//export plugin_manifest
func plugin_manifest() {
	_, _ = os.Stdout.Write([]byte(`{
  "id": "hello-plugin",
  "name": "Hello Plugin",
  "version": "0.1.0",
  "description": "A minimal Veloce WASM plugin example.",
  "author": "Veloce",
  "permissions": ["frontend.sidebar", "frontend.routes"],
  "hooks": [
    { "point": "app.boot", "mode": "async" }
  ],
  "frontend": {
    "sidebar": [
      { "label": "示例插件", "path": "hello" }
    ],
    "routes": [
      {
        "path": "hello",
        "title": "示例插件",
        "description": "这个页面由 WASM 插件的 plugin_manifest 声明。",
        "page": {
          "type": "card",
          "title": "Hello Plugin",
          "children": [
            { "type": "text", "text": "这是一个单文件 WASM 插件示例。插件信息、侧边栏和页面都来自 WASM 本身。" },
            {
              "type": "form",
              "fields": [
                { "type": "input", "name": "name", "label": "名称", "default": "Veloce" }
              ],
              "submit_label": "调用插件 Action",
              "action": "hello"
            }
          ]
        }
      }
    ]
  },
  "settings": {
    "type": "form",
    "fields": [
      { "type": "input", "name": "greeting", "label": "问候语", "default": "Hello" },
      { "type": "switch", "name": "enabled", "label": "启用问候", "default": true }
    ]
  }
}`))
}

//export plugin_init
func plugin_init() {}

//export plugin_handle_action
func plugin_handle_action() {
	raw, _ := io.ReadAll(os.Stdin)
	response := map[string]interface{}{
		"ok":      true,
		"message": "Hello from WASM plugin",
		"input":   string(raw),
	}
	_ = json.NewEncoder(os.Stdout).Encode(response)
}
