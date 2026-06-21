package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/WindyPear-Team/flai/internal/config"
	"github.com/WindyPear-Team/flai/internal/model"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type AuthService struct {
	jwtSecret    []byte
	oidcMu       sync.Mutex
	oidcCacheKey string
	oidcClient   *oidcRuntimeClient
}

type oidcRuntimeConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type oidcRuntimeClient struct {
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier
}

type oidcClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func NewAuthService() (*AuthService, error) {
	jwtSecret := []byte(config.JWTSecret)

	return &AuthService{
		jwtSecret: jwtSecret,
	}, nil
}

func (s *AuthService) GetAuthURL(ctx context.Context, state string) (string, error) {
	client, err := s.loadOIDCClient(ctx)
	if err != nil {
		return "", err
	}
	return client.oauth2Config.AuthCodeURL(state), nil
}

func (s *AuthService) HandleCallback(ctx context.Context, code string, referralCode string) (*model.User, string, error) {
	if code == "" {
		return nil, "", errors.New("missing authorization code")
	}
	claims, err := s.verifyOIDCClaims(ctx, code)
	if err != nil {
		return nil, "", err
	}
	isBootstrapAdmin, err := shouldBootstrapAdmin(claims.Email, claims.Subject)
	if err != nil {
		return nil, "", err
	}
	email := firstNonEmpty(claims.Email, claims.Subject)

	// Find or create user
	var user model.User
	foundUser, err := findUserByOIDCSubjectOrEmail(&user, claims.Subject, email)
	if err != nil {
		return nil, "", err
	}
	if foundUser {
		if user.OIDCSub == nil || *user.OIDCSub != claims.Subject {
			oidcSub := claims.Subject
			if err := model.DB.Model(&user).Update("oidc_sub", oidcSub).Error; err != nil {
				return nil, "", err
			}
			user.OIDCSub = &oidcSub
		}
		if user.AvatarURL == "" && strings.TrimSpace(claims.Picture) != "" {
			avatarURL := strings.TrimSpace(claims.Picture)
			if err := model.DB.Model(&user).Update("avatar_url", avatarURL).Error; err != nil {
				return nil, "", err
			}
			user.AvatarURL = avatarURL
		}
	} else {
		defaultGroup, err := model.EnsureDefaultGroup()
		if err != nil {
			return nil, "", err
		}
		apiKeyRaw, _, err := GenerateAPIKey()
		if err != nil {
			return nil, "", err
		}
		username, err := uniqueUsername(firstNonEmpty(claims.Name, emailUsername(claims.Email), claims.Subject))
		if err != nil {
			return nil, "", err
		}

		userReferralCode, err := model.NewUniqueReferralCode()
		if err != nil {
			return nil, "", err
		}
		referrerID, err := referrerIDFromReferralCode(referralCode)
		if err != nil {
			return nil, "", err
		}

		// Create new user
		user = model.User{
			Username:      username,
			Email:         email,
			OIDCSub:       &claims.Subject,
			EmailVerified: strings.TrimSpace(claims.Email) != "",
			AvatarURL:     strings.TrimSpace(claims.Picture),
			APIKey:        apiKeyRaw,
			GroupID:       defaultGroup.ID,
			ReferralCode:  &userReferralCode,
			ReferrerID:    referrerID,
			IsAdmin:       isBootstrapAdmin,
		}
		if err := model.DB.Create(&user).Error; err != nil {
			return nil, "", err
		}
		if err := model.DB.Where(&model.UserGroupMembership{UserID: user.ID, GroupID: defaultGroup.ID}).
			FirstOrCreate(&model.UserGroupMembership{UserID: user.ID, GroupID: defaultGroup.ID}).Error; err != nil {
			return nil, "", err
		}
	}
	if isBootstrapAdmin && !user.IsAdmin {
		if err := model.DB.Model(&user).Update("is_admin", true).Error; err != nil {
			return nil, "", err
		}
		user.IsAdmin = true
	}
	if err := ensureUserReferralCode(&user); err != nil {
		return nil, "", err
	}

	tokenString, err := s.issueJWT(&user)
	if err != nil {
		return nil, "", err
	}

	return &user, tokenString, nil
}

