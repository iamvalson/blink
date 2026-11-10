package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/model"
)

func (p *PublishProcessor) validateInstagramVideo(ctx context.Context, media *model.Media, format string) error {
	const maxBytes = 1_000_000_000
	invalid := func() error {
		return newPublishError(connectors.ErrorPermanent, errors.New("Instagram requires an MP4/MOV video with H.264 or HEVC video and AAC audio if present"))
	}
	if media.FileSize <= 0 || media.FileSize > maxBytes {
		return invalid()
	}
	binary, err := exec.LookPath("ffprobe")
	if err != nil {
		return newPublishError(connectors.ErrorRetryable, errors.New("ffprobe is required to validate Instagram video"))
	}
	body, err := p.mediaStore.Get(ctx, media.StorageKey)
	if err != nil {
		return err
	}
	defer body.Close()
	file, err := os.CreateTemp("", "blink-ig-video-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	written, copyErr := io.Copy(file, io.LimitReader(body, maxBytes+1))
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return err
	}
	if written != media.FileSize {
		return invalid()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(probeCtx, binary, "-v", "error", "-protocol_whitelist", "file,pipe",
		"-format_whitelist", "mov", "-show_entries", "format=duration:stream=codec_type,codec_name,width,height",
		"-of", "json", file.Name()).Output()
	if probeCtx.Err() != nil {
		return newPublishError(connectors.ErrorRetryable, errors.New("Instagram video validation was interrupted"))
	}
	if err != nil {
		return invalid()
	}
	var metadata struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Width  int
			Height int
		} `json:"streams"`
	}
	if json.Unmarshal(raw, &metadata) != nil {
		return invalid()
	}
	duration, err := strconv.ParseFloat(metadata.Format.Duration, 64)
	maxDuration := 900.0
	if format == "STORIES" || format == "CAROUSEL" {
		maxDuration = 60
	}
	if err != nil || !(duration >= 3 && duration <= maxDuration) {
		return newPublishError(connectors.ErrorPermanent, errors.New("Instagram video duration is outside the supported range for this format"))
	}
	videoStreams := 0
	for _, stream := range metadata.Streams {
		switch stream.Type {
		case "video":
			videoStreams++
			if (stream.Codec != "h264" && stream.Codec != "hevc") || stream.Width < 1 || stream.Width > 1920 || stream.Height < 1 {
				return invalid()
			}
		case "audio":
			if stream.Codec != "aac" {
				return invalid()
			}
		}
	}
	if videoStreams != 1 {
		return invalid()
	}
	return nil
}
