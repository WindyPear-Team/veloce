package service

import (
	"strings"
	"unicode"
)

const (
	SensitiveFilterScopeRequest         = "request"
	SensitiveFilterScopeRequestResponse = "request_response"
)

type SensitiveWordMatch struct {
	Word string
}

type SensitiveFilterHooks struct {
	Enabled    func() bool
	Scope      func() string
	MatchWords func(text string) (SensitiveWordMatch, bool)
	Words      func() []string
}

var sensitiveFilterHooks SensitiveFilterHooks

func RegisterSensitiveFilterHooks(hooks SensitiveFilterHooks) {
	sensitiveFilterHooks = hooks
}

func SensitiveFilterEnabled() bool {
	if sensitiveFilterHooks.Enabled != nil {
		return sensitiveFilterHooks.Enabled()
	}
	return false
}

func SensitiveFilterScope() string {
	if sensitiveFilterHooks.Scope != nil {
		return sensitiveFilterHooks.Scope()
	}
	return SensitiveFilterScopeRequest
}

func MatchSensitiveWords(text string) (SensitiveWordMatch, bool) {
	if sensitiveFilterHooks.MatchWords != nil {
		return sensitiveFilterHooks.MatchWords(text)
	}
	return SensitiveWordMatch{}, false
}

func SensitiveWords() []string {
	if sensitiveFilterHooks.Words != nil {
		return sensitiveFilterHooks.Words()
	}
	return nil
}

func normalizedRequestText(request normalizedAIRequest) string {
	parts := make([]string, 0, len(request.Messages)+1)
	if strings.TrimSpace(request.System) != "" {
		parts = append(parts, request.System)
	}
	for _, message := range request.Messages {
		if strings.TrimSpace(message.Content) != "" {
			parts = append(parts, message.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func parseDelimitedList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '\n' || r == '\r' || r == ';' || r == '；' || unicode.IsSpace(r) && r != ' '
	})
	items := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		item := strings.TrimSpace(field)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}
	return items
}