func (s *AuthService) verifyOIDCClaims(ctx context.Context, code string) (oidcClaims, error) {
	client, err := s.loadOIDCClient(ctx)
	if err != nil {
		return oidcClaims{}, err
	}

	oauth2Token, err := client.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return oidcClaims{}, fmt.Errorf("failed to exchange token: %v", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return oidcClaims{}, errors.New("no id_token in oauth2 token")
	}

	idToken, err := client.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return oidcClaims{}, fmt.Errorf("failed to verify ID token: %v", err)
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return oidcClaims{}, fmt.Errorf("failed to parse claims: %v", err)
	}
	if claims.Subject == "" {
		return oidcClaims{}, errors.New("OIDC subject is missing")
	}
	return claims, nil
}

func (s *AuthService) issueJWT(user *model.User) (string, error) {
	if user == nil || user.ID == 0 {
		return "", errors.New("user is required")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":       user.ID,
		"is_admin": user.IsAdmin,
		"exp":      time.Now().Add(time.Hour * 24 * 7).Unix(),
	})
	return token.SignedString(s.jwtSecret)
}

func referrerIDFromReferralCode(code string) (*uint, error) {
	if !settingBool("referral_enabled", false) {
		return nil, nil
	}
	code = model.NormalizeReferralCode(code)
	if code == "" {
		return nil, nil
	}
	var referrer model.User
	err := model.DB.Where("referral_code = ?", code).First(&referrer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &referrer.ID, nil
}

func ensureUserReferralCode(user *model.User) error {
	if user == nil {
		return nil
	}
	if user.ReferralCode != nil && strings.TrimSpace(*user.ReferralCode) != "" {
		return nil
	}
	code, err := model.NewUniqueReferralCode()
	if err != nil {
		return err
	}
	if err := model.DB.Model(user).Update("referral_code", code).Error; err != nil {
		return err
	}
	user.ReferralCode = &code
	return nil
}

func settingBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(model.GetSystemSetting(key, strconv.FormatBool(fallback))))
	switch value {
	case "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return fallback
	}
}

func (s *AuthService) loadOIDCClient(ctx context.Context) (*oidcRuntimeClient, error) {
	if !settingBool("oidc_enabled", false) {
		return nil, errors.New("OIDC is disabled")
	}
	cfg := currentOIDCConfig()
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("OIDC not configured")
	}

	cacheKey := cfg.cacheKey()
	s.oidcMu.Lock()
	if s.oidcClient != nil && s.oidcCacheKey == cacheKey {
		client := s.oidcClient
		s.oidcMu.Unlock()
		return client, nil
	}
	s.oidcMu.Unlock()

	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		if config.IsDevelopmentLike(config.Environment) {
			log.Printf("OIDC provider initialization failed; dashboard login is disabled until configuration or provider availability is fixed: %v", err)
		}
		return nil, fmt.Errorf("initialize OIDC provider %q: %w", cfg.Issuer, err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	oauthConfig := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	client := &oidcRuntimeClient{
		oauth2Config: oauthConfig,
		verifier:     verifier,
	}

	s.oidcMu.Lock()
	s.oidcCacheKey = cacheKey
	s.oidcClient = client
	s.oidcMu.Unlock()

	return client, nil
}

func currentOIDCConfig() oidcRuntimeConfig {
	return oidcRuntimeConfig{
		Issuer:       model.GetSystemSetting("oidc_issuer", config.OIDCIssuer),
		ClientID:     model.GetSystemSetting("oidc_client_id", config.OIDCClientID),
		ClientSecret: model.GetSystemSetting("oidc_client_secret", config.OIDCSecret),
		RedirectURL:  model.GetSystemSetting("oidc_redirect_url", config.OIDCRedirect),
	}
}

