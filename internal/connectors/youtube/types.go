package youtube

type YouTubeConfig struct {
	ClientID		string
	ClientSecret		string
	CallbackURL		string
}


type YouTubeChannelInfo struct {
	ID		string		`json:"id"`
	Snippet struct{
		Title			string		`json:"title"`
		Description		string		`json:"description"`
		Thumbnails		map[string]struct {
			URL		string		`json:"url"`
		} `json:"thumbnails"`
	} `json:"snippet"`
}