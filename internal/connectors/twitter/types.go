package twitter

import "strconv"

// TwitterConfig holds Twitter API credentials
type TwitterConfig struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
	BearerToken  string // For API calls after OAuth
}

// TwitterUserInfo holds authenticated user data
type TwitterUserInfo struct {
	ID       string
	Username string
	Name     string
}

// TweetResponse is what Twitter API returns after posting
type TweetResponse struct {
	Data struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
}

// MediaUploadResponse represents the response from X media upload endpoints.
type MediaUploadResponse struct {
	Data struct {
		ID             string          `json:"id"`
		MediaKey       string          `json:"media_key"`
		ProcessingInfo *ProcessingInfo `json:"processing_info"`
	} `json:"data"`
	MediaIDString  string          `json:"media_id_string"`
	MediaID        int64           `json:"media_id"`
	ProcessingInfo *ProcessingInfo `json:"processing_info"`
}

func (r *MediaUploadResponse) GetMediaID() string {
	if r.Data.ID != "" {
		return r.Data.ID
	}
	if r.MediaIDString != "" {
		return r.MediaIDString
	}
	if r.MediaID != 0 {
		return strconv.FormatInt(r.MediaID, 10)
	}
	return ""
}

func (r *MediaUploadResponse) GetProcessingInfo() *ProcessingInfo {
	if r.Data.ProcessingInfo != nil {
		return r.Data.ProcessingInfo
	}
	return r.ProcessingInfo
}

// ProcessingInfo represents asynchronous processing status for media.
type ProcessingInfo struct {
	State          string           `json:"state"`
	CheckAfterSecs int              `json:"check_after_secs"`
	ProgressPct    int              `json:"progress_percent"`
	Error          *ProcessingError `json:"error"`
}

// ProcessingError represents an error returned by X during media processing.
type ProcessingError struct {
	Code    int    `json:"code"`
	Name    string `json:"name"`
	Message string `json:"message"`
}
