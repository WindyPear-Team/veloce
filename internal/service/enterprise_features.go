package service

import "github.com/WindyPear-Team/veloce/internal/config"

// EnterpriseFeaturesEnabled gates single-enterprise private-deployment routes
// and migrations. It defaults to false so existing personal and community
// installations retain their current behavior until explicitly enabled.
func EnterpriseFeaturesEnabled() bool {
	return config.EnterpriseFeaturesEnabled
}
