package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/WindyPear-Team/veloce/internal/config"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const (
	builtinEmbeddingModelPrefix = "builtin:"
	builtinEmbeddingModelsDir   = "embedding-models"
	builtinEmbeddingMaxInputs   = 128
	builtinEmbeddingMaxBytes    = 1024 * 1024
)

var builtinEmbeddingModelIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// BuiltinEmbeddingModel describes a pure-Go WASI model package found below
// DATA_PATH/embedding-models/<id>/manifest.json.
type BuiltinEmbeddingModel struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Dimensions     int    `json:"dimensions"`
	WASM           string `json:"wasm"`
	Entrypoint     string `json:"entrypoint"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Directory      string `json:"-"`
}

type builtinEmbeddingManifest struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Dimensions     int    `json:"dimensions"`
	WASM           string `json:"wasm"`
	Entrypoint     string `json:"entrypoint"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// InitBuiltinEmbeddingEngine prepares the model root and reports malformed
// packages without preventing the application from starting.
func InitBuiltinEmbeddingEngine() error {
	root := builtinEmbeddingModelsRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("prepare built-in embedding model directory: %w", err)
	}
	for _, err := range builtinEmbeddingModelLoadErrors() {
		log.Printf("Built-in embedding model skipped: %v", err)
	}
	return nil
}

func ListBuiltinEmbeddingModels() []BuiltinEmbeddingModel {
	entries, err := os.ReadDir(builtinEmbeddingModelsRoot())
	if err != nil {
		return []BuiltinEmbeddingModel{}
	}
	models := make([]BuiltinEmbeddingModel, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		model, err := loadBuiltinEmbeddingModel(entry.Name())
		if err == nil {
			models = append(models, model)
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models
}

func IsBuiltinEmbeddingModel(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), builtinEmbeddingModelPrefix)
}

func BuiltinEmbeddingModelName(id string) string {
	return builtinEmbeddingModelPrefix + strings.TrimSpace(id)
}

func LoadBuiltinEmbeddingModel(name string) (BuiltinEmbeddingModel, error) {
	name = strings.TrimSpace(name)
	if !IsBuiltinEmbeddingModel(name) {
		return BuiltinEmbeddingModel{}, fmt.Errorf("built-in embedding model name must start with %q", builtinEmbeddingModelPrefix)
	}
	id := strings.TrimSpace(strings.TrimPrefix(name, builtinEmbeddingModelPrefix))
	return loadBuiltinEmbeddingModel(id)
}

// CreateBuiltinEmbeddings executes a model package using Wazero's pure-Go WASI
// runtime. The WASM module receives {"input":[...]} on stdin, can read its
// package assets from /model, and must print {"embeddings":[[...]]} or an
// OpenAI-compatible {"data":[{"index":0,"embedding":[...]}]} response.
func CreateBuiltinEmbeddings(ctx context.Context, name string, input []string) ([][]float32, BuiltinEmbeddingModel, error) {
	model, err := LoadBuiltinEmbeddingModel(name)
	if err != nil {
		return nil, BuiltinEmbeddingModel{}, err
	}
	if len(input) == 0 || len(input) > builtinEmbeddingMaxInputs {
		return nil, model, fmt.Errorf("embedding input count must be between 1 and %d", builtinEmbeddingMaxInputs)
	}
	request := struct {
		Input []string `json:"input"`
	}{Input: input}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, model, err
	}
	if len(payload) > builtinEmbeddingMaxBytes {
		return nil, model, fmt.Errorf("embedding input exceeds %d bytes", builtinEmbeddingMaxBytes)
	}
	output, err := runBuiltinEmbeddingWASM(ctx, model, payload)
	if err != nil {
		return nil, model, err
	}
	vectors, err := parseBuiltinEmbeddingOutput(output, len(input), model.Dimensions)
	if err != nil {
		return nil, model, err
	}
	return vectors, model, nil
}

func builtinEmbeddingModelsRoot() string {
	return filepath.Join(config.DataPath, builtinEmbeddingModelsDir)
}

func builtinEmbeddingModelLoadErrors() []error {
	entries, err := os.ReadDir(builtinEmbeddingModelsRoot())
	if err != nil {
		return []error{err}
	}
	errors := make([]error, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			if _, err := loadBuiltinEmbeddingModel(entry.Name()); err != nil {
				errors = append(errors, err)
			}
		}
	}
	return errors
}

