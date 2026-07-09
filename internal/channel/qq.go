package channel

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type qqProvider struct{}

const qqMarkdownChunkLimit = 1900

var (
	qqMarkdownImagePattern     = regexp.MustCompile(`!\[[^\]]*]\((https?://[^)\s]+)\)`)
	qqBareImageURLPattern      = regexp.MustCompile(`https?://[^\s<>()"]+\.(?i:png|jpe?g|gif|webp)(?:\?[^\s<>()"]*)?`)
	qqMarkdownImageSizePattern = regexp.MustCompile(`#\d+px\s+#\d+px`)
)

func (qqProvider) Name() string {
	return providerQQ
}

func (qqProvider) ExtractWebhookSummary(body []byte) webhookSummary {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return webhookSummary{}
	}
	data := objectAt(payload, "d")
	if len(data) == 0 {
		data = payload
	}
	author := objectAt(data, "author")
	member := objectAt(data, "member")
	groupID := firstStringID(data["group_openid"], data["group_id"])
	userOpenID := firstStringID(data["user_openid"], data["openid"], author["user_openid"], author["openid"], member["user_openid"], member["openid"])
	userID := firstStringID(author["id"], data["author_id"], data["user_id"], userOpenID)
	channelID := firstStringID(data["channel_id"], data["channel_openid"])
	guildID := stringID(data["guild_id"])
	dmsID := firstStringID(data["dms_openid"], data["dms_id"])

	externalChatID := ""
	switch {
	case groupID != "":
		externalChatID = "group:" + groupID
	case dmsID != "":
		externalChatID = "dms:" + dmsID
	case userOpenID != "":
		externalChatID = "user:" + userOpenID
	case channelID != "":
		externalChatID = "channel:" + channelID
	case guildID != "":
		externalChatID = "guild:" + guildID
	}
	content := stringValue(data["content"])
	if content == "" {
		content = stringValue(data["text"])
	}
	return webhookSummary{
		ExternalChatID:    externalChatID,
		ExternalUserID:    userID,
		ExternalUserName:  firstStringValue(member["nick"], member["name"], author["username"], author["nick"], author["name"], data["username"], data["nick"]),
		ExternalMessageID: stringID(data["id"]),
		Content:           strings.TrimSpace(content),
	}
}

func (qqProvider) SendReply(ctx context.Context, integration MessageChannelIntegration, inbound MessageChannelMessage, content string) error {
	config := providerConfig(integration)
	baseURL := configString(config, "base_url", "api_base_url", "endpoint")
	if baseURL == "" {
		baseURL = "https://api.sgroup.qq.com"
	}
	accessToken, err := qqAppAccessToken(ctx, config)
	if err != nil {
		return err
	}
	authorization := qqAccessTokenAuthorization(accessToken)
	targetType, targetID := splitTarget(inbound.ExternalChatID, "group")
	var endpoint string
	switch targetType {
	case "channel":
		endpoint = joinEndpoint(baseURL, "/channels/"+targetID+"/messages")
	case "dms":
		endpoint = joinEndpoint(baseURL, "/dms/"+targetID+"/messages")
	case "user", "private", "c2c":
		endpoint = joinEndpoint(baseURL, "/v2/users/"+targetID+"/messages")
	case "group":
		endpoint = joinEndpoint(baseURL, "/v2/groups/"+targetID+"/messages")
	default:
		endpoint = joinEndpoint(baseURL, "/v2/groups/"+targetID+"/messages")
	}
	if endpoint == "" {
		return errors.New("invalid qq api endpoint")
	}
	messageType := qqMessageType(config)
	if messageType == 2 {
		return sendQQMarkdownMessage(ctx, endpoint, authorization, inbound, content, config)
	}
	payload := qqBaseMessagePayload(inbound, config, messageType, 0)
	if strings.TrimSpace(content) != "" {
		payload["content"] = content
	}
	return postProviderJSON(ctx, endpoint, authorization, payload)
}

func sendQQMarkdownMessage(ctx context.Context, endpoint string, authorization string, inbound MessageChannelMessage, content string, config map[string]interface{}) error {
	prepared := prepareQQMarkdownContent(ctx, content)
	if strings.TrimSpace(prepared) == "" {
		return nil
	}
	chunks := chunkQQText(prepared, qqMarkdownChunkLimit)
	for index, chunk := range chunks {
		payload := qqBaseMessagePayload(inbound, config, 2, index)
		payload["markdown"] = map[string]interface{}{"content": chunk}
		if err := postProviderJSON(ctx, endpoint, authorization, payload); err != nil {
			return err
		}
	}
	return nil
}

