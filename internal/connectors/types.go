package connectors

import "time"



type AuthParams struct {
	Code 			string
	CodeVerifier	string
	State 			string
}

type AuthResult struct {
	PlatformUserID		string
	AccessToken			string
	RefreshToken		string
	Expiry				time.Time
}