package twitter

// TwitterConfig holds Twitter API credentials
type TwitterConfig struct {
	ClientID		string
	ClientSecret	string
	CallbackURL		string
	BearerToken		string	// For API calls after OAuth
}


// TwitterUserInfo holds authenticated user data
type TwitterUserInfo struct {
	ID				string
	Username		string
	Name			string
}


// TweetResponse is what Twitter API returns after posting
type TweetResponse struct {
	Data struct {
		ID		string	`json:"id"`
		Text	string	`json:"text"`
	} `json:"data"`
}