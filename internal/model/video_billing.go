package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

const QuotaTypeVideoResolutionDuration = 100

type VideoBillingConfig struct {
	Resolutions []VideoResolutionPrice `json:"resolutions"`
}

type VideoResolutionPrice struct {
	Resolution        string               `json:"resolution"`
	DurationUnitPrice decimal.Decimal      `json:"duration_unit_price,omitempty"`
	Durations         []VideoDurationPrice `json:"durations,omitempty"`
}

type VideoDurationPrice struct {
	Seconds int             `json:"seconds"`
	Price   decimal.Decimal `json:"price"`
}

func (config VideoBillingConfig) Value() (driver.Value, error) {
	normalized := NormalizeVideoBillingConfig(config)
	if len(normalized.Resolutions) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (config *VideoBillingConfig) Scan(value interface{}) error {
	if config == nil {
		return nil
	}
	if value == nil {
		*config = VideoBillingConfig{}
		return nil
	}

	var raw string
	switch typed := value.(type) {
	case []byte:
		raw = string(typed)
	case string:
		raw = typed
	default:
		return fmt.Errorf("unsupported video billing config value %T", value)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		*config = VideoBillingConfig{}
		return nil
	}

	var parsed VideoBillingConfig
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return err
	}
	*config = NormalizeVideoBillingConfig(parsed)
	return nil
}

func NormalizeVideoBillingConfig(config VideoBillingConfig) VideoBillingConfig {
	resolutionMap := map[string]VideoResolutionPrice{}
	for _, item := range config.Resolutions {
		resolution := NormalizeVideoResolution(item.Resolution)
		if resolution == "" {
			continue
		}

		durationMap := map[int]VideoDurationPrice{}
		for _, duration := range item.Durations {
			if duration.Seconds <= 0 || duration.Price.LessThan(decimal.Zero) {
				continue
			}
			durationMap[duration.Seconds] = VideoDurationPrice{
				Seconds: duration.Seconds,
				Price:   duration.Price,
			}
		}
		durations := make([]VideoDurationPrice, 0, len(durationMap))
		for _, duration := range durationMap {
			durations = append(durations, duration)
		}
		sort.Slice(durations, func(i, j int) bool {
			return durations[i].Seconds < durations[j].Seconds
		})

		unitPrice := item.DurationUnitPrice
		if unitPrice.LessThan(decimal.Zero) {
			unitPrice = decimal.Zero
		}
		if len(durations) > 0 {
			unitPrice = decimal.Zero
		}
		if len(durations) == 0 && unitPrice.IsZero() {
			continue
		}

		resolutionMap[resolution] = VideoResolutionPrice{
			Resolution:        resolution,
			DurationUnitPrice: unitPrice,
			Durations:         durations,
		}
	}
	resolutions := make([]VideoResolutionPrice, 0, len(resolutionMap))
	for _, item := range resolutionMap {
		resolutions = append(resolutions, item)
	}
	sort.Slice(resolutions, func(i, j int) bool {
		return resolutions[i].Resolution < resolutions[j].Resolution
	})

	return VideoBillingConfig{
		Resolutions: resolutions,
	}
}

func MultiplyVideoBillingConfig(config VideoBillingConfig, multiplier decimal.Decimal) VideoBillingConfig {
	normalized := NormalizeVideoBillingConfig(config)
	if multiplier.IsZero() {
		return normalized
	}
	for index := range normalized.Resolutions {
		normalized.Resolutions[index].DurationUnitPrice = normalized.Resolutions[index].DurationUnitPrice.Mul(multiplier)
		for durationIndex := range normalized.Resolutions[index].Durations {
			normalized.Resolutions[index].Durations[durationIndex].Price = normalized.Resolutions[index].Durations[durationIndex].Price.Mul(multiplier)
		}
	}
	return normalized
}

func NormalizeVideoResolution(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "*", "x")
	return value
}
