package model

import (
	"crypto/rand"
	"encoding/base32"
	"log"
	"strings"

	"github.com/WindyPear-Team/flai/internal/config"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	var err error
	DB, err = gorm.Open(sqlite.Open(config.DBPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	hadCachedInputPrice := DB.Migrator().HasColumn(&Model{}, "cached_input_price")
	hadCacheWriteInputPrice := DB.Migrator().HasColumn(&Model{}, "cache_write_input_price")
	hadCacheWrite1hInputPrice := DB.Migrator().HasColumn(&Model{}, "cache_write_1h_input_price")

	// Auto Migrate
	err = DB.AutoMigrate(
		&User{},
		&APIKey{},
		&EmailVerificationCode{},
		&OIDCBindRequest{},
		&WebAuthnChallenge{},
		&PasskeyCredential{},
		&CheckInRecord{},
		&PaymentOrder{},
		&Group{},
		&UserGroupMembership{},
		&ChannelGroupMultiplier{},
		&ModelGroupMultiplier{},
		&ReferralCommissionLog{},
		&UserChannel{},
		&Channel{},
		&Model{},
		&ModelConfig{},
		&StatusMonitor{},
		&StatusCheck{},
		&Announcement{},
		&SystemSetting{},
		&VideoTask{},
		&TokenLog{},
	)
	if err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}
	if !hadCachedInputPrice {
		if err := DB.Model(&Model{}).
			Where("cached_input_price = ? AND input_price > ?", decimal.Zero, decimal.Zero).
			Update("cached_input_price", gorm.Expr("input_price")).Error; err != nil {
			log.Fatalf("failed to initialize cached input prices: %v", err)
		}
	}
	if !hadCacheWriteInputPrice {
		if err := DB.Model(&Model{}).
			Where("cache_write_input_price = ? AND input_price > ?", decimal.Zero, decimal.Zero).
			Update("cache_write_input_price", gorm.Expr("input_price")).Error; err != nil {
			log.Fatalf("failed to initialize cache write input prices: %v", err)
		}
	}
	if !hadCacheWrite1hInputPrice {
		if err := DB.Model(&Model{}).
			Where("cache_write_1h_input_price = ? AND input_price > ?", decimal.Zero, decimal.Zero).
			Update("cache_write_1h_input_price", gorm.Expr("input_price")).Error; err != nil {
			log.Fatalf("failed to initialize 1h cache write input prices: %v", err)
		}
	}

	// Initial data
	initData()
}

func initData() {
	if _, err := EnsureDefaultGroup(); err != nil {
		log.Fatalf("failed to initialize default group: %v", err)
	}
	if err := EnsureDefaultUserGroupMemberships(); err != nil {
		log.Fatalf("failed to initialize user group memberships: %v", err)
	}
	if err := EnsureUserReferralCodes(); err != nil {
		log.Fatalf("failed to initialize user referral codes: %v", err)
	}
	if err := NormalizeEmptyOIDCSubjects(); err != nil {
		log.Fatalf("failed to normalize empty oidc subjects: %v", err)
	}
	if _, err := EnsureDefaultUserChannel(); err != nil {
		log.Fatalf("failed to initialize default user channel: %v", err)
	}
	if err := EnsureGlobalModels(); err != nil {
		log.Fatalf("failed to initialize global models: %v", err)
	}
	if err := EnsureDefaultSystemSettings(); err != nil {
		log.Fatalf("failed to initialize system settings: %v", err)
	}
}

func NormalizeEmptyOIDCSubjects() error {
	return DB.Model(&User{}).Where("oidc_sub = ?", "").Update("oidc_sub", nil).Error
}

func EnsureDefaultGroup() (Group, error) {
	group := Group{Name: "user"}
	err := DB.Where(&Group{Name: "user"}).
		Attrs(Group{Multiplier: decimal.NewFromInt(1)}).
		FirstOrCreate(&group).Error
	return group, err
}

func EnsureDefaultUserGroupMemberships() error {
	group, err := EnsureDefaultGroup()
	if err != nil {
		return err
	}

	if err := DB.Model(&User{}).Where("group_id = 0 OR group_id IS NULL").Update("group_id", group.ID).Error; err != nil {
		return err
	}

	var users []User
	if err := DB.Find(&users).Error; err != nil {
		return err
	}
	for _, user := range users {
		groupID := user.GroupID
		if groupID == 0 {
			groupID = group.ID
		}
		membership := UserGroupMembership{UserID: user.ID, GroupID: groupID}
		if err := DB.Where(&UserGroupMembership{UserID: user.ID, GroupID: groupID}).
			FirstOrCreate(&membership).Error; err != nil {
			return err
		}
	}
	return nil
}

func EnsureUserReferralCodes() error {
	var users []User
	if err := DB.Where("referral_code IS NULL OR referral_code = ?", "").Find(&users).Error; err != nil {
		return err
	}
	for _, user := range users {
		code, err := NewUniqueReferralCode()
		if err != nil {
			return err
		}
		if err := DB.Model(&user).Update("referral_code", code).Error; err != nil {
			return err
		}
	}
	return nil
}

func NewUniqueReferralCode() (string, error) {
	for i := 0; i < 50; i++ {
		code, err := NewReferralCode()
		if err != nil {
			return "", err
		}
		var count int64
		if err := DB.Model(&User{}).Where("referral_code = ?", code).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", gorm.ErrDuplicatedKey
}

func NewReferralCode() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:]), nil
}

func NormalizeReferralCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")
	return code
}

func EnsureDefaultUserChannel() (UserChannel, error) {
	userChannel := UserChannel{Name: "default"}
	err := DB.Where(&UserChannel{Name: "default"}).
		Attrs(UserChannel{Description: "Default user-facing channel", Multiplier: decimal.NewFromInt(1), RoutingAlgorithm: "priority", Enabled: true}).
		FirstOrCreate(&userChannel).Error
	if err != nil {
		return userChannel, err
	}

	if err := DB.Model(&UserChannel{}).
		Where("multiplier = ?", decimal.Zero).
		Update("multiplier", decimal.NewFromInt(1)).Error; err != nil {
		return userChannel, err
	}
	if err := DB.Model(&UserChannel{}).
		Where("routing_algorithm = ? OR routing_algorithm IS NULL", "").
		Update("routing_algorithm", "priority").Error; err != nil {
		return userChannel, err
	}

	err = DB.Model(&Channel{}).
		Where("user_channel_id IS NULL").
		Update("user_channel_id", userChannel.ID).Error
	return userChannel, err
}

func EnsureGlobalModels() error {
	if !DB.Migrator().HasTable("model_configs") || !DB.Migrator().HasColumn("model_configs", "model_name") {
		return nil
	}

	type legacyModelConfig struct {
		ID                uint
		ModelID           uint
		ModelName         string
		Provider          string
		ProviderIconURL   string
		UpstreamModelName string
		InputPrice        decimal.Decimal
		OutputPrice       decimal.Decimal
	}

	var configs []legacyModelConfig
	if err := DB.Table("model_configs").
		Select("id, model_id, model_name, provider, provider_icon_url, upstream_model_name, input_price, output_price").
		Find(&configs).Error; err != nil {
		return err
	}

	for _, config := range configs {
		modelName := strings.TrimSpace(config.ModelName)
		if modelName == "" {
			continue
		}

		globalModel := Model{ModelName: modelName}
		if err := DB.Where(&Model{ModelName: modelName}).
			Attrs(Model{
				Provider:        strings.TrimSpace(config.Provider),
				ProviderIconURL: strings.TrimSpace(config.ProviderIconURL),
				Enabled:         true,
			}).
			FirstOrCreate(&globalModel).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{}
		if config.ModelID == 0 {
			updates["model_id"] = globalModel.ID
		}
		if strings.TrimSpace(config.UpstreamModelName) == "" {
			updates["upstream_model_name"] = modelName
		}
		if len(updates) > 0 {
			if err := DB.Table("model_configs").Where("id = ?", config.ID).Updates(updates).Error; err != nil {
				return err
			}
		}

		modelUpdates := map[string]interface{}{}
		if strings.TrimSpace(globalModel.Provider) == "" && strings.TrimSpace(config.Provider) != "" {
			modelUpdates["provider"] = strings.TrimSpace(config.Provider)
		}
		if strings.TrimSpace(globalModel.ProviderIconURL) == "" && strings.TrimSpace(config.ProviderIconURL) != "" {
			modelUpdates["provider_icon_url"] = strings.TrimSpace(config.ProviderIconURL)
		}
		if globalModel.InputPrice.IsZero() && !config.InputPrice.IsZero() {
			modelUpdates["input_price"] = config.InputPrice
		}
		if globalModel.OutputPrice.IsZero() && !config.OutputPrice.IsZero() {
			modelUpdates["output_price"] = config.OutputPrice
		}
		if len(modelUpdates) > 0 {
			if err := DB.Model(&globalModel).Updates(modelUpdates).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func EnsureDefaultSystemSettings() error {
	oidcDefault := "false"
	var oidcUserCount int64
	if err := DB.Model(&User{}).Where("oidc_sub IS NOT NULL AND oidc_sub <> ?", "").Count(&oidcUserCount).Error; err == nil && oidcUserCount > 0 {
		oidcDefault = "true"
	}
	defaults := map[string]string{
		"site_name":                        "flai",
		"base_url":                         "",
		"icon_url":                         "",
		"footer_text":                      "",
		"about_html":                       "",
		"home_iframe_url":                  "",
		"privacy_policy":                   "",
		"terms":                            "",
		"privacy_policy_url":               "",
		"terms_url":                        "",
		"auth_agreement_mode":              "notice",
		"announcement":                     "",
		"top_nav_enabled":                  "false",
		"top_nav_items":                    "",
		"page_layouts":                     "{}",
		"theme_light_background":           "#ffffff",
		"theme_light_foreground":           "#020817",
		"theme_light_card":                 "#ffffff",
		"theme_light_card_foreground":      "#020817",
		"theme_light_primary":              "#0f172a",
		"theme_light_primary_foreground":   "#f8fafc",
		"theme_light_secondary":            "#f1f5f9",
		"theme_light_secondary_foreground": "#0f172a",
		"theme_light_accent":               "#f1f5f9",
		"theme_light_accent_foreground":    "#0f172a",
		"theme_light_muted":                "#f1f5f9",
		"theme_light_muted_foreground":     "#64748b",
		"theme_light_border":               "#e2e8f0",
		"theme_dark_background":            "#020817",
		"theme_dark_foreground":            "#f8fafc",
		"theme_dark_card":                  "#020817",
		"theme_dark_card_foreground":       "#f8fafc",
		"theme_dark_primary":               "#f8fafc",
		"theme_dark_primary_foreground":    "#0f172a",
		"theme_dark_secondary":             "#1e293b",
		"theme_dark_secondary_foreground":  "#f8fafc",
		"theme_dark_accent":                "#1e293b",
		"theme_dark_accent_foreground":     "#f8fafc",
		"theme_dark_muted":                 "#1e293b",
		"theme_dark_muted_foreground":      "#94a3b8",
		"theme_dark_border":                "#1e293b",
		"sidebar_dashboard_enabled":        "true",
		"sidebar_usage_enabled":            "true",
		"sidebar_wallet_enabled":           "true",
		"sidebar_data_board_enabled":       "true",
		"sidebar_api_keys_enabled":         "true",
		"sidebar_chat_enabled":             "true",
		"sidebar_images_enabled":           "true",
		"sidebar_settings_enabled":         "true",
		"sidebar_system_enabled":           "true",
		"sidebar_admin_overview_enabled":   "true",
		"sidebar_channels_enabled":         "true",
		"sidebar_models_enabled":           "true",
		"sidebar_users_enabled":            "true",
		"chat_page_mode":                   "basic",
		"message_channel_enabled":          "false",
		"referral_enabled":                 "false",
		"referral_commission_rate":         "0",
		"group_multiplier_mode":            "min",
		"pricing_endpoint_enabled":         "false",
		"status_monitor_enabled":           "false",
		"checkin_enabled":                  "false",
		"checkin_daily_reward":             "0",
		"checkin_timezone":                 "Asia/Shanghai",
		"checkin_streak_enabled":           "false",
		"checkin_streak_cycle_days":        "7",
		"checkin_streak_rewards":           "{}",
		"checkin_random_enabled":           "false",
		"checkin_random_min":               "0",
		"checkin_random_max":               "0",
		"payment_enabled":                  "false",
		"payment_currency_display_name":    "$",
		"payment_usd_to_rmb_rate":          "7.20",
		"payment_min_recharge_amount":      "1",
		"payment_recharge_presets":         "[\"5\",\"10\",\"20\",\"50\",\"100\"]",
		"payment_methods":                  "[\"alipay\",\"wxpay\"]",
		"payment_gateway_provider":         "yipay",
		"payment_yipay_gateway_url":        "",
		"payment_yipay_pid":                "",
		"payment_yipay_key":                "",
		"payment_yipay_notify_url":         "",
		"payment_yipay_return_url":         "",
		"payment_openpayment_base_url":     "",
		"payment_openpayment_config_url":   "",
		"payment_openpayment_merchant_id":  "",
		"payment_openpayment_key":          "",
		"payment_openpayment_notify_url":   "",
		"payment_openpayment_return_url":   "",
		"rate_limit_enabled":               "true",
		"rate_limit_requests_per_minute":   "60",
		"rate_limit_burst":                 "10",
		"sensitive_filter_enabled":         "false",
		"sensitive_words":                  "",
		"sensitive_filter_scope":           "request",
		"ssrf_protection_enabled":          "true",
		"ssrf_allow_private_networks":      "false",
		"ssrf_allowed_hosts":               "",
		"oidc_enabled":                     oidcDefault,
		"passkey_enabled":                  "false",
		"password_login_enabled":           "true",
		"password_registration_enabled":    "true",
		"password_hcaptcha_enabled":        "false",
		"hcaptcha_site_key":                "",
		"hcaptcha_secret":                  "",
		"email_verification_required":      "false",
		"smtp_host":                        "",
		"smtp_port":                        "587",
		"smtp_username":                    "",
		"smtp_password":                    "",
		"smtp_from":                        "",
		"oidc_issuer":                      "",
		"oidc_client_id":                   "",
		"oidc_client_secret":               "",
		"oidc_redirect_url":                "",
		"oauth_providers":                  "[]",
	}
	for key, value := range defaults {
		setting := SystemSetting{Key: key}
		if err := DB.Where(&SystemSetting{Key: key}).
			Attrs(SystemSetting{Value: value}).
			FirstOrCreate(&setting).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetSystemSetting(key, fallback string) string {
	var setting SystemSetting
	if err := DB.First(&setting, "key = ?", key).Error; err != nil || setting.Value == "" {
		return fallback
	}
	return setting.Value
}

func SetSystemSetting(key, value string) error {
	setting := SystemSetting{Key: key}
	return DB.Where(&SystemSetting{Key: key}).
		Assign(SystemSetting{Value: value}).
		FirstOrCreate(&setting).Error
}
