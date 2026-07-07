package service

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/WindyPear-Team/flai/internal/model"
)

var ErrInsufficientBalance = errors.New("insufficient balance")

type StartupHook func() error
type RouteHook func(*gin.RouterGroup)
type UsageChargeHook func(tx *gorm.DB, userID uint, cost decimal.Decimal) error
type MetaModelListHook func(*gin.Context) ([]string, error)
type MetaModelResolveHook func(*gin.Context, MetaModelResolveInput) (MetaModelResolveResult, error)
type MetaModelCatalogHook func(*gin.Context) ([]MetaModelCatalogItem, error)
type GeneratedAssetHook func(context.Context, GeneratedAssetInput)

type MetaModelResolveInput struct {
	ModelName    string
	RequestBody  map[string]interface{}
	OriginalBody []byte
}

type MetaModelResolveResult struct {
	Matched              bool
	ModelName            string
	BillingMode          string
	BillingModel         *model.Model
	SkipAPIKeyModelCheck bool
	ErrorStatus          int
	ErrorMessage         string
}

type MetaModelCatalogItem struct {
	Name                   string          `json:"name"`
	Description            string          `json:"description"`
	Provider               string          `json:"provider"`
	ProviderName           string          `json:"provider_name"`
	ProviderIconURL        string          `json:"provider_icon_url"`
	BillingMode            string          `json:"billing_mode"`
	InputPrice             decimal.Decimal `json:"input_price"`
	OutputPrice            decimal.Decimal `json:"output_price"`
	CachedInputPrice       decimal.Decimal `json:"cached_input_price"`
	ExposeReferencedModels bool            `json:"expose_referenced_models"`
	ReferencedModels       []string        `json:"referenced_models"`
}

type GeneratedAssetInput struct {
	UserID       uint
	Kind         string
	ModelName    string
	ResponseData map[string]interface{}
	ResponseBody []byte
	Source       string
}

var startupHooks []StartupHook
var publicAPIRouteHooks []RouteHook
var adminRouteHooks []RouteHook
var userRouteHooks []RouteHook
var usageChargeHook UsageChargeHook
var metaModelListHook MetaModelListHook
var metaModelResolveHook MetaModelResolveHook
var metaModelCatalogHook MetaModelCatalogHook
var generatedAssetHook GeneratedAssetHook

func RegisterStartupHook(hook StartupHook) {
	startupHooks = append(startupHooks, hook)
}

func RunStartupHooks() error {
	for _, hook := range startupHooks {
		if hook == nil {
			continue
		}
		if err := hook(); err != nil {
			return err
		}
	}
	return nil
}

func RegisterAdminRouteHook(hook RouteHook) {
	adminRouteHooks = append(adminRouteHooks, hook)
}

func RegisterPublicAPIRouteHook(hook RouteHook) {
	publicAPIRouteHooks = append(publicAPIRouteHooks, hook)
}

func RegisterUserRouteHook(hook RouteHook) {
	userRouteHooks = append(userRouteHooks, hook)
}

func ApplyPublicAPIRouteHooks(group *gin.RouterGroup) {
	for _, hook := range publicAPIRouteHooks {
		if hook != nil {
			hook(group)
		}
	}
}

func ApplyAdminRouteHooks(group *gin.RouterGroup) {
	for _, hook := range adminRouteHooks {
		if hook != nil {
			hook(group)
		}
	}
}

func ApplyUserRouteHooks(group *gin.RouterGroup) {
	for _, hook := range userRouteHooks {
		if hook != nil {
			hook(group)
		}
	}
}

func RegisterUsageChargeHook(hook UsageChargeHook) {
	usageChargeHook = hook
}

func RegisterMetaModelHooks(listHook MetaModelListHook, resolveHook MetaModelResolveHook) {
	metaModelListHook = listHook
	metaModelResolveHook = resolveHook
}

func RegisterMetaModelCatalogHook(hook MetaModelCatalogHook) {
	metaModelCatalogHook = hook
}

func RegisterGeneratedAssetHook(hook GeneratedAssetHook) {
	generatedAssetHook = hook
}

func ListMetaModelNames(c *gin.Context) ([]string, error) {
	if metaModelListHook == nil {
		return nil, nil
	}
	return metaModelListHook(c)
}

func ListMetaModelCatalog(c *gin.Context) ([]MetaModelCatalogItem, error) {
	if metaModelCatalogHook == nil {
		return nil, nil
	}
	return metaModelCatalogHook(c)
}

func ApplyGeneratedAssetHook(ctx context.Context, input GeneratedAssetInput) {
	if generatedAssetHook == nil {
		return
	}
	generatedAssetHook(ctx, input)
}

func ResolveMetaModel(c *gin.Context, input MetaModelResolveInput) (MetaModelResolveResult, error) {
	if metaModelResolveHook == nil {
		return MetaModelResolveResult{}, nil
	}
	result, err := metaModelResolveHook(c, input)
	if result.ErrorStatus == 0 && result.ErrorMessage != "" {
		result.ErrorStatus = http.StatusBadRequest
	}
	return result, err
}

func ApplyUsageCharge(tx *gorm.DB, userID uint, cost decimal.Decimal) error {
	if cost.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	if PersonalModeEnabled() {
		return nil
	}
	if usageChargeHook != nil {
		return usageChargeHook(tx, userID, cost)
	}
	balanceUpdate := tx.Exec("UPDATE users SET balance = balance - ? WHERE id = ? AND balance >= ?", cost, userID, cost)
	if balanceUpdate.Error != nil {
		return balanceUpdate.Error
	}
	if balanceUpdate.RowsAffected == 0 {
		return ErrInsufficientBalance
	}
	return nil
}
