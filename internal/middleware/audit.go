package middleware

import (
	"net/url"
	"strings"
	"time"

	"github.com/WindyPear-Team/flai/internal/model"
	"github.com/WindyPear-Team/flai/internal/service"
	"github.com/gin-gonic/gin"
)

func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !service.IsAuditablePath(path) {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		if shouldSkipAuditLog(path) {
			return
		}

		var userID *uint
		if value, exists := c.Get("user"); exists {
			if user, ok := value.(*model.User); ok && user != nil && user.ID != 0 {
				id := user.ID
				userID = &id
			}
		}
		var apiKeyID *uint
		if value, exists := c.Get("api_key"); exists {
			if apiKey, ok := value.(*model.APIKey); ok && apiKey != nil && apiKey.ID != 0 {
				id := apiKey.ID
				apiKeyID = &id
			}
		}

		statusCode := c.Writer.Status()
		service.RecordAuditLog(service.AuditLogInput{
			LogType:    service.AuditLogTypeForRequest(c.Request.Method, path),
			Action:     service.AuditActionForRequest(c.Request.Method, path, statusCode),
			Resource:   auditResource(c.Request.Method, path),
			UserID:     userID,
			APIKeyID:   apiKeyID,
			Method:     c.Request.Method,
			Path:       path,
			Query:      redactedRawQuery(c.Request.URL.RawQuery),
			StatusCode: statusCode,
			IPAddress:  c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			DurationMs: time.Since(start).Milliseconds(),
		})
	}
}

func shouldSkipAuditLog(path string) bool {
	return path == "/api/audit-logs"
}

func redactedRawQuery(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return ""
	}
	for key := range values {
		switch strings.ToLower(key) {
		case "key", "token", "access_token", "refresh_token", "code", "client_secret", "password":
			values.Set(key, "[redacted]")
		}
	}
	return values.Encode()
}

func auditResource(method, path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return strings.ToUpper(method)
	}
	if len(parts) >= 2 && parts[0] == "api" {
		return strings.ToUpper(method) + " /" + strings.Join(parts[:2], "/")
	}
	return strings.ToUpper(method) + " /" + parts[0]
}
