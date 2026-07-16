package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/WindyPear-Team/veloce/internal/config"
	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	advancedChatKnowledgeEmbeddingPending    = "pending"
	advancedChatKnowledgeEmbeddingProcessing = "processing"
	advancedChatKnowledgeEmbeddingReady      = "ready"
	advancedChatKnowledgeEmbeddingFailed     = "failed"
	advancedChatKnowledgeEmbeddingSkipped    = "skipped"
)

type AdvancedChatKnowledgeChunk struct {
	ID              string    `gorm:"primaryKey;size:80" json:"id"`
	KnowledgeBaseID string    `gorm:"index:idx_advanced_chat_knowledge_chunk_scope;size:80;not null" json:"knowledge_base_id"`
	DocumentID      string    `gorm:"index:idx_advanced_chat_knowledge_chunk_scope;size:80;not null" json:"document_id"`
	UserID          uint      `gorm:"index:idx_advanced_chat_knowledge_chunk_scope;not null" json:"user_id"`
	Ordinal         int       `gorm:"not null" json:"ordinal"`
	Content         string    `gorm:"type:text;not null" json:"content"`
	ContentHash     string    `gorm:"size:64;not null" json:"content_hash"`
	Embedding       []byte    `gorm:"not null" json:"-"`
	EmbeddingModel  string    `gorm:"index:idx_advanced_chat_knowledge_chunk_scope;size:120;not null" json:"embedding_model"`
	EmbeddingDim    int       `gorm:"index:idx_advanced_chat_knowledge_chunk_scope;not null" json:"embedding_dimensions"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type advancedChatKnowledgeEmbeddingConfig struct {
	ModelName     string
	UserChannelID uint
}

type advancedChatKnowledgeSearchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type advancedChatKnowledgeSearchResult struct {
	DocumentID string  `json:"document_id"`
	ChunkID    string  `json:"chunk_id"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

var (
	advancedChatKnowledgeEmbeddingQueue = make(chan string, 128)
	advancedChatKnowledgeEmbeddingOnce  sync.Once
)

func startAdvancedChatKnowledgeEmbeddingWorker() {
	advancedChatKnowledgeEmbeddingOnce.Do(func() {
		go func() {
			processAdvancedChatKnowledgeEmbeddingQueue()
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				processAdvancedChatKnowledgeEmbeddingQueue()
			}
		}()
	})
}

func queueAdvancedChatKnowledgeEmbedding(documentID string) {
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return
	}
	select {
	case advancedChatKnowledgeEmbeddingQueue <- documentID:
		go processAdvancedChatKnowledgeEmbeddingQueue()
	default:
	}
}

func processAdvancedChatKnowledgeEmbeddingQueue() {
	for {
		var documentID string
		select {
		case documentID = <-advancedChatKnowledgeEmbeddingQueue:
		default:
			var pending AdvancedChatKnowledgeDocument
			if err := model.DB.Table("advanced_chat_knowledge_documents").
				Joins("JOIN advanced_chat_user_settings ON advanced_chat_user_settings.user_id = advanced_chat_knowledge_documents.user_id").
				Where("advanced_chat_knowledge_documents.embedding_status = ? AND TRIM(advanced_chat_user_settings.knowledge_embedding_model_name) <> ''", advancedChatKnowledgeEmbeddingPending).
				Order("advanced_chat_knowledge_documents.updated_at ASC").First(&pending).Error; err != nil {
				return
			}
			documentID = pending.ID
		}
		_ = processAdvancedChatKnowledgeDocumentEmbedding(documentID)
	}
}

func advancedChatKnowledgeEmbeddingSettings(userID uint) advancedChatKnowledgeEmbeddingConfig {
	settings := ensureAdvancedChatUserSettings(userID)
	return advancedChatKnowledgeEmbeddingConfig{
		ModelName:     strings.TrimSpace(settings.KnowledgeEmbeddingModelName),
		UserChannelID: settings.KnowledgeEmbeddingUserChannelID,
	}
}

