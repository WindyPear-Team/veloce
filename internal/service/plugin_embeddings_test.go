package service

import "testing"

func TestParsePluginEmbeddingOutput(t *testing.T) {
	vectors, err := parsePluginEmbeddingOutput([]byte(`{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}]}`), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || vectors[0][0] != 1 || vectors[1][1] != 4 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
	if _, err := parsePluginEmbeddingOutput([]byte(`{"embeddings":[[1]]}`), 1, 2); err == nil {
		t.Fatal("expected dimensions validation error")
	}
}

func TestPluginEmbeddingManifestDefaults(t *testing.T) {
	manifest := normalizePluginManifest(PluginManifest{Embedding: &PluginEmbeddingManifest{Dimensions: 384}})
	if manifest.Embedding.Entrypoint != "embedding" || manifest.Embedding.TimeoutSeconds != 60 {
		t.Fatalf("unexpected defaults: %#v", manifest.Embedding)
	}
}