func (cfg oidcRuntimeConfig) cacheKey() string {
	return strings.Join([]string{cfg.Issuer, cfg.ClientID, cfg.ClientSecret, cfg.RedirectURL}, "\x00")
}

func findUserByOIDCSubjectOrEmail(user *model.User, oidcSub, email string) (bool, error) {
	result := model.DB.Where("oidc_sub = ?", oidcSub).First(user)
	if result.Error == nil {
		return true, nil
	}
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, result.Error
	}

	email = strings.TrimSpace(email)
	if email == "" {
		return false, nil
	}
	result = model.DB.Where("email = ?", email).First(user)
	if result.Error == nil {
		return true, nil
	}
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, result.Error
	}
	return false, nil
}

func (s *AuthService) VerifyJWT(tokenString string) (uint, bool, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return 0, false, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, false, errors.New("invalid claims")
	}

	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return 0, false, errors.New("missing or invalid expiration")
	}

	userID, err := claimUint(claims, "id")
	if err != nil {
		return 0, false, err
	}
	isAdmin, err := claimBool(claims, "is_admin")
	if err != nil {
		return 0, false, err
	}

	return userID, isAdmin, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func uniqueUsername(raw string) (string, error) {
	base := truncateUsername(strings.TrimSpace(raw))
	if base == "" {
		base = "user"
	}

	for i := 0; i < 1000; i++ {
		candidate := base
		if i > 0 {
			suffix := fmt.Sprintf("-%d", i+1)
			candidate = truncateUsernameForSuffix(base, suffix) + suffix
		}

		var count int64
		if err := model.DB.Model(&model.User{}).Where("username = ?", candidate).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
	}

	return "", errors.New("failed to generate unique username")
}

func emailUsername(email string) string {
	email = strings.TrimSpace(email)
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	return email
}

func truncateUsername(value string) string {
	runes := []rune(value)
	if len(runes) <= 100 {
		return string(runes)
	}
	return string(runes[:100])
}

func truncateUsernameForSuffix(value, suffix string) string {
	limit := 100 - len([]rune(suffix))
	if limit < 1 {
		limit = 1
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func shouldBootstrapAdmin(email, oidcSub string) (bool, error) {
	if config.IsBootstrapAdmin(email, oidcSub) {
		return true, nil
	}

	hasAdmin, err := hasAdminUser()
	if err != nil {
		return false, err
	}
	return !hasAdmin, nil
}

func EnsureFirstAdmin(user *model.User) error {
	if user == nil || user.IsAdmin {
		return nil
	}

	hasAdmin, err := hasAdminUser()
	if err != nil {
		return err
	}
	if hasAdmin {
		return nil
	}

	if err := model.DB.Model(user).Update("is_admin", true).Error; err != nil {
		return err
	}
	user.IsAdmin = true
	return nil
}

func hasAdminUser() (bool, error) {
	var adminCount int64
	if err := model.DB.Model(&model.User{}).Where("is_admin = ?", true).Count(&adminCount).Error; err != nil {
		return false, err
	}
	return adminCount > 0, nil
}

func claimUint(claims jwt.MapClaims, key string) (uint, error) {
	switch value := claims[key].(type) {
	case float64:
		if value <= 0 || math.Trunc(value) != value {
			return 0, fmt.Errorf("invalid %s claim", key)
		}
		return uint(value), nil
	case string:
		parsed, err := strconv.ParseUint(value, 10, 0)
		if err != nil || parsed == 0 {
			return 0, fmt.Errorf("invalid %s claim", key)
		}
		return uint(parsed), nil
	default:
		return 0, fmt.Errorf("missing %s claim", key)
	}
}

func claimBool(claims jwt.MapClaims, key string) (bool, error) {
	switch value := claims[key].(type) {
	case bool:
		return value, nil
	case string:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("invalid %s claim", key)
		}
		return parsed, nil
	default:
		return false, fmt.Errorf("missing %s claim", key)
	}
}
