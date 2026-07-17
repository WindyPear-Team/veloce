package premium

import "github.com/WindyPear-Team/veloce/internal/model"

func init() {
	model.RegisterSQLiteMigrationModels(
		&SubscriptionPlan{},
		&UserSubscription{},
		&PremiumRedeemCode{},
		&PremiumRedemptionLog{},
		&MetaModel{},
		&AdvancedChatMemoryDocument{},
	)
}
