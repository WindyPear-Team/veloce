package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (api *advancedChatAPI) listBuiltinEmbeddingModels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"models": ListBuiltinEmbeddingModels()})
}

func (api *advancedChatAPI) createBuiltinEmbeddings(c *gin.Context) {
	if _, ok := currentAdvancedChatUser(c); !ok {
		return
	}
	var input struct {
		Model string      `json:"model"`
		Input interface{} `json:"input"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	texts, ok := builtinEmbeddingInputStrings(input.Input)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Embedding input must be a string or string array"})
		return
	}
	vectors, model, err := CreateBuiltinEmbeddings(c.Request.Context(), input.Model, texts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data := make([]gin.H, len(vectors))
	for index, vector := range vectors {
		data[index] = gin.H{"object": "embedding", "index": index, "embedding": vector}
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "model": BuiltinEmbeddingModelName(model.ID), "data": data, "usage": gin.H{"prompt_tokens": 0, "total_tokens": 0}})
}

func builtinEmbeddingInputStrings(value interface{}) ([]string, bool) {
	switch input := value.(type) {
	case string:
		return []string{input}, strings.TrimSpace(input) != ""
	case []interface{}:
		result := make([]string, 0, len(input))
		for _, item := range input {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, false
			}
			result = append(result, text)
		}
		return result, len(result) > 0
	default:
		return nil, false
	}
}
