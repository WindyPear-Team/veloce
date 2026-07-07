package api

import (
	"crypto/md5"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/WindyPear-Team/flai/internal/model"
	"github.com/WindyPear-Team/flai/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	paymentStatusPending = "pending"
	paymentStatusPaid    = "paid"

	paymentProviderYipay       = "yipay"
	paymentProviderOpenPayment = "openpayment"
)

type PaymentAPI struct{}

var paymentFeatureEnabled bool

func EnablePaymentFeature() {
	paymentFeatureEnabled = true
}

func PaymentFeatureEnabled() bool {
	return paymentFeatureEnabled
}

type paymentConfig struct {
	Enabled               bool
	Provider              string
	CurrencyDisplayName   string
	USDToRMBRate          decimal.Decimal
	MinRechargeAmount     decimal.Decimal
	RechargePresets       []string
	Methods               []string
	GatewayURL            string
	PID                   string
	Key                   string
	NotifyURL             string
	ReturnURL             string
	OpenPaymentBaseURL    string
	OpenPaymentConfigURL  string
	OpenPaymentMerchantID string
	OpenPaymentKey        string
	OpenPaymentNotifyURL  string
	OpenPaymentReturnURL  string
}

type paymentConfigResponse struct {
	Enabled             bool     `json:"enabled"`
	CurrencyDisplayName string   `json:"currency_display_name"`
	USDToRMBRate        string   `json:"usd_to_rmb_rate"`
	MinRechargeAmount   string   `json:"min_recharge_amount"`
	RechargePresets     []string `json:"recharge_presets"`
	Methods             []string `json:"methods"`
}

type createPaymentOrderInput struct {
	Amount string `json:"amount"`
	Method string `json:"method"`
}

