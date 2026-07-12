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
    { "point": "app.boot", "mode": "async" },
    { "point": "advanced_chat.runtime_extension", "mode": "sync", "priority": 10 },
    { "point": "advanced_chat.tool_call", "mode": "sync", "action": "hello_plugin_echo", "priority": 10 },
    { "point": "plugin.settings.updated", "mode": "async" },
    { "point": "plugin.action.before", "mode": "sync", "action": "*" },
    { "point": "plugin.action.after", "mode": "async", "action": "*" }
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
    "tabs": [
      { "id": "general", "label": "通用", "description": "基础行为设置。" },
      { "id": "advanced", "label": "高级", "description": "运行时扩展设置。" }
    ],
    "fields": [
      { "type": "input", "name": "greeting", "label": "问候语", "default": "Hello", "tab": "general" },
      { "type": "switch", "name": "enabled", "label": "启用问候", "default": true, "tab": "general" },
      { "type": "textarea", "name": "system_hint", "label": "系统提示补充", "tab": "advanced" }
    ]
  }
}`))
}

//export plugin_init
func plugin_init() {}

//export plugin_handle_hook
func plugin_handle_hook() {
	raw, _ := io.ReadAll(os.Stdin)
	var input map[string]interface{}
	_ = json.Unmarshal(raw, &input)
	point, _ := input["point"].(string)
	action, _ := input["action"].(string)
	if point == "advanced_chat.runtime_extension" {
		response := map[string]interface{}{
			"system_prompt": "The Hello Plugin is active. Use hello_plugin_echo when the user asks to test the plugin echo tool.",
			"tools": []map[string]interface{}{
				{
					"name":        "hello_plugin_echo",
					"description": "Echo a short message through the Hello Plugin WASM hook.",
					"schema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"message": map[string]interface{}{"type": "string", "description": "Message to echo."},
						},
						"required": []string{"message"},
					},
				},
			},
		}
		_ = json.NewEncoder(os.Stdout).Encode(response)
		return
	}
	if point == "advanced_chat.tool_call" && action == "hello_plugin_echo" {
		response := map[string]interface{}{
			"text": "Hello Plugin handled tool call: " + string(raw),
		}
		_ = json.NewEncoder(os.Stdout).Encode(response)
		return
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"ok": true})
}

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
