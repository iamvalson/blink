package model

import (
	"errors"
	"math"
)

// TikTokOptions records the creator's explicit choices for one target account.
type TikTokOptions struct {
	CommercialContent  bool    `json:"commercial_content"`
	PrivacyLevel       string  `json:"privacy_level"`
	DisableComment     bool    `json:"disable_comment"`
	DisableDuet        bool    `json:"disable_duet"`
	DisableStitch      bool    `json:"disable_stitch"`
	BrandContentToggle bool    `json:"brand_content_toggle"`
	BrandOrganicToggle bool    `json:"brand_organic_toggle"`
	IsAIGC             bool    `json:"is_aigc"`
	Consent            bool    `json:"consent"`
	DurationSeconds    float64 `json:"duration_seconds"`
}

func (o TikTokOptions) Validate() error {
	if o.CommercialContent && !o.BrandContentToggle && !o.BrandOrganicToggle {
		return errors.New("TikTok commercial content requires a disclosure selection")
	}
	if !o.CommercialContent && (o.BrandContentToggle || o.BrandOrganicToggle) {
		return errors.New("TikTok commercial disclosure must be enabled")
	}
	if !o.Consent {
		return errors.New("TikTok publishing requires explicit consent")
	}
	switch o.PrivacyLevel {
	case "PUBLIC_TO_EVERYONE", "MUTUAL_FOLLOW_FRIENDS", "FOLLOWER_OF_CREATOR", "SELF_ONLY":
	default:
		return errors.New("TikTok privacy must be explicitly selected")
	}
	if o.BrandContentToggle && o.PrivacyLevel != "PUBLIC_TO_EVERYONE" && o.PrivacyLevel != "MUTUAL_FOLLOW_FRIENDS" {
		return errors.New("TikTok branded content requires public or friends visibility")
	}
	if math.IsNaN(o.DurationSeconds) || math.IsInf(o.DurationSeconds, 0) || o.DurationSeconds <= 0 {
		return errors.New("TikTok video duration is required")
	}
	return nil
}