type paymentOrderResponse struct {
	OrderNo    string `json:"order_no"`
	Amount     string `json:"amount"`
	RMBAmount  string `json:"rmb_amount"`
	Method     string `json:"method"`
	Status     string `json:"status"`
	PaymentURL string `json:"payment_url,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	PaidAt     string `json:"paid_at,omitempty"`
}

func (api *PaymentAPI) Config(c *gin.Context) {
	if !paymentFeatureEnabled || service.PersonalModeEnabled() {
		c.JSON(http.StatusOK, paymentConfigResponse{
			Enabled:             false,
			CurrencyDisplayName: firstNonEmptyString(settingString("payment_currency_display_name", "$"), "$"),
			USDToRMBRate:        settingDecimal("payment_usd_to_rmb_rate", "7.20").String(),
			MinRechargeAmount:   settingDecimal("payment_min_recharge_amount", "1").String(),
			RechargePresets:     []string{},
			Methods:             []string{},
		})
		return
	}
	cfg := currentPaymentConfig()
	c.JSON(http.StatusOK, paymentConfigResponse{
		Enabled:             cfg.Enabled,
		CurrencyDisplayName: cfg.CurrencyDisplayName,
		USDToRMBRate:        cfg.USDToRMBRate.String(),
		MinRechargeAmount:   cfg.MinRechargeAmount.String(),
		RechargePresets:     cfg.RechargePresets,
		Methods:             cfg.Methods,
	})
}

func (api *PaymentAPI) CreateOrder(c *gin.Context) {
	if !requirePaymentFeature(c) {
		return
	}
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	cfg := currentPaymentConfig()
	if !cfg.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment is disabled"})
		return
	}
	if err := validatePaymentGatewayConfig(cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var input createPaymentOrderInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(input.Amount))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid recharge amount"})
		return
	}
	if amount.LessThan(cfg.MinRechargeAmount) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Recharge amount is below the minimum"})
		return
	}
	method := strings.ToLower(strings.TrimSpace(input.Method))
	if !paymentMethodAllowed(method, cfg.Methods) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment method is not enabled"})
		return
	}
	if cfg.USDToRMBRate.LessThanOrEqual(decimal.Zero) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payment exchange rate"})
		return
	}
	orderNo, err := generatePaymentOrderNo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment order"})
		return
	}
	rmbAmount := amount.Mul(cfg.USDToRMBRate).Round(2)
	order := model.PaymentOrder{
		OrderNo:         orderNo,
		UserID:          user.ID,
		Amount:          amount,
		RMBAmount:       rmbAmount,
		ExchangeRate:    cfg.USDToRMBRate,
		Method:          method,
		Status:          paymentStatusPending,
		GatewayProvider: cfg.Provider,
	}
	if err := model.DB.Create(&order).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment order"})
		return
	}
	paymentURL, err := buildPaymentURL(c, cfg, order)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build payment URL"})
		return
	}
	response := toPaymentOrderResponse(order)
	response.PaymentURL = paymentURL
	c.JSON(http.StatusOK, response)
}

func (api *PaymentAPI) ListOrders(c *gin.Context) {
	if !requirePaymentFeature(c) {
		return
	}
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var orders []model.PaymentOrder
	query := model.DB.Model(&model.PaymentOrder{}).Where("user_id = ?", user.ID)
	var err error
	query, err = applyCreatedAtRange(query, c, "created_at")
	if writePaginationError(c, err) {
		return
	}
	if !wantsPaginatedResponse(c) {
		if err := query.Order("created_at DESC").Limit(100).Find(&orders).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load payment orders"})
			return
		}
		response := make([]paymentOrderResponse, 0, len(orders))
		for _, order := range orders {
			response = append(response, toPaymentOrderResponse(order))
		}
		c.JSON(http.StatusOK, response)
		return
	}

	page, pageSize := parsePagination(c)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count payment orders"})
		return
	}
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load payment orders"})
		return
	}
	response := make([]paymentOrderResponse, 0, len(orders))
	for _, order := range orders {
		response = append(response, toPaymentOrderResponse(order))
	}
	c.JSON(http.StatusOK, paginatedResponse{Items: response, Total: total, Page: page, PageSize: pageSize})
}

func (api *PaymentAPI) GetOrder(c *gin.Context) {
	if !requirePaymentFeature(c) {
		return
	}
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var order model.PaymentOrder
	if err := model.DB.Where("user_id = ? AND order_no = ?", user.ID, c.Param("order_no")).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment order not found"})
		return
	}
	c.JSON(http.StatusOK, toPaymentOrderResponse(order))
}

func (api *PaymentAPI) Notify(c *gin.Context) {
	if !paymentFeatureEnabled || service.PersonalModeEnabled() {
		c.String(http.StatusNotFound, "payment requires premium edition")
		return
	}
	ok, err := handlePaymentCallback(c)
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	if !ok {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	c.String(http.StatusOK, paymentNotifySuccessBody(c))
}

func (api *PaymentAPI) Return(c *gin.Context) {
	if !paymentFeatureEnabled || service.PersonalModeEnabled() {
		c.Redirect(http.StatusFound, "/dashboard/wallet?payment=failed")
		return
	}
	ok, _ := handlePaymentCallback(c)
	status := "failed"
	if ok {
		status = "success"
	}
	orderNo := firstNonEmptyString(strings.TrimSpace(c.Query("out_trade_no")), strings.TrimSpace(c.Query("merchant_order_no")))
	c.Redirect(http.StatusFound, "/dashboard/wallet?payment="+url.QueryEscape(status)+"&order_no="+url.QueryEscape(orderNo))
}

func handlePaymentCallback(c *gin.Context) (bool, error) {
	cfg := currentPaymentConfig()
	if cfg.Provider == paymentProviderOpenPayment {
		return handleOpenPaymentCallback(c, cfg)
	}
	return handleYipayCallback(c)
}

func handleYipayCallback(c *gin.Context) (bool, error) {
	cfg := currentPaymentConfig()
	if strings.TrimSpace(cfg.PID) == "" || strings.TrimSpace(cfg.Key) == "" {
		return false, errors.New("payment gateway is not configured")
	}
	params := paymentParams(c)
	if !verifyYipaySign(params, cfg.Key) {
		return false, errors.New("invalid sign")
	}
	if params["pid"] != cfg.PID {
		return false, errors.New("invalid pid")
	}
	if !yipayTradeSuccessful(params) {
		return false, errors.New("trade not successful")
	}
	orderNo := strings.TrimSpace(params["out_trade_no"])
	if orderNo == "" {
		return false, errors.New("missing order no")
	}
	money, err := decimal.NewFromString(strings.TrimSpace(params["money"]))
	if err != nil {
		return false, err
	}
	notifyPayload := paramsJSON(params)
	var order model.PaymentOrder
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			return err
		}
		if order.Status == paymentStatusPaid {
			return nil
		}
		if !money.Round(2).Equal(order.RMBAmount.Round(2)) {
			return errors.New("payment amount mismatch")
		}
		now := time.Now()
		updates := map[string]interface{}{
			"status":           paymentStatusPaid,
			"gateway_trade_no": params["trade_no"],
			"notify_payload":   notifyPayload,
			"paid_at":          &now,
		}
		if err := tx.Model(&order).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", order.UserID).UpdateColumn("balance", gorm.Expr("balance + ?", order.Amount)).Error
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func currentPaymentConfig() paymentConfig {
	if !paymentFeatureEnabled || service.PersonalModeEnabled() {
		return paymentConfig{
			Enabled:             false,
			Provider:            paymentProviderYipay,
			CurrencyDisplayName: firstNonEmptyString(settingString("payment_currency_display_name", "$"), "$"),
			USDToRMBRate:        settingDecimal("payment_usd_to_rmb_rate", "7.20"),
			MinRechargeAmount:   settingDecimal("payment_min_recharge_amount", "1"),
			RechargePresets:     []string{},
			Methods:             []string{},
		}
	}
	return paymentConfig{
		Enabled:               settingBool("payment_enabled", false),
		Provider:              normalizePaymentProvider(settingString("payment_gateway_provider", paymentProviderYipay)),
		CurrencyDisplayName:   firstNonEmptyString(settingString("payment_currency_display_name", "$"), "$"),
		USDToRMBRate:          settingDecimal("payment_usd_to_rmb_rate", "7.20"),
		MinRechargeAmount:     settingDecimal("payment_min_recharge_amount", "1"),
		RechargePresets:       parseJSONStringList(settingString("payment_recharge_presets", "[\"5\",\"10\",\"20\",\"50\",\"100\"]")),
		Methods:               parseJSONStringList(settingString("payment_methods", "[\"alipay\",\"wxpay\"]")),
		GatewayURL:            settingString("payment_yipay_gateway_url", ""),
		PID:                   settingString("payment_yipay_pid", ""),
		Key:                   settingString("payment_yipay_key", ""),
		NotifyURL:             settingString("payment_yipay_notify_url", ""),
		ReturnURL:             settingString("payment_yipay_return_url", ""),
		OpenPaymentBaseURL:    settingString("payment_openpayment_base_url", ""),
		OpenPaymentConfigURL:  settingString("payment_openpayment_config_url", ""),
		OpenPaymentMerchantID: settingString("payment_openpayment_merchant_id", ""),
		OpenPaymentKey:        settingString("payment_openpayment_key", ""),
		OpenPaymentNotifyURL:  "",
		OpenPaymentReturnURL:  "",
	}
}

func requirePaymentFeature(c *gin.Context) bool {
	if paymentFeatureEnabled && !service.PersonalModeEnabled() {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "Payment requires premium edition"})
	return false
}

func normalizePaymentProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "epay", "yipay":
		return paymentProviderYipay
	case "openpayment", "open-payment", "open_payment", "ops":
		return paymentProviderOpenPayment
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func validatePaymentGatewayConfig(cfg paymentConfig) error {
	switch cfg.Provider {
	case paymentProviderYipay:
		if strings.TrimSpace(cfg.GatewayURL) == "" || strings.TrimSpace(cfg.PID) == "" || strings.TrimSpace(cfg.Key) == "" {
			return errors.New("payment gateway is not configured")
		}
	case paymentProviderOpenPayment:
		if strings.TrimSpace(firstNonEmptyString(cfg.OpenPaymentConfigURL, cfg.OpenPaymentBaseURL)) == "" {
			return errors.New("Open Payment discovery URL is not configured")
		}
		if strings.TrimSpace(openPaymentMerchantID(cfg)) == "" || strings.TrimSpace(openPaymentMerchantKey(cfg)) == "" {
			return errors.New("Open Payment merchant is not configured")
		}
	default:
		return errors.New("unsupported payment gateway provider")
	}
	return nil
}

func parseJSONStringList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return []string{}
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func paymentMethodAllowed(method string, methods []string) bool {
	for _, allowed := range methods {
		if strings.EqualFold(method, strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func generatePaymentOrderNo() (string, error) {
	var raw [4]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("PAY%s%s", time.Now().Format("20060102150405"), strings.ToUpper(hex.EncodeToString(raw[:]))), nil
}

func buildPaymentURL(c *gin.Context, cfg paymentConfig, order model.PaymentOrder) (string, error) {
	if cfg.Provider == paymentProviderOpenPayment {
		return buildOpenPaymentPaymentURL(c, cfg, order)
	}
	return buildYipayPaymentURL(c, cfg, order)
}

func buildYipayPaymentURL(c *gin.Context, cfg paymentConfig, order model.PaymentOrder) (string, error) {
	notifyURL := firstNonEmptyString(cfg.NotifyURL, publicCallbackURL(c, "/api/payment/yipay/notify"))
	returnURL := firstNonEmptyString(cfg.ReturnURL, publicCallbackURL(c, "/api/payment/yipay/return"))
	params := map[string]string{
		"pid":          cfg.PID,
		"type":         order.Method,
		"out_trade_no": order.OrderNo,
		"notify_url":   notifyURL,
		"return_url":   returnURL,
		"name":         "Balance recharge " + order.OrderNo,
		"money":        order.RMBAmount.StringFixed(2),
		"sitename":     settingString("site_name", "flai"),
	}
	params["sign"] = buildYipaySign(params, cfg.Key)
	params["sign_type"] = "MD5"
	parsed, err := url.Parse(strings.TrimSpace(cfg.GatewayURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid payment gateway URL")
	}
	query := parsed.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func buildYipaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for name, value := range params {
		if name == "sign" || name == "sign_type" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+params[name])
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + key))
	return hex.EncodeToString(sum[:])
}

func verifyYipaySign(params map[string]string, key string) bool {
	sign := strings.ToLower(strings.TrimSpace(params["sign"]))
	if sign == "" {
		return false
	}
	return sign == buildYipaySign(params, key)
}

func paymentParams(c *gin.Context) map[string]string {
	_ = c.Request.ParseForm()
	params := map[string]string{}
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "application/json") && c.Request.Body != nil {
		var body map[string]interface{}
		if err := json.NewDecoder(c.Request.Body).Decode(&body); err == nil {
			for key, value := range body {
				params[key] = fmt.Sprint(value)
			}
		}
	}
	for key, values := range c.Request.Form {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	return params
}

func yipayTradeSuccessful(params map[string]string) bool {
	tradeStatus := strings.ToUpper(strings.TrimSpace(params["trade_status"]))
	status := strings.ToLower(strings.TrimSpace(params["status"]))
	return tradeStatus == "TRADE_SUCCESS" || tradeStatus == "TRADE_FINISHED" || tradeStatus == "SUCCESS" || status == "1" || status == "success" || status == "paid" || strings.TrimSpace(params["trade_no"]) != ""
}

func paramsJSON(params map[string]string) string {
	data, _ := json.Marshal(params)
	return string(data)
}

func publicCallbackURL(c *gin.Context, path string) string {
	baseURL := strings.TrimRight(settingString("base_url", ""), "/")
	if baseURL == "" {
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		baseURL = scheme + "://" + c.Request.Host
	}
	return baseURL + path
}

func toPaymentOrderResponse(order model.PaymentOrder) paymentOrderResponse {
	response := paymentOrderResponse{
		OrderNo:   order.OrderNo,
		Amount:    order.Amount.String(),
		RMBAmount: order.RMBAmount.StringFixed(2),
		Method:    order.Method,
		Status:    order.Status,
		CreatedAt: order.CreatedAt.Format(time.RFC3339),
	}
	if order.PaidAt != nil {
		response.PaidAt = order.PaidAt.Format(time.RFC3339)
	}
	return response
}