func qqBaseMessagePayload(inbound MessageChannelMessage, config map[string]interface{}, messageType int, offset int) map[string]interface{} {
	payload := map[string]interface{}{
		"msg_type": messageType,
	}
	if strings.TrimSpace(inbound.ExternalMsgID) != "" {
		payload["msg_id"] = inbound.ExternalMsgID
	}
	if seq := qqMessageSeq(config) + offset; seq > 0 {
		payload["msg_seq"] = seq
	}
	return payload
}

func qqMessageType(config map[string]interface{}) int {
	value := configString(config, "msg_type", "message_type")
	if value == "" {
		return 2
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func qqMessageSeq(config map[string]interface{}) int {
	value := configString(config, "msg_seq", "message_seq")
	if value == "" {
		return 1
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 1
	}
	return parsed
}

func prepareQQMarkdownContent(ctx context.Context, content string) string {
	result := strings.TrimSpace(content)
	if result == "" {
		return ""
	}
	existingImageURLs := map[string]struct{}{}
	result = qqMarkdownImagePattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := qqMarkdownImagePattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		imageURL := strings.TrimSpace(parts[1])
		existingImageURLs[imageURL] = struct{}{}
		if qqMarkdownImageSizePattern.MatchString(match) {
			return match
		}
		size := fetchImageSize(ctx, imageURL)
		return formatQQMarkdownImage(imageURL, size)
	})

	appendImages := []string{}
	result = qqBareImageURLPattern.ReplaceAllStringFunc(result, func(match string) string {
		imageURL := strings.TrimRight(strings.TrimSpace(match), ".,;)")
		if _, exists := existingImageURLs[imageURL]; exists {
			return ""
		}
		appendImages = append(appendImages, formatQQMarkdownImage(imageURL, fetchImageSize(ctx, imageURL)))
		return ""
	})
	if len(appendImages) > 0 {
		result = strings.TrimSpace(result)
		if result != "" {
			result += "\n\n"
		}
		result += strings.Join(appendImages, "\n")
	}
	return strings.TrimSpace(result)
}

type qqImageSize struct {
	Width  int
	Height int
}

func fetchImageSize(ctx context.Context, imageURL string) *qqImageSize {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil
	}
	resp, err := providerHTTPClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil
	}
	config, _, err := image.DecodeConfig(resp.Body)
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return nil
	}
	return &qqImageSize{Width: config.Width, Height: config.Height}
}

func formatQQMarkdownImage(imageURL string, size *qqImageSize) string {
	if size == nil {
		return "![img #500px #500px](" + imageURL + ")"
	}
	return "![img #" + strconv.Itoa(size.Width) + "px #" + strconv.Itoa(size.Height) + "px](" + imageURL + ")"
}

func chunkQQText(value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return []string{value}
	}
	chunks := []string{}
	var current strings.Builder
	count := 0
	for _, r := range value {
		if count >= limit && (r == '\n' || r == ' ') {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
			count = 0
			continue
		}
		if count >= limit {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
			count = 0
		}
		current.WriteRune(r)
		count++
	}
	if strings.TrimSpace(current.String()) != "" {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}
	return chunks
}

func firstStringID(values ...interface{}) string {
	for _, value := range values {
		if id := stringID(value); id != "" {
			return id
		}
	}
	return ""
}

func qqAuthorization(config map[string]interface{}, botToken string) string {
	authorization := configString(config, "authorization", "auth_header")
	if authorization != "" {
		return authorization
	}
	botID := configString(config, "bot_id", "robot_id", "app_id", "appid")
	botSecret := configString(config, "bot_secret", "secret", "client_secret")
	if botID != "" && botSecret != "" {
		return "QQBot " + botID + "." + botSecret
	}
	token := strings.TrimSpace(botToken)
	if token != "" {
		return "QQBot " + token
	}
	return ""
}

func splitTarget(value string, fallbackType string) (string, string) {
	value = strings.TrimSpace(value)
	if left, right, ok := strings.Cut(value, ":"); ok {
		return strings.ToLower(strings.TrimSpace(left)), strings.TrimSpace(right)
	}
	return fallbackType, value
}
