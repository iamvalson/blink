package tiktok

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf16"

	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/model"
)

var _ connectors.AsyncVideoPublisher = (*Connector)(nil)

type DirectPostRequest struct {
	PostInfo   PostInfo   `json:"post_info"`
	SourceInfo SourceInfo `json:"source_info"`
}
type PostInfo struct {
	Title              string `json:"title"`
	PrivacyLevel       string `json:"privacy_level"`
	DisableComment     bool   `json:"disable_comment"`
	DisableDuet        bool   `json:"disable_duet"`
	DisableStitch      bool   `json:"disable_stitch"`
	BrandContentToggle bool   `json:"brand_content_toggle"`
	BrandOrganicToggle bool   `json:"brand_organic_toggle"`
	IsAIGC             bool   `json:"is_aigc"`
}
type SourceInfo struct {
	Source   string `json:"source"`
	VideoURL string `json:"video_url"`
}

// ValidateMediaURL requires the exact configured HTTPS origin and path boundary.
// Configuring a prefix does not perform TikTok's dashboard ownership verification.
func ValidateMediaURL(raw, prefix string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid TikTok media URL")
	}
	p, err := url.Parse(prefix)
	if err != nil || p.Scheme != "https" || p.Host == "" || p.RawQuery != "" || p.Fragment != "" || p.User != nil {
		return errors.New("verified HTTPS media prefix is not configured")
	}
	if u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || net.ParseIP(u.Hostname()) != nil || !strings.EqualFold(u.Host, p.Host) || !strings.HasPrefix(u.EscapedPath(), strings.TrimRight(p.EscapedPath(), "/")+"/") || u.RawPath != "" || strings.Contains(u.Path, "..") {
		return errors.New("TikTok media URL must belong to the configured verified HTTPS prefix")
	}
	return nil
}
func validationError(err error) error {
	return connectors.NewClassifiedError(connectors.ErrorPermanent, "TIKTOK_VALIDATION", err)
}
func (c *Connector) ValidateVideo(ctx context.Context, token string, in connectors.VideoPublishInput) error {
	if err := in.Options.Validate(); err != nil {
		return validationError(err)
	}
	if len(utf16.Encode([]rune(in.Caption))) > 2200 {
		return validationError(errors.New("TikTok caption exceeds 2200 UTF-16 units"))
	}
	if err := ValidateMediaURL(in.MediaURL, c.mediaURLPrefix); err != nil {
		return validationError(err)
	}
	creator, err := c.QueryCreatorInfo(ctx, token)
	if err != nil {
		if connectors.ClassifyError(err) == connectors.ErrorAmbiguous {
			return connectors.NewClassifiedError(connectors.ErrorRetryable, "CREATOR_QUERY_UNAVAILABLE", errors.New("TikTok creator query unavailable"))
		}
		return err
	}
	return ValidateCreatorOptions(in.Options, creator)
}
func ValidateCreatorOptions(o model.TikTokOptions, creator *TikTokQueryCreatorDataResponse) error {
	if creator == nil {
		return validationError(errors.New("TikTok creator information is unavailable"))
	}
	allowed := false
	for _, p := range creator.PrivacyLevel {
		if p == o.PrivacyLevel {
			allowed = true
		}
	}
	if !allowed {
		return validationError(errors.New("TikTok privacy is unavailable for this creator"))
	}
	if creator.MaxVideoPostDuration <= 0 || o.DurationSeconds > float64(creator.MaxVideoPostDuration) {
		return validationError(errors.New("TikTok video exceeds creator duration limit"))
	}
	if creator.CommentDisabled && !o.DisableComment || creator.DuetDisabled && !o.DisableDuet || creator.StitchDisabled && !o.DisableStitch {
		return validationError(errors.New("TikTok interaction is disabled by the creator"))
	}
	return nil
}
func (c *Connector) InitializeVideo(ctx context.Context, token string, in connectors.VideoPublishInput) (string, error) {
	if err := in.Options.Validate(); err != nil {
		return "", validationError(err)
	}
	if len(utf16.Encode([]rune(in.Caption))) > 2200 {
		return "", validationError(errors.New("TikTok caption exceeds 2200 UTF-16 units"))
	}
	if err := ValidateMediaURL(in.MediaURL, c.mediaURLPrefix); err != nil {
		return "", validationError(err)
	}
	o := in.Options
	body := DirectPostRequest{PostInfo: PostInfo{in.Caption, o.PrivacyLevel, o.DisableComment, o.DisableDuet, o.DisableStitch, o.BrandContentToggle, o.BrandOrganicToggle, o.IsAIGC}, SourceInfo: SourceInfo{"PULL_FROM_URL", in.MediaURL}}
	var data struct {
		PublishID string `json:"publish_id"`
	}
	if err := c.postJSON(ctx, token, "/v2/post/publish/video/init/", body, &data, true); err != nil {
		return "", err
	}
	if data.PublishID == "" || len(data.PublishID) > 64 {
		return "", connectors.NewClassifiedError(connectors.ErrorAmbiguous, "TIKTOK_INIT_UNKNOWN", errors.New("TikTok initialization returned no valid publish ID"))
	}
	return data.PublishID, nil
}
func (c *Connector) FetchPublishStatus(ctx context.Context, token, id string) (*connectors.VideoPublishStatus, error) {
	var data struct {
		Status        string        `json:"status"`
		FailureReason string        `json:"fail_reason"`
		IDs           []json.Number `json:"publicaly_available_post_id"`
	}
	if err := c.postJSON(ctx, token, "/v2/post/publish/status/fetch/", map[string]string{"publish_id": id}, &data, false); err != nil {
		return nil, err
	}
	result := &connectors.VideoPublishStatus{Status: data.Status, FailureReason: data.FailureReason}
	for _, id := range data.IDs {
		result.PublicPostIDs = append(result.PublicPostIDs, id.String())
	}
	return result, nil
}
func (c *Connector) postJSON(ctx context.Context, token, path string, input, output any, initializing bool) error {
	if strings.TrimSpace(token) == "" {
		return connectors.ErrAuthFailed
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return validationError(errors.New("invalid TikTok request"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://open.tiktokapis.com"+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := c.httpClient.Do(req)
	unknown := func() error {
		class := connectors.ErrorRetryable
		if initializing {
			class = connectors.ErrorAmbiguous
		}
		return connectors.NewClassifiedError(class, "TIKTOK_RESPONSE_UNKNOWN", errors.New("TikTok request outcome could not be confirmed"))
	}
	if err != nil {
		return unknown()
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSuccessResponseBytes+1))
	if err != nil || len(raw) > maxSuccessResponseBytes {
		return unknown()
	}
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error TikTokAPIError  `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return unknown()
	}
	if resp.StatusCode >= 500 {
		return unknown()
	}
	if envelope.Error.Code != "ok" || resp.StatusCode != http.StatusOK {
		if envelope.Error.Code == "" {
			return unknown()
		}
		// Never include remote error text: it can echo a signed media URL or credentials.
		code := envelope.Error.Code
		if code == "spam_risk_too_many_posts" || code == "reached_active_user_cap" {
			return connectors.NewClassifiedError(connectors.ErrorPermanent, code, fmt.Errorf("TikTok rejected request: %s", code))
		}
		return classifyTikTokAPIError(code, "TikTok rejected request", "", resp.StatusCode, resp.Header)
	}
	if json.Unmarshal(envelope.Data, output) != nil {
		return unknown()
	}
	return nil
}
