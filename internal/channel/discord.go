package channel

import (
	"context"
	"encoding/json"
)

type discordProvider struct{}

func (discordProvider) Name() string {
	return providerDiscord
}

func (discordProvider) ExtractWebhookSummary(body []byte) webhookSummary {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return webhookSummary{}
	}
	author := objectAt(payload, "author")
	return webhookSummary{
		ExternalChatID:    stringID(payload["channel_id"]),
		ExternalUserID:    stringID(author["id"]),
		ExternalUserName:  firstStringValue(author["global_name"], author["display_name"], author["username"]),
		ExternalMessageID: stringID(payload["id"]),
		Content:           stringValue(payload["content"]),
	}
}

func (discordProvider) SendReply(ctx context.Context, integration MessageChannelIntegration, inbound MessageChannelMessage, content string) error {
	config := providerConfig(integration)
	baseURL := configString(config, "base_url", "api_base_url")
	if baseURL == "" {
		baseURL = "https://discord.com/api/v10"
	}
	return postProviderJSON(ctx, joinEndpoint(baseURL, "/channels/"+inbound.ExternalChatID+"/messages"), "Bot "+integration.BotToken, map[string]interface{}{
		"content": content,
	})
}