func (c advancedChatKnowledgeEmbeddingConfig) configured() bool {
	return c.ModelName != ""
}

func processAdvancedChatKnowledgeDocumentEmbedding(documentID string) error {
	var document AdvancedChatKnowledgeDocument
	var file AdvancedChatFile
	if err := model.DB.Where("id = ?", documentID).First(&document).Error; err != nil {
		return err
	}
	cfg := advancedChatKnowledgeEmbeddingSettings(document.UserID)
	if !cfg.configured() {
		return nil
	}
	claimed := model.DB.Model(&AdvancedChatKnowledgeDocument{}).
		Where("id = ? AND embedding_status = ?", documentID, advancedChatKnowledgeEmbeddingPending).
		Updates(map[string]interface{}{"embedding_status": advancedChatKnowledgeEmbeddingProcessing, "embedding_error": ""})
	if claimed.Error != nil || claimed.RowsAffected != 1 {
		return claimed.Error
	}
	if err := model.DB.Where("id = ? AND user_id = ?", document.FileID, document.UserID).First(&file).Error; err != nil {
		return failAdvancedChatKnowledgeDocumentEmbedding(documentID, "Knowledge file is unavailable")
	}
	chunks := advancedChatKnowledgeTextChunks(file.TextExtract)
	if len(chunks) == 0 {
		return model.DB.Model(&AdvancedChatKnowledgeDocument{}).Where("id = ?", documentID).Updates(map[string]interface{}{
			"embedding_status": advancedChatKnowledgeEmbeddingSkipped, "embedding_error": "No extractable text", "chunk_count": 0,
		}).Error
	}
	var user model.User
	if err := model.DB.First(&user, document.UserID).Error; err != nil {
		return failAdvancedChatKnowledgeDocumentEmbedding(documentID, "Knowledge owner is unavailable")
	}
	embeddings, err := createAdvancedChatKnowledgeEmbeddings(context.Background(), nil, &user, cfg, chunks)
	if err != nil {
		return failAdvancedChatKnowledgeDocumentEmbedding(documentID, err.Error())
	}
	if len(embeddings) != len(chunks) || len(embeddings) == 0 {
		return failAdvancedChatKnowledgeDocumentEmbedding(documentID, "Embedding provider returned incomplete data")
	}
	dimension := len(embeddings[0])
	for _, embedding := range embeddings {
		if len(embedding) != dimension || dimension == 0 {
			return failAdvancedChatKnowledgeDocumentEmbedding(documentID, "Embedding provider returned inconsistent dimensions")
		}
	}

	now := time.Now()
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("document_id = ? AND user_id = ?", document.ID, document.UserID).Delete(&AdvancedChatKnowledgeChunk{}).Error; err != nil {
			return err
		}
		for index, content := range chunks {
			chunk := AdvancedChatKnowledgeChunk{
				ID: newAdvancedChatID("akc"), KnowledgeBaseID: document.KnowledgeBaseID, DocumentID: document.ID, UserID: document.UserID,
				Ordinal: index, Content: content, ContentHash: advancedChatKnowledgeContentHash(content), Embedding: encodeAdvancedChatKnowledgeEmbedding(embeddings[index]),
				EmbeddingModel: cfg.ModelName, EmbeddingDim: dimension,
			}
			if err := tx.Create(&chunk).Error; err != nil {
				return err
			}
			if err := saveAdvancedChatKnowledgeNativeVector(tx, chunk.ID, embeddings[index]); err != nil {
				return err
			}
		}
		return tx.Model(&AdvancedChatKnowledgeDocument{}).Where("id = ?", document.ID).Updates(map[string]interface{}{
			"embedding_status": advancedChatKnowledgeEmbeddingReady, "embedding_error": "", "embedding_model": cfg.ModelName,
			"embedding_dim": dimension, "chunk_count": len(chunks), "embedded_at": &now,
		}).Error
	})
	if err != nil {
		return failAdvancedChatKnowledgeDocumentEmbedding(documentID, err.Error())
	}
	return nil
}

