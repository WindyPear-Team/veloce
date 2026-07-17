package service

import "testing"

func TestParseBuiltinEmbeddingOutput(t *testing.T) {
	vectors, err := parseBuiltinEmbeddingOutput([]byte(`{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}]}`), 2, 2)
	if err != nil {
		t.Fatalf("parse OpenAI-style output: %v", err)
	}
	if len(vectors) != 2 || vectors[0][0] != 1 || vectors[1][0] != 3 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
	if _, err := parseBuiltinEmbeddingOutput([]byte(`{"embeddings":[[1]]}`), 1, 2); err == nil {
		t.Fatal("expected dimension validation error")
	}
}

func TestLoadBuiltinEmbeddingModelRequiresPrefix(t *testing.T) {
	if _, err := LoadBuiltinEmbeddingModel("local-model"); err == nil {
		t.Fatal("expected non-built-in model name to be rejected")
	}
}
