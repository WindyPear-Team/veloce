package channel

import (
	"context"
	"encoding/json"
)

type telegramProvider struct{}

func (telegramProvider) Name() string {
	return providerTelegram
}

func (telegramProvider) ExtractWebhookSummary(body []byte) webhookSummary {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return webhookSummary{}
	}
	message := objectAt(payload, "message")
	if len(message) == 0 {
		message = objectAt(payload, "edited_message")
	}
	chat := objectAt(message, "chat")
	from := objectAt(message, "from")
	displayName := joinedNameParts(from["first_name"], from["last_name"])
	if displayName == "" {
		displayName = firstStringValue(from["username"], chat["title"], chat["username"])
	}
	return webhookSummary{
		ExternalChatID:    stringID(chat["id"]),
		ExternalUserID:    stringID(from["id"]),
		ExternalUserName:  displayName,
		ExternalMessageID: stringID(message["message_id"]),
		Content:           stringValue(message["text"]),
	}
}

func (telegramProvider) SendReply(ctx context.Context, integration MessageChannelIntegration, inbound MessageChannelMessage, content string) error {
	config := providerConfig(integration)
	baseURL := configString(config, "base_url", "api_base_url")
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	payload := map[string]interface{}{
		"chat_id": inbound.ExternalChatID,
		"text":    content,
	}
	if parseMode := configString(config, "parse_mode"); parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	return postProviderJSON(ctx, joinEndpoint(baseURL, "/bot"+integration.BotToken+"/sendMessage"), "", payload)
}
