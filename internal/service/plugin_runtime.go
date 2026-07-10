package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/WindyPear-Team/flai/internal/model"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const pluginWASMTimeout = 5 * time.Second

// InitializePluginWASM verifies that the plugin WASM can be loaded and calls
// plugin_init when the module exports it.
func InitializePluginWASM(ctx context.Context, plugin model.Plugin) error {
	if strings.TrimSpace(plugin.WASMPath) == "" {
		return nil
	}
	_, err := runPluginWASM(ctx, plugin, "plugin_init")
	return err
}

// InvokePluginAction is the backend entry point for declarative frontend
// actions. The full JSON memory ABI is intentionally kept behind this function
// so the HTTP surface stays stable while the ABI evolves.
func InvokePluginAction(ctx context.Context, plugin model.Plugin, userID uint, action string, payload map[string]interface{}) (map[string]interface{}, error) {
	if strings.TrimSpace(plugin.WASMPath) == "" {
		return nil, errors.New("plugin has no WASM module")
	}
	metadata := mustJSON(map[string]interface{}{
		"user_id": userID,
		"action":  action,
		"payload": payload,
	})
	recordPluginLog(userID, plugin.ID, "info", "action_requested", "Plugin action requested", metadata)
	stdout, err := runPluginWASM(ctx, plugin, "plugin_handle_action")
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok":     true,
		"stdout": stdout,
	}, nil
}

func runPluginWASM(ctx context.Context, plugin model.Plugin, functionName string) (string, error) {
	wasmPath := strings.TrimSpace(plugin.WASMPath)
	if wasmPath == "" {
		return "", nil
	}
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		return "", fmt.Errorf("failed to read plugin WASM: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, pluginWASMTimeout)
	defer cancel()

	runtime := wazero.NewRuntime(runCtx)
	defer runtime.Close(runCtx)
	wasi_snapshot_preview1.MustInstantiate(runCtx, runtime)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	config := wazero.NewModuleConfig().
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithStdin(io.Reader(strings.NewReader("")))
	module, err := runtime.InstantiateWithConfig(runCtx, wasm, config)
	if err != nil {
		return stdout.String(), fmt.Errorf("failed to instantiate plugin WASM: %w%s", err, pluginStderrSuffix(stderr.String()))
	}
	defer module.Close(runCtx)

	fn := module.ExportedFunction(functionName)
	if fn == nil {
		return stdout.String(), nil
	}
	if _, err := fn.Call(runCtx); err != nil {
		return stdout.String(), fmt.Errorf("plugin %s failed: %w%s", functionName, err, pluginStderrSuffix(stderr.String()))
	}
	return stdout.String(), nil
}

func pluginStderrSuffix(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}
	return ": " + stderr
}
