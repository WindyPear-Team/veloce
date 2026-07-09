package channel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type oneBotProvider struct{}

func (oneBotProvider) Name() string {
	return providerOneBot
}

func (oneBotProvider) ExtractWebhookSummary(body []byte) webhookSummary {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return webhookSummary{}
	}
	messageType := strings.ToLower(stringValue(payload["message_type"]))
	groupID := stringID(payload["group_id"])
	userID := stringID(payload["user_id"])
	externalChatID := ""
	switch messageType {
	case "private":
		externalChatID = "private:" + userID
	case "group":
		externalChatID = "group:" + groupID
	default:
		if groupID != "" {
			externalChatID = "group:" + groupID
		} else if userID != "" {
			externalChatID = "private:" + userID
		}
	}
	content := stringValue(payload["raw_message"])
	if content == "" {
		content = stringValue(payload["message"])
	}
	sender := objectAt(payload, "sender")
	return webhookSummary{
		ExternalChatID:    externalChatID,
		ExternalUserID:    userID,
		ExternalUserName:  firstStringValue(sender["card"], sender["nickname"], sender["name"], payload["sender_name"]),
		ExternalMessageID: stringID(payload["message_id"]),
		Content:           strings.TrimSpace(content),
	}
}

func (oneBotProvider) SendReply(ctx context.Context, integration MessageChannelIntegration, inbound MessageChannelMessage, content string) error {
	config := providerConfig(integration)
	baseURL := configString(config, "base_url", "api_base_url", "endpoint")
	if baseURL == "" && strings.HasPrefix(strings.TrimSpace(integration.BotToken), "http") {
		baseURL = strings.TrimSpace(integration.BotToken)
	}
	if baseURL == "" {
		return errors.New("onebot base_url is required")
	}
	accessToken := configString(config, "access_token", "token")
	if accessToken == "" && !strings.HasPrefix(strings.TrimSpace(integration.BotToken), "http") {
		accessToken = strings.TrimSpace(integration.BotToken)
	}
	authorization := ""
	if accessToken != "" {
		authorization = "Bearer " + accessToken
	}
	targetType, targetID := splitTarget(inbound.ExternalChatID, "group")
	action := "send_group_msg"
	payload := map[string]interface{}{
		"message": content,
	}
	switch targetType {
	case "private", "user":
		action = "send_private_msg"
		payload["user_id"] = targetID
	default:
		payload["group_id"] = targetID
	}
	if customAction := configString(config, "send_action", "action"); customAction != "" {
		action = customAction
	}
	endpoint := joinEndpoint(baseURL, action)
	if endpoint == "" {
		return errors.New("invalid onebot api endpoint")
	}
	return postProviderJSON(ctx, endpoint, authorization, payload)
}
