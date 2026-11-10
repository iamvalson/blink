package instagram

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/iamvalson/blink/internal/connectors"
)

func (c *Connector) endpoint(id, edge string) string {
	return c.graph + "/" + c.version + "/" + id + edge
}

func (c *Connector) CreateContainer(ctx context.Context, token, account string, params url.Values) (string, error) {
	var result struct {
		ID string `json:"id"`
	}
	if !remoteID.MatchString(account) {
		return "", connectors.ErrValidationFailed
	}
	if err := c.call(ctx, "POST", c.endpoint(account, "/media"), token, params, &result); err != nil {
		return "", err
	}
	if !remoteID.MatchString(result.ID) {
		return "", errors.New("Instagram returned no container ID")
	}
	return result.ID, nil
}

func (c *Connector) ContainerStatus(ctx context.Context, token, id string) (string, error) {
	var result struct {
		Code string `json:"status_code"`
	}
	if !remoteID.MatchString(id) {
		return "", connectors.ErrValidationFailed
	}
	err := c.call(ctx, "GET", c.endpoint(id, ""), token, url.Values{"fields": {"status_code"}}, &result)
	return result.Code, err
}

func (c *Connector) PublishContainer(ctx context.Context, token, account, id string) (string, error) {
	var result struct {
		ID string `json:"id"`
	}
	if !remoteID.MatchString(account) || !remoteID.MatchString(id) {
		return "", connectors.ErrValidationFailed
	}
	if err := c.call(ctx, "POST", c.endpoint(account, "/media_publish"), token, url.Values{"creation_id": {id}}, &result); err != nil {
		return "", err
	}
	if !remoteID.MatchString(result.ID) {
		return "", errors.New("Instagram did not return a published media ID")
	}
	return result.ID, nil
}

func (c *Connector) Permalink(ctx context.Context, token, id string) string {
	var result struct {
		URL string `json:"permalink"`
	}
	if !remoteID.MatchString(id) || c.call(ctx, "GET", c.endpoint(id, ""), token, url.Values{"fields": {"permalink"}}, &result) != nil {
		return ""
	}
	parsed, err := url.Parse(result.URL)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != "www.instagram.com" && parsed.Host != "instagram.com") {
		return ""
	}
	return parsed.String()
}

func (c *Connector) CheckPublishingLimit(ctx context.Context, token, account string) error {
	if !remoteID.MatchString(account) {
		return connectors.ErrValidationFailed
	}
	var result struct {
		Data []struct {
			Usage  int `json:"quota_usage"`
			Config struct {
				Total int `json:"quota_total"`
			} `json:"config"`
		} `json:"data"`
	}
	err := c.call(ctx, "GET", c.endpoint(account, "/content_publishing_limit"), token, url.Values{"fields": {"quota_usage,config"}}, &result)
	if err != nil {
		return err
	}
	if len(result.Data) > 0 && result.Data[0].Config.Total > 0 && result.Data[0].Usage >= result.Data[0].Config.Total {
		return connectors.ErrRateLimited
	}
	return nil
}

func MediaParams(format, contentType, mediaURL, caption string, child bool) url.Values {
	params := url.Values{}
	if format != "IMAGE" {
		params.Set("media_type", format)
	}
	if strings.HasPrefix(contentType, "video/") {
		params.Set("video_url", mediaURL)
	} else {
		params.Set("image_url", mediaURL)
	}
	if child {
		params.Set("is_carousel_item", "true")
	} else if format != "STORIES" && caption != "" {
		params.Set("caption", caption)
	}
	return params
}
