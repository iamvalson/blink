package tiktok

type TikTokConfig struct {
	ClientKey    string
	ClientSecret string
	Scopes       []string
	RedirectURI  string
}

type TikTokTokenResponse struct {
	OpenId           string `json:"open_id"`
	Scope            string `json:"scope"`
	AccessToken      string `json:"access_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
}

type TikTokAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	LogID   string `json:"log_id"`
}

type TikTokQueryCreatorDataResponse struct {
	AvatarURL            string   `json:"creator_avatar_url"`
	Username             string   `json:"creator_username"`
	Nickname             string   `json:"creator_nickname"`
	PrivacyLevel         []string `json:"privacy_level_options"`
	CommentDisabled      bool     `json:"comment_disabled"`
	DuetDisabled         bool     `json:"duet_disabled"`
	StitchDisabled       bool     `json:"stitch_disabled"`
	MaxVideoPostDuration int64    `json:"max_video_post_duration_sec"`
}

type TikTokCreatorInfoAPIResponse struct {
	Data    TikTokQueryCreatorDataResponse `json:"data"`
	Error   TikTokAPIError                 `json:"error"`
	Code    string                         `json:"code,omitempty"`
	Message string                         `json:"message,omitempty"`
	LogID   string                         `json:"log_id,omitempty"`
}
