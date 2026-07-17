package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WindyPear-Team/veloce/internal/model"
)

const (
	pluginEmbeddingModelPrefix = "plugin:"
	pluginEmbeddingMaxInputs   = 128
	pluginEmbeddingMaxBytes    = 1024 * 1024
)

// PluginEmbeddingModel is an embedding-capable WASM plugin visible to a user.
type PluginEmbeddingModel struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Dimensions  int    `json:"dimensions"`
}

func IsPluginEmbeddingModel(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), pluginEmbeddingModelPrefix)
}

func PluginEmbeddingModelName(id string) string {
	return pluginEmbeddingModelPrefix + strings.TrimSpace(id)
}

func ListPluginEmbeddingModels(userID uint) []PluginEmbeddingModel {
	var plugins []model.Plugin
	if err := model.DB.Where("enabled = ?", true).Order("id asc").Find(&plugins).Error; err != nil {
		return []PluginEmbeddingModel{}
	}
	states := userPluginStates(userID)
	result := make([]PluginEmbeddingModel, 0, len(plugins))
	for _, plugin := range plugins {
		if !pluginUserEnabled(plugin, states) {
			continue
		}
		embedding, ok := pluginEmbeddingManifest(plugin)
		if !ok {
			continue
		}
		result = append(result, PluginEmbeddingModel{ID: plugin.ID, Model: PluginEmbeddingModelName(plugin.ID), Name: plugin.Name, Description: plugin.Description, Dimensions: embedding.Dimensions})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// CreatePluginEmbeddings invokes a declared embedding entrypoint through the
// normal WASM plugin runtime. It never selects an upstream channel or bills an
// external embedding provider.
func CreatePluginEmbeddings(ctx context.Context, userID uint, name string, input []string) ([][]float32, PluginEmbeddingModel, error) {
	plugin, embedding, err := loadPluginEmbeddingForUser(userID, name)
	if err != nil {
		return nil, PluginEmbeddingModel{}, err
	}
	embeddingModel := PluginEmbeddingModel{ID: plugin.ID, Model: PluginEmbeddingModelName(plugin.ID), Name: plugin.Name, Description: plugin.Description, Dimensions: embedding.Dimensions}
	if len(input) == 0 || len(input) > pluginEmbeddingMaxInputs {
		return nil, embeddingModel, fmt.Errorf("embedding input count must be between 1 and %d", pluginEmbeddingMaxInputs)
	}
	payload, err := json.Marshal(struct {
		Input []string `json:"input"`
	}{Input: input})
	if err != nil {
		return nil, embeddingModel, err
	}
	if len(payload) > pluginEmbeddingMaxBytes {
		return nil, embeddingModel, fmt.Errorf("embedding input exceeds %d bytes", pluginEmbeddingMaxBytes)
	}
	output, err := runPluginWASMWithTimeout(ctx, plugin, embedding.Entrypoint, payload, time.Duration(embedding.TimeoutSeconds)*time.Second)
	if err != nil {
		return nil, embeddingModel, err
	}
	vectors, err := parsePluginEmbeddingOutput([]byte(output), len(input), embedding.Dimensions)
	if err != nil {
		return nil, embeddingModel, err
	}
	return vectors, embeddingModel, nil
}

func loadPluginEmbeddingForUser(userID uint, name string) (model.Plugin, PluginEmbeddingManifest, error) {
	name = strings.TrimSpace(name)
	if !IsPluginEmbeddingModel(name) {
		return model.Plugin{}, PluginEmbeddingManifest{}, fmt.Errorf("embedding plugin model name must start with %q", pluginEmbeddingModelPrefix)
	}
	id := strings.TrimSpace(strings.TrimPrefix(name, pluginEmbeddingModelPrefix))
	var plugin model.Plugin
	if id == "" || model.DB.Where("id = ?", id).Limit(1).Find(&plugin).Error != nil || plugin.ID == "" {
		return model.Plugin{}, PluginEmbeddingManifest{}, errors.New("embedding plugin not found")
	}
	if !pluginUserEnabled(plugin, userPluginStates(userID)) {
		return model.Plugin{}, PluginEmbeddingManifest{}, errors.New("embedding plugin is disabled")
	}
	embedding, ok := pluginEmbeddingManifest(plugin)
	if !ok {
		return model.Plugin{}, PluginEmbeddingManifest{}, errors.New("plugin does not provide embeddings")
	}
	return plugin, embedding, nil
}

func pluginEmbeddingManifest(plugin model.Plugin) (PluginEmbeddingManifest, bool) {
	var manifest PluginManifest
	if strings.TrimSpace(plugin.ManifestJSON) == "" || json.Unmarshal([]byte(plugin.ManifestJSON), &manifest) != nil {
		return PluginEmbeddingManifest{}, false
	}
	manifest = normalizePluginManifest(manifest)
	if manifest.Embedding == nil || validatePluginManifest(manifest) != nil {
		return PluginEmbeddingManifest{}, false
	}
	return *manifest.Embedding, true
}

func parsePluginEmbeddingOutput(raw []byte, count, dimensions int) ([][]float32, error) {
	var response struct {
		Embeddings [][]float32 `json:"embeddings"`
		Data       []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &response); err != nil {
		return nil, fmt.Errorf("embedding plugin returned invalid JSON: %w", err)
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
		return nil, errors.New("embedding plugin returned an incomplete embedding list")
	}
	for _, vector := range vectors {
		if len(vector) != dimensions {
			return nil, fmt.Errorf("embedding plugin returned %d dimensions; expected %d", len(vector), dimensions)
		}
	}
	return vectors, nil
}
