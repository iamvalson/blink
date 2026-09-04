package connectors



type AuthParams struct {
	Code 			string
	CodeVerifier	string
	State 			string
}