package connectors

import "time"

type AuthParams struct {
	Code         string
	CodeVerifier string
	State        string
}

type AuthResult struct {
	AvatarURL       string
	ProviderSubject string
	Username        string
	PlatformUserID  string
	AccessToken     string
	RefreshToken    string
	Expiry          time.Time
}