func failAdvancedChatKnowledgeDocumentEmbedding(documentID, message string) error {
	message = strings.TrimSpace(message)
	if len(message) > 1000 {
		message = message[:1000]
	}
	return model.DB.Model(&AdvancedChatKnowledgeDocument{}).Where("id = ?", documentID).Updates(map[string]interface{}{
		"embedding_status": advancedChatKnowledgeEmbeddingFailed, "embedding_error": message,
	}).Error
}

func advancedChatKnowledgeTextChunks(text string) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	const maxRunes = 1200
	const overlapWords = 24
	chunks := make([]string, 0, len(words)/120+1)
	for start := 0; start < len(words); {
		end := start
		length := 0
		for end < len(words) {
			wordLen := utf8.RuneCountInString(words[end])
			if end > start {
				wordLen++
			}
			if end > start && length+wordLen > maxRunes {
				break
			}
			length += wordLen
			end++
		}
		chunks = append(chunks, strings.Join(words[start:end], " "))
		if end == len(words) {
			break
		}
		next := end - overlapWords
		if next <= start {
			next = end
		}
		start = next
	}
	return chunks
}

func createAdvancedChatKnowledgeEmbeddings(ctx context.Context, requestContext *gin.Context, user *model.User, cfg advancedChatKnowledgeEmbeddingConfig, input []string) ([][]float32, error) {
	if user == nil {
		return nil, errors.New("Knowledge owner is required")
	}
	candidates, err := serverChatCandidates(cfg.ModelName, cfg.UserChannelID)
	if err != nil || len(candidates) == 0 {
		return nil, errors.New("No available channel for the configured embedding model")
	}
	modelConfig := serverChatExecutor().selectModelConfig(candidates, cfg.ModelName)
	channel := modelConfig.Channel
	if channelProtocol(channel.Type) != protocolOpenAI && channelProtocol(channel.Type) != protocolResponses {
		return nil, errors.New("The configured embedding channel must support the OpenAI embeddings API")
	}
	if err := ValidateConfiguredHTTPURL(channel.BaseURL); err != nil {
		return nil, errors.New("Embedding upstream URL is blocked")
	}
	upstreamModel := strings.TrimSpace(modelConfig.UpstreamModelName)
	if upstreamModel == "" {
		upstreamModel = cfg.ModelName
	}
	payload, err := json.Marshal(map[string]interface{}{"model": upstreamModel, "input": input, "encoding_format": "float"})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, upstreamURLForRequest(channel.BaseURL, "/v1/embeddings"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+channel.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("Embedding upstream request failed: %w", err)
	}
	defer resp.Body.Close()
	var body struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage map[string]interface{} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("Invalid embedding response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		if strings.TrimSpace(body.Error.Message) != "" {
			return nil, errors.New(body.Error.Message)
		}
		return nil, fmt.Errorf("Embedding upstream returned HTTP %d", resp.StatusCode)
	}
	usagePayload := map[string]interface{}{"usage": body.Usage}
	usage, usageOK := parseUsageTokens(usagePayload)
	if !usageOK {
		usage.InputTokens = len(strings.Join(input, " ")) / 4
		if usage.InputTokens < 1 {
			usage.InputTokens = 1
		}
	}
	if _, status, message, billErr := serverChatExecutor().billServerUsage(requestContext, user, nil, &channel, &modelConfig, cfg.ModelName, usage, false); billErr != nil {
		if status == http.StatusPaymentRequired {
			return nil, errors.New("Insufficient balance for knowledge embedding")
		}
		return nil, fmt.Errorf("Failed to charge knowledge embedding: %s", message)
	}
	result := make([][]float32, len(input))
	for _, item := range body.Data {
		if item.Index >= 0 && item.Index < len(result) {
			result[item.Index] = item.Embedding
		}
	}
	return result, nil
}

func encodeAdvancedChatKnowledgeEmbedding(vector []float32) []byte {
	encoded := make([]byte, len(vector)*4)
	for index, value := range vector {
		binary.LittleEndian.PutUint32(encoded[index*4:], math.Float32bits(value))
	}
	return encoded
}

