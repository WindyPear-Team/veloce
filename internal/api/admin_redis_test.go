package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRedisSettingsPersistWithoutExposingPassword(t *testing.T) {
	previousDB := model.DB
	t.Cleanup(func() { model.DB = previousDB })
	database, err := gorm.Open(sqlite.Open("file:redis-settings-api-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.SystemSetting{}); err != nil {
		t.Fatal(err)
	}
	model.DB = database

	body, err := json.Marshal(map[string]any{
		"redis_enabled":     true,
		"redis_address":     "cache.internal:6380",
		"redis_username":    "service",
		"redis_password":    "secret",
		"redis_database":    "2",
		"redis_tls_enabled": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPut, "/settings", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	(&SystemAPI{}).UpdateSettings(context)
	if response.Code != http.StatusOK {
		t.Fatalf("update Redis settings: status=%d body=%s", response.Code, response.Body.String())
	}
	if got := model.GetSystemSetting("redis_password", ""); got != "secret" {
		t.Fatalf("stored Redis password = %q, want secret", got)
	}

	settings := currentAdminSystemSettings()
	if !settings.RedisEnabled || settings.RedisAddress != "cache.internal:6380" || settings.RedisUsername != "service" || settings.RedisDatabase != "2" || !settings.RedisTLSEnabled || !settings.RedisPasswordSet {
		t.Fatalf("unexpected Redis settings: %+v", settings)
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("secret")) {
		t.Fatalf("Redis password was exposed in settings response: %s", encoded)
	}
}
