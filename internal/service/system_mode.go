package service

import (
	"strings"

	"github.com/WindyPear-Team/veloce/internal/model"
)

const (
	SystemModeOperation = "operation"
	SystemModePersonal  = "personal"
)

func NormalizeSystemMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SystemModePersonal:
		return SystemModePersonal
	default:
		return SystemModeOperation
	}
}

func CurrentSystemMode() string {
	if model.DB == nil {
		return SystemModeOperation
	}
	return NormalizeSystemMode(model.GetSystemSetting("system_mode", SystemModeOperation))
}

func PersonalModeEnabled() bool {
	return CurrentSystemMode() == SystemModePersonal
}