func loadBuiltinEmbeddingModel(id string) (BuiltinEmbeddingModel, error) {
	if !builtinEmbeddingModelIDPattern.MatchString(id) {
		return BuiltinEmbeddingModel{}, fmt.Errorf("invalid built-in embedding model id %q", id)
	}
	directory := filepath.Join(builtinEmbeddingModelsRoot(), id)
	manifestPath := filepath.Join(directory, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return BuiltinEmbeddingModel{}, fmt.Errorf("read built-in embedding manifest for %s: %w", id, err)
	}
	var manifest builtinEmbeddingManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return BuiltinEmbeddingModel{}, fmt.Errorf("parse built-in embedding manifest for %s: %w", id, err)
	}
	if strings.TrimSpace(manifest.ID) == "" {
		manifest.ID = id
	}
	if manifest.ID != id || !builtinEmbeddingModelIDPattern.MatchString(manifest.ID) {
		return BuiltinEmbeddingModel{}, fmt.Errorf("built-in embedding manifest id must match directory %q", id)
	}
	if strings.TrimSpace(manifest.Name) == "" || manifest.Dimensions < 1 || manifest.Dimensions > 8192 {
		return BuiltinEmbeddingModel{}, fmt.Errorf("built-in embedding manifest for %s is missing name or valid dimensions", id)
	}
	if strings.TrimSpace(manifest.WASM) == "" {
		manifest.WASM = "model.wasm"
	}
	wasmPath, err := builtinEmbeddingModelAssetPath(directory, manifest.WASM)
	if err != nil {
		return BuiltinEmbeddingModel{}, err
	}
	if info, err := os.Stat(wasmPath); err != nil || info.IsDir() {
		return BuiltinEmbeddingModel{}, fmt.Errorf("built-in embedding WASM is unavailable for %s", id)
	}
	if strings.TrimSpace(manifest.Entrypoint) == "" {
		manifest.Entrypoint = "embed"
	}
	if manifest.TimeoutSeconds < 1 {
		manifest.TimeoutSeconds = 60
	}
	if manifest.TimeoutSeconds > 300 {
		return BuiltinEmbeddingModel{}, fmt.Errorf("built-in embedding timeout for %s exceeds 300 seconds", id)
	}
	return BuiltinEmbeddingModel{ID: id, Name: manifest.Name, Description: manifest.Description, Dimensions: manifest.Dimensions, WASM: manifest.WASM, Entrypoint: manifest.Entrypoint, TimeoutSeconds: manifest.TimeoutSeconds, Directory: directory}, nil
}

func builtinEmbeddingModelAssetPath(directory, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.IsAbs(name) {
		return "", errors.New("built-in embedding asset path is invalid")
	}
	target := filepath.Join(directory, filepath.Clean(name))
	relative, err := filepath.Rel(directory, target)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return "", errors.New("built-in embedding asset path escapes model directory")
	}
	return target, nil
}

func runBuiltinEmbeddingWASM(ctx context.Context, model BuiltinEmbeddingModel, payload []byte) ([]byte, error) {
	wasmPath, err := builtinEmbeddingModelAssetPath(model.Directory, model.WASM)
	if err != nil {
		return nil, err
	}
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("read built-in embedding WASM: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(model.TimeoutSeconds)*time.Second)
	defer cancel()
	runtime := wazero.NewRuntime(runCtx)
	defer runtime.Close(runCtx)
	if _, err := wasi_snapshot_preview1.Instantiate(runCtx, runtime); err != nil {
		return nil, fmt.Errorf("initialize built-in embedding WASI runtime: %w", err)
	}
	var stdout, stderr bytes.Buffer
	module, err := runtime.InstantiateWithConfig(runCtx, wasm, wazero.NewModuleConfig().
		WithStdin(io.Reader(bytes.NewReader(payload))).
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithFSConfig(wazero.NewFSConfig().WithReadOnlyDirMount(model.Directory, "/model")).
		WithStartFunctions("_initialize"))
	if err != nil {
		return nil, fmt.Errorf("instantiate built-in embedding WASM: %w%s", err, builtinEmbeddingStderrSuffix(stderr.String()))
	}
	defer module.Close(runCtx)
	entrypoint := module.ExportedFunction(model.Entrypoint)
	if entrypoint == nil {
		return nil, fmt.Errorf("built-in embedding WASM does not export %q", model.Entrypoint)
	}
	if _, err := entrypoint.Call(runCtx); err != nil {
		return nil, fmt.Errorf("built-in embedding WASM failed: %w%s", err, builtinEmbeddingStderrSuffix(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func parseBuiltinEmbeddingOutput(raw []byte, count, dimensions int) ([][]float32, error) {
	var response struct {
		Embeddings [][]float32 `json:"embeddings"`
		Data       []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &response); err != nil {
		return nil, fmt.Errorf("built-in embedding WASM returned invalid JSON: %w", err)
	}
	if strings.TrimSpace(response.Error) != "" {
		return nil, errors.New(response.Error)
	}
	vectors := response.Embeddings
	if len(response.Data) > 0 {
		vectors = make([][]float32, count)
		for _, item := range response.Data {
			if item.Index >= 0 && item.Index < len(vectors) {
				vectors[item.Index] = item.Embedding
			}
		}
	}
	if len(vectors) != count {
		return nil, errors.New("built-in embedding WASM returned an incomplete embedding list")
	}
	for _, vector := range vectors {
		if len(vector) != dimensions {
			return nil, fmt.Errorf("built-in embedding WASM returned %d dimensions; expected %d", len(vector), dimensions)
		}
	}
	return vectors, nil
}

func builtinEmbeddingStderrSuffix(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return ": " + value
	}
	return ""
}
