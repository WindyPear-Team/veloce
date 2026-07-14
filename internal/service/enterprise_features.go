package service

import "github.com/WindyPear-Team/veloce/internal/config"

// EnterpriseFeaturesEnabled gates enterprise-only routes and migrations while
// the multi-tenant implementation is introduced incrementally. It defaults to
// false so existing personal and community installations retain their current
// behavior until an administrator explicitly opts in.
func EnterpriseFeaturesEnabled() bool {
	return config.EnterpriseFeaturesEnabled
}
