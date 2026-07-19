package cache

import "testing"

func TestResolveRedisConfigUsesSettings(t *testing.T) {
	settings := map[string]string{
		redisSettingEnabled:  "true",
		redisSettingAddress:  "cache.internal:6380",
		redisSettingUsername: "service",
		redisSettingPassword: "secret",
		redisSettingDatabase: "2",
		redisSettingTLS:      "true",
	}
	config, err := resolveRedisConfig(settingValue(settings), func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("resolve Redis config: %v", err)
	}
	if !config.Enabled || config.Address != "cache.internal:6380" || config.Username != "service" || config.Password != "secret" || config.Database != 2 || !config.TLS {
		t.Fatalf("unexpected Redis config: %#v", config)
	}
}

func TestResolveRedisConfigEnvironmentOverridesSettings(t *testing.T) {
	settings := map[string]string{
		redisSettingEnabled:  "false",
		redisSettingAddress:  "settings:6379",
		redisSettingUsername: "settings-user",
		redisSettingPassword: "settings-password",
		redisSettingDatabase: "1",
		redisSettingTLS:      "false",
	}
	environment := map[string]string{
		"REDIS_ENABLED":  "true",
		"REDIS_ADDR":     "environment:6381",
		"REDIS_USERNAME": "environment-user",
		"REDIS_PASSWORD": "environment-password",
		"REDIS_DB":       "3",
		"REDIS_TLS":      "true",
	}
	config, err := resolveRedisConfig(settingValue(settings), environmentValue(environment))
	if err != nil {
		t.Fatalf("resolve Redis config: %v", err)
	}
	if !config.Enabled || config.Address != "environment:6381" || config.Username != "environment-user" || config.Password != "environment-password" || config.Database != 3 || !config.TLS {
		t.Fatalf("unexpected Redis config: %#v", config)
	}
}

func TestResolveRedisConfigURLOverridesIndividualEnvironmentVariables(t *testing.T) {
	environment := map[string]string{
		"REDIS_URL":  "rediss://url-user:url-password@cache.example:6380/4",
		"REDIS_ADDR": "ignored:6379",
		"REDIS_DB":   "2",
	}
	config, err := resolveRedisConfig(settingValue(nil), environmentValue(environment))
	if err != nil {
		t.Fatalf("resolve Redis config: %v", err)
	}
	if config.Address != "cache.example:6380" || config.Username != "url-user" || config.Password != "url-password" || config.Database != 4 || !config.TLS {
		t.Fatalf("unexpected Redis URL config: %#v", config)
	}
}

func TestResolveRedisConfigRejectsInvalidEnabledConfiguration(t *testing.T) {
	_, err := resolveRedisConfig(settingValue(map[string]string{redisSettingEnabled: "true", redisSettingAddress: ""}), func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("expected empty enabled Redis address to be rejected")
	}
}

func settingValue(values map[string]string) func(string, string) string {
	return func(key, fallback string) string {
		if value, ok := values[key]; ok {
			return value
		}
		return fallback
	}
}

func environmentValue(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
