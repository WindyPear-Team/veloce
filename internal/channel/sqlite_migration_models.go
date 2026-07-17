package channel

import "github.com/WindyPear-Team/veloce/internal/model"

func init() {
	model.RegisterSQLiteMigrationModels(&MessageChannelIntegration{}, &MessageChannelMessage{})
}
