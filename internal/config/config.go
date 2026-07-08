package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

var (
	Environment  string
	Port         string
	DBPath       string
	DataPath     string
	JWTSecret    string
	OIDCIssuer   string
	OIDCClientID string
	OIDCSecret   string
	OIDCRedirect string

	BootstrapAdminEmails   string
	BootstrapAdminOIDCSubs string
)

func Init() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	Environment = getEnv("APP_ENV", "development")
	Port = getEnv("PORT", "8080")
	DBPath = getEnv("DB_PATH", "flai.db")
	DataPath = getEnv("DATA_PATH", "data")
	JWTSecret = getEnv("JWT_SECRET", "change-me-please")
	if requiresSecureSecrets(Environment) && JWTSecret == "change-me-please" {
		log.Fatal("JWT_SECRET must be set to a secure value outside development")
	}
	OIDCIssuer = os.Getenv("OIDC_ISSUER")
	OIDCClientID = os.Getenv("OIDC_CLIENT_ID")
	OIDCSecret = os.Getenv("OIDC_CLIENT_SECRET")
	OIDCRedirect = os.Getenv("OIDC_REDIRECT_URL")
	BootstrapAdminEmails = os.Getenv("BOOTSTRAP_ADMIN_EMAILS")
	BootstrapAdminOIDCSubs = os.Getenv("BOOTSTRAP_ADMIN_OIDC_SUBS")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func requiresSecureSecrets(env string) bool {
	return !IsDevelopmentLike(env)
}

func IsDevelopmentLike(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "", "development", "dev", "local", "test":
		return true
	default:
		return false
	}
}

func IsBootstrapAdmin(email, oidcSub string) bool {
	return csvContains(BootstrapAdminEmails, email, false) ||
		csvContains(BootstrapAdminOIDCSubs, oidcSub, true)
}

func csvContains(raw, value string, caseSensitive bool) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if caseSensitive {
			if item == value {
				return true
			}
			continue
		}
		if strings.EqualFold(item, value) {
			return true
		}
	}

	return false
}
