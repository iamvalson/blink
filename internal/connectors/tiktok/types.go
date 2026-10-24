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