func decodeAdvancedChatKnowledgeEmbedding(data []byte) ([]float32, bool) {
	if len(data) == 0 || len(data)%4 != 0 {
		return nil, false
	}
	result := make([]float32, len(data)/4)
	for index := range result {
		result[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
	}
	return result, true
}

func advancedChatKnowledgeContentHash(content string) string {
	// Existing file hashes are SHA-256; keeping the same stable format avoids duplicate chunks during re-indexing.
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}

func cosineAdvancedChatKnowledgeSimilarity(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for index := range left {
		dot += float64(left[index] * right[index])
		leftNorm += float64(left[index] * left[index])
		rightNorm += float64(right[index] * right[index])
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / math.Sqrt(leftNorm*rightNorm)
}

func (api *advancedChatAPI) reindexKnowledgeDocument(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	base, found := loadAdvancedChatKnowledgeBase(c, user.ID, c.Param("id"))
	if !found {
		return
	}
	var document AdvancedChatKnowledgeDocument
	if err := model.DB.Where("id = ? AND knowledge_base_id = ? AND user_id = ?", c.Param("document_id"), base.ID, user.ID).First(&document).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Knowledge document not found"})
		return
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("document_id = ? AND user_id = ?", document.ID, user.ID).Delete(&AdvancedChatKnowledgeChunk{}).Error; err != nil {
			return err
		}
		return tx.Model(&document).Updates(map[string]interface{}{"embedding_status": advancedChatKnowledgeEmbeddingPending, "embedding_error": "", "chunk_count": 0, "embedding_model": "", "embedding_dim": 0}).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue document indexing"})
		return
	}
	queueAdvancedChatKnowledgeEmbedding(document.ID)
	c.JSON(http.StatusOK, gin.H{"document": advancedChatKnowledgeDocumentResponseFromModel(document)})
}

func (api *advancedChatAPI) updateKnowledgeEmbeddingSettings(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var input advancedChatKnowledgeEmbeddingSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	modelName := strings.TrimSpace(input.ModelName)
	if len([]rune(modelName)) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Embedding model name is too long"})
		return
	}
	settings := ensureAdvancedChatUserSettings(user.ID)
	changed := settings.KnowledgeEmbeddingModelName != modelName || settings.KnowledgeEmbeddingUserChannelID != input.UserChannelID
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&settings).Updates(map[string]interface{}{"knowledge_embedding_model_name": modelName, "knowledge_embedding_user_channel_id": input.UserChannelID}).Error; err != nil {
			return err
		}
		if !changed {
			return nil
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&AdvancedChatKnowledgeChunk{}).Error; err != nil {
			return err
		}
		return tx.Model(&AdvancedChatKnowledgeDocument{}).Where("user_id = ? AND text_available = ?", user.ID, true).Updates(map[string]interface{}{"embedding_status": advancedChatKnowledgeEmbeddingPending, "embedding_error": "", "embedding_model": "", "embedding_dim": 0, "chunk_count": 0}).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save knowledge embedding settings"})
		return
	}
	if changed && modelName != "" {
		var documents []AdvancedChatKnowledgeDocument
		_ = model.DB.Where("user_id = ? AND text_available = ?", user.ID, true).Find(&documents).Error
		for _, document := range documents {
			queueAdvancedChatKnowledgeEmbedding(document.ID)
		}
	}
	c.JSON(http.StatusOK, currentAdvancedChatUserSettings(user.ID))
}

