package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var providerHTTPClient = &http.Client{Timeout: 15 * time.Second}

type webhookSummary struct {
	ExternalChatID    string
	ExternalUserID    string
	ExternalUserName  string
	ExternalMessageID string
	Content           string
}

type providerAdapter interface {
	Name() string
	ExtractWebhookSummary(body []byte) webhookSummary
	SendReply(ctx context.Context, integration MessageChannelIntegration, inbound MessageChannelMessage, content string) error
}

var providerAdapters = map[string]providerAdapter{
	providerTelegram:       telegramProvider{},
	providerDiscord:        discordProvider{},
	providerQQ:             qqProvider{},
	providerOneBot:         oneBotProvider{},
	providerWeixin:         weixinProvider{},
	providerTencentChannel: tencentChannelProvider{},
}

func supportedProviders() []string {
	return []string{providerTelegram, providerDiscord, providerQQ, providerOneBot, providerWeixin, providerTencentChannel}
}

func providerFor(provider string) (providerAdapter, bool) {
	adapter, ok := providerAdapters[normalizeProvider(provider)]
	return adapter, ok
}

func sendProviderReply(ctx context.Context, integration MessageChannelIntegration, externalChatID string, content string) error {
	return sendProviderReplyForMessage(ctx, integration, MessageChannelMessage{ExternalChatID: externalChatID}, content)
}

func sendProviderReplyForMessage(ctx context.Context, integration MessageChannelIntegration, inbound MessageChannelMessage, content string) error {
	externalChatID := strings.TrimSpace(inbound.ExternalChatID)
	if strings.TrimSpace(externalChatID) == "" {
		return errors.New("external chat id is empty")
	}
	adapter, ok := providerFor(integration.Provider)
	if !ok {
		return errors.New("unsupported provider")
	}
	return adapter.SendReply(ctx, integration, inbound, content)
}

func postProviderJSON(ctx context.Context, endpoint string, authorization string, payload map[string]interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := providerHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(respBody))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return errors.New("provider returned status " + strconv.Itoa(resp.StatusCode) + ": " + message)
	}
	return nil
}

func providerConfig(integration MessageChannelIntegration) map[string]interface{} {
	options := decodeAdvancedOptions(integration.AdvancedOptions)
	raw := strings.TrimSpace(options.CustomProviderConfigJSON)
	if raw == "" {
		return map[string]interface{}{}
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return map[string]interface{}{}
	}
	return config
}

func configString(config map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(config[key]); value != "" {
			return value
		}
	}
	return ""
}

func joinEndpoint(base string, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

func objectAt(data map[string]interface{}, key string) map[string]interface{} {
	value, _ := data[key].(map[string]interface{})
	if value == nil {
		return map[string]interface{}{}
	}
	return value
}

func stringID(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func stringValue(value interface{}) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func firstStringValue(values ...interface{}) string {
	for _, value := range values {
		if text := stringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func joinedNameParts(values ...interface{}) string {
	parts := []string{}
	for _, value := range values {
		if text := stringValue(value); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
