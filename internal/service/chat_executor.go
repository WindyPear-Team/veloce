package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/WindyPear-Team/flai/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ChatExecutorMessage is a single message in a server-side chat completion call.
// Content may be empty when the assistant message only carries tool calls, and
// ToolCalls / ToolCallID carry OpenAI tool-calling state across turns.
type ChatExecutorMessage struct {
	Role       string                   `json:"role"`
	Content    string                   `json:"content"`
	ToolCalls  []map[string]interface{} `json:"tool_calls,omitempty"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
	Name       string                   `json:"name,omitempty"`
}

// ChatExecutorTool describes a tool exposed to the upstream model. Schema is the
// JSON Schema object for the tool's parameters.
type ChatExecutorTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Schema      map[string]interface{} `json:"schema"`
}

// ChatExecutorRequest is one server-side, billed completion turn against a model.
// UserChannelID, when non-zero, pins the request to a single user-facing channel
// (渠道) exactly like an API key bound to that channel would.
type ChatExecutorRequest struct {
	ModelName     string
	UserChannelID uint
	Messages      []ChatExecutorMessage
	System        string
	Tools         []ChatExecutorTool
	MaxTokens     int
	Temperature   *float64
}

// ChatExecutorToolCall is a tool invocation requested by the model.
type ChatExecutorToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ChatExecutorResult is the outcome of a single completion turn.
type ChatExecutorResult struct {
	Content   string
	ToolCalls []ChatExecutorToolCall
	// AssistantMessage is the raw assistant message as returned upstream, suitable
	// for appending verbatim to the next turn's message list.
	AssistantMessage map[string]interface{}
	FinishReason     string
	Cost             decimal.Decimal
}

// ChatExecutorError carries an HTTP status so callers can surface upstream and
// billing failures with the right code.
type ChatExecutorError struct {
	Status  int
	Message string
}

func (e *ChatExecutorError) Error() string { return e.Message }

func newChatExecutorError(status int, message string) *ChatExecutorError {
	return &ChatExecutorError{Status: status, Message: message}
}

// ExecuteServerChatCompletion runs one billed chat-completion turn for a user
// directly against a channel, without going through the public /v1 HTTP endpoint
// or a user token. It selects a channel for (model, userChannel) using the same
// routing the proxy uses, calls the OpenAI-protocol upstream (optionally with
// tools), bills the user via the shared usage-charge path, and returns the
// assistant message plus any tool calls.
//
// Only OpenAI-protocol channels support tools. When tools are requested but the
// resolved channel speaks another protocol, the call fails with a clear error so
// the caller can retry without tools.
func ExecuteServerChatCompletion(c *gin.Context, user *model.User, req ChatExecutorRequest) (*ChatExecutorResult, error) {
	if user == nil {
		return nil, newChatExecutorError(http.StatusUnauthorized, "Unauthorized")
	}
	modelName := strings.TrimSpace(strings.TrimPrefix(req.ModelName, "models/"))
	if modelName == "" {
		return nil, newChatExecutorError(http.StatusBadRequest, "Model not specified")
	}
	if user.Balance.LessThanOrEqual(decimal.Zero) {
		return nil, newChatExecutorError(http.StatusPaymentRequired, "Insufficient balance")
	}

	candidates, err := serverChatCandidates(modelName, req.UserChannelID)
	if err != nil {
		return nil, newChatExecutorError(http.StatusInternalServerError, "Failed to find available channels")
	}
	if len(candidates) == 0 {
		return nil, newChatExecutorError(http.StatusServiceUnavailable, "No available channel for this model")
	}

	executor := serverChatExecutor()
	modelConfig := executor.selectModelConfig(candidates, modelName)
	channel := modelConfig.Channel
	if channel.ID == 0 {
		return nil, newChatExecutorError(http.StatusServiceUnavailable, "No enabled model configuration for this model")
	}

	protocol := channelProtocol(channel.Type)
	if len(req.Tools) > 0 && protocol != protocolOpenAI {
		return nil, newChatExecutorError(http.StatusBadRequest, "Selected channel does not support tool calling")
	}

	if err := ValidateConfiguredHTTPURL(channel.BaseURL); err != nil {
		return nil, newChatExecutorError(http.StatusBadGateway, "Upstream URL blocked by SSRF protection")
	}

	upstreamModelName := strings.TrimSpace(modelConfig.UpstreamModelName)
	if upstreamModelName == "" {
		upstreamModelName = modelName
	}

	prepared, err := prepareServerChatRequest(&channel, protocol, upstreamModelName, req)
	if err != nil {
		return nil, newChatExecutorError(http.StatusBadRequest, err.Error())
	}

	resp, err := executor.doUpstreamRequest(prepared)
	if err != nil {
		logUpstreamRequestFailure(c, &channel, prepared.URL, prepared.Body, err)
		return nil, newChatExecutorError(http.StatusBadGateway, "Upstream request failed")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newChatExecutorError(http.StatusBadGateway, "Failed to read upstream response")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		logUpstreamError(c, &channel, prepared.URL, resp.StatusCode, prepared.Body, respBody)
		return nil, newChatExecutorError(resp.StatusCode, "Upstream request failed")
	}

	var responseData map[string]interface{}
	if err := json.Unmarshal(respBody, &responseData); err != nil {
		return nil, newChatExecutorError(http.StatusBadGateway, "Failed to parse upstream response")
	}

	usage, ok := parseUsageTokens(responseData)
	if !ok {
		usage = usageTokenCounts{
			InputTokens:  CountTokens(modelName, serverChatMessagesText(req)),
			OutputTokens: CountTokens(modelName, string(respBody)),
		}
	}

	apiKey := currentAPIKey(c)
	cost, status, message, err := executor.billServerUsage(c, user, apiKey, &channel, &modelConfig, modelName, usage)
	if err != nil {
		return nil, newChatExecutorError(status, message)
	}

	result := parseServerChatResponse(responseData)
	result.Cost = cost
	return result, nil
}

func serverChatExecutor() *ProxyService {
	return NewProxyService()
}

func serverChatCandidates(modelName string, userChannelID uint) ([]model.ModelConfig, error) {
	var candidates []model.ModelConfig
	query := model.DB.
		Preload("Channel.UserChannel").
		Preload("Model").
		Joins("JOIN channels ON channels.id = model_configs.channel_id").
		Joins("JOIN models ON models.id = model_configs.model_id").
		Joins("JOIN user_channels ON user_channels.id = channels.user_channel_id").
		Where("channels.enabled = ? AND model_configs.enabled = ? AND models.enabled = ? AND models.model_name = ? AND user_channels.enabled = ?", true, true, true, modelName, true)
	if userChannelID != 0 {
		query = query.Where("channels.user_channel_id = ?", userChannelID)
	}
	if err := query.Order("channels.priority DESC, channels.weight DESC, channels.id ASC").Find(&candidates).Error; err != nil {
		return nil, err
	}
	return candidates, nil
}

// billServerUsage mirrors ProxyService.billUsage but returns the computed cost so
// the caller can accumulate it across an agent loop. It reuses the same group and
// channel multipliers, balance deduction, token log, and referral commission.
func (s *ProxyService) billServerUsage(c *gin.Context, user *model.User, apiKey *model.APIKey, channel *model.Channel, modelConfig *model.ModelConfig, modelName string, usage usageTokenCounts) (decimal.Decimal, int, string, error) {
	groupMultiplier, err := effectiveUserGroupMultiplier(user, channel.ID, modelConfig.ID)
	if err != nil {
		return decimal.Zero, http.StatusInternalServerError, "User group not found", err
	}
	usage = normalizeUsageTokenCounts(usage)
	billingModel := modelConfig.Model
	cost := calculateModelUsageCost(usage, billingModel).
		Mul(groupMultiplier).
		Mul(userChannelMultiplier(channel))

	tx := model.DB.Begin()
	if tx.Error != nil {
		return decimal.Zero, http.StatusInternalServerError, "Failed to start transaction", tx.Error
	}
	if exceeded, err := APIKeyQuotaExceededInTx(tx, apiKey, cost); err != nil {
		tx.Rollback()
		return decimal.Zero, http.StatusInternalServerError, "Failed to check API key quota", err
	} else if exceeded {
		tx.Rollback()
		return decimal.Zero, http.StatusPaymentRequired, "API key quota exceeded", ErrAPIKeyQuotaExceeded
	}
	if err := ApplyUsageCharge(tx, user.ID, cost); err != nil {
		tx.Rollback()
		if errors.Is(err, ErrInsufficientBalance) {
			return decimal.Zero, http.StatusPaymentRequired, "Insufficient balance", err
		}
		return decimal.Zero, http.StatusInternalServerError, "Failed to update balance", err
	}
	tokenLog := model.TokenLog{
		UserID:                  user.ID,
		APIKeyID:                apiKeyID(apiKey),
		UserChannelID:           channel.UserChannelID,
		ChannelID:               channel.ID,
		ModelName:               modelName,
		InputTokens:             usage.InputTokens,
		OutputTokens:            usage.OutputTokens,
		CachedInputTokens:       usage.CachedInputTokens,
		CacheWriteInputTokens:   usage.CacheWriteInputTokens,
		CacheWrite1hInputTokens: usage.CacheWrite1hInputTokens,
		ImageInputTokens:        usage.ImageInputTokens,
		ImageOutputTokens:       usage.ImageOutputTokens,
		AudioInputTokens:        usage.AudioInputTokens,
		AudioOutputTokens:       usage.AudioOutputTokens,
		Cost:                    cost,
		IP:                      clientIPForLog(c),
		UserAgent:               userAgentForLog(c),
		CreatedAt:               time.Now(),
	}
	if err := tx.Create(&tokenLog).Error; err != nil {
		tx.Rollback()
		return decimal.Zero, http.StatusInternalServerError, "Failed to log usage", err
	}
	if err := applyReferralCommission(tx, user, tokenLog.ID, cost); err != nil {
		tx.Rollback()
		return decimal.Zero, http.StatusInternalServerError, "Failed to apply referral commission", err
	}
	if err := tx.Commit().Error; err != nil {
		return decimal.Zero, http.StatusInternalServerError, "Failed to commit usage", err
	}
	return cost, 0, "", nil
}

func clientIPForLog(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.ClientIP()
}

func userAgentForLog(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return c.Request.UserAgent()
}

// prepareServerChatRequest builds the upstream HTTP request. Tool-enabled requests
// are only built for the OpenAI protocol (guarded by the caller); other protocols
// reuse the normalized text payload builders.
func prepareServerChatRequest(channel *model.Channel, protocol proxyProtocol, upstreamModelName string, req ChatExecutorRequest) (preparedUpstreamRequest, error) {
	if protocol == protocolOpenAI {
		return prepareServerOpenAIChatRequest(channel, upstreamModelName, req)
	}
	normalized := normalizedAIRequest{
		Model:       upstreamModelName,
		System:      req.System,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	for _, message := range req.Messages {
		addNormalizedMessage(&normalized, message.Role, message.Content)
	}
	return prepareProviderRequest(channel, protocol, normalized)
}

func prepareServerOpenAIChatRequest(channel *model.Channel, upstreamModelName string, req ChatExecutorRequest) (preparedUpstreamRequest, error) {
	messages := make([]map[string]interface{}, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, map[string]interface{}{"role": "system", "content": req.System})
	}
	for _, message := range req.Messages {
		entry := map[string]interface{}{"role": message.Role}
		if message.Content != "" || (len(message.ToolCalls) == 0 && message.Role != "assistant") {
			entry["content"] = message.Content
		}
		if len(message.ToolCalls) > 0 {
			entry["tool_calls"] = message.ToolCalls
		}
		if message.ToolCallID != "" {
			entry["tool_call_id"] = message.ToolCallID
		}
		if message.Name != "" {
			entry["name"] = message.Name
		}
		messages = append(messages, entry)
	}
	if len(messages) == 0 {
		return preparedUpstreamRequest{}, errors.New("messages are required")
	}
	payload := map[string]interface{}{
		"model":    upstreamModelName,
		"messages": messages,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(req.Tools))
		for _, tool := range req.Tools {
			parameters := tool.Schema
			if parameters == nil {
				parameters = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
			}
			tools = append(tools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  parameters,
				},
			})
		}
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return preparedUpstreamRequest{}, err
	}
	headers := jsonHeaders()
	headers.Set("Authorization", "Bearer "+strings.TrimSpace(channel.APIKey))
	return preparedUpstreamRequest{
		Method: http.MethodPost,
		URL:    upstreamURLForRequest(channel.BaseURL, "/v1/chat/completions"),
		Body:   body,
		Header: headers,
	}, nil
}

func parseServerChatResponse(responseData map[string]interface{}) *ChatExecutorResult {
	result := &ChatExecutorResult{}
	choices, ok := responseData["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		result.Content = openAIResponseText(responseData)
		return result
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return result
	}
	result.FinishReason = stringFromValue(choice["finish_reason"])
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		result.Content = openAIResponseText(responseData)
		return result
	}
	result.AssistantMessage = message
	result.Content = stringFromValue(message["content"])
	if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
		for _, raw := range toolCalls {
			call, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			function, _ := call["function"].(map[string]interface{})
			result.ToolCalls = append(result.ToolCalls, ChatExecutorToolCall{
				ID:        stringFromValue(call["id"]),
				Name:      stringFromValue(function["name"]),
				Arguments: stringFromValue(function["arguments"]),
			})
		}
	}
	return result
}

func serverChatMessagesText(req ChatExecutorRequest) string {
	parts := make([]string, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		parts = append(parts, req.System)
	}
	for _, message := range req.Messages {
		if message.Content != "" {
			parts = append(parts, message.Content)
		}
	}
	return strings.Join(parts, "\n")
}