func (api *advancedChatAPI) searchKnowledgeBase(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	base, found := loadAdvancedChatKnowledgeBase(c, user.ID, c.Param("id"))
	if !found {
		return
	}
	var input advancedChatKnowledgeSearchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Search query is required"})
		return
	}
	if input.Limit < 1 {
		input.Limit = 5
	}
	if input.Limit > 20 {
		input.Limit = 20
	}
	cfg := advancedChatKnowledgeEmbeddingSettings(user.ID)
	if !cfg.configured() {
		c.JSON(http.StatusConflict, gin.H{"error": "Knowledge embedding is not configured"})
		return
	}
	vectors, err := createAdvancedChatKnowledgeEmbeddings(c.Request.Context(), c, user, cfg, []string{input.Query})
	if err != nil || len(vectors) != 1 || len(vectors[0]) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to embed search query"})
		return
	}
	results, err := searchAdvancedChatKnowledgeChunks(user.ID, base.ID, cfg.ModelName, vectors[0], input.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search knowledge base"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

func searchAdvancedChatKnowledgeChunks(userID uint, baseID, embeddingModel string, vector []float32, limit int) ([]advancedChatKnowledgeSearchResult, error) {
	if advancedChatKnowledgePostgresVectorAvailable() {
		var rows []advancedChatKnowledgeSearchResult
		err := model.DB.Raw("SELECT document_id, id AS chunk_id, content, 1 - (embedding_vector <=> CAST(? AS vector)) AS score FROM advanced_chat_knowledge_chunks WHERE user_id = ? AND knowledge_base_id = ? AND embedding_model = ? AND embedding_dim = ? AND embedding_vector IS NOT NULL ORDER BY embedding_vector <=> CAST(? AS vector) LIMIT ?", postgresVectorLiteral(vector), userID, baseID, embeddingModel, len(vector), postgresVectorLiteral(vector), limit).Scan(&rows).Error
		if err == nil {
			return rows, nil
		}
	}
	var chunks []AdvancedChatKnowledgeChunk
	if err := model.DB.Where("user_id = ? AND knowledge_base_id = ? AND embedding_model = ? AND embedding_dim = ?", userID, baseID, embeddingModel, len(vector)).Find(&chunks).Error; err != nil {
		return nil, err
	}
	results := make([]advancedChatKnowledgeSearchResult, 0, len(chunks))
	for _, chunk := range chunks {
		candidate, valid := decodeAdvancedChatKnowledgeEmbedding(chunk.Embedding)
		if !valid {
			continue
		}
		results = append(results, advancedChatKnowledgeSearchResult{DocumentID: chunk.DocumentID, ChunkID: chunk.ID, Content: chunk.Content, Score: cosineAdvancedChatKnowledgeSimilarity(vector, candidate)})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func advancedChatKnowledgePostgresVectorAvailable() bool {
	if config.DBDriver != "postgres" && config.DBDriver != "postgresql" {
		return false
	}
	var count int64
	if err := model.DB.Raw("SELECT COUNT(*) FROM pg_extension WHERE extname = 'vector'").Scan(&count).Error; err != nil || count == 0 {
		return false
	}
	var columnCount int64
	return model.DB.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'advanced_chat_knowledge_chunks' AND column_name = 'embedding_vector'").Scan(&columnCount).Error == nil && columnCount > 0
}

func ensureAdvancedChatKnowledgePostgresVectorColumn() {
	if config.DBDriver != "postgres" && config.DBDriver != "postgresql" {
		return
	}
	var count int64
	if err := model.DB.Raw("SELECT COUNT(*) FROM pg_extension WHERE extname = 'vector'").Scan(&count).Error; err != nil || count == 0 {
		return
	}
	// The extension is optional. Do not attempt CREATE EXTENSION here because many hosted PostgreSQL roles cannot do so.
	_ = model.DB.Exec("ALTER TABLE advanced_chat_knowledge_chunks ADD COLUMN IF NOT EXISTS embedding_vector vector").Error
}

func saveAdvancedChatKnowledgeNativeVector(tx *gorm.DB, chunkID string, vector []float32) error {
	if !advancedChatKnowledgePostgresVectorAvailable() {
		return nil
	}
	return tx.Exec("UPDATE advanced_chat_knowledge_chunks SET embedding_vector = CAST(? AS vector) WHERE id = ?", postgresVectorLiteral(vector), chunkID).Error
}

func postgresVectorLiteral(vector []float32) string {
	values := make([]string, len(vector))
	for index, value := range vector {
		values[index] = fmt.Sprintf("%g", value)
	}
	return "[" + strings.Join(values, ",") + "]"
}
