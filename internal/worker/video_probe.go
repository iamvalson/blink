package worker

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
)

func probeTikTokVideo(ctx context.Context, path string) (float64, error) {
	binary, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0, connectors.NewClassifiedError(connectors.ErrorRetryable, "VIDEO_PROBE_UNAVAILABLE", errors.New("worker requires ffprobe for TikTok video validation"))
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-select_streams", "v:0", "-show_entries", "format=duration:stream=codec_name,width,height,avg_frame_rate", "-of", "json", path).Output()
	invalid := func() (float64, error) {
		return 0, connectors.NewClassifiedError(connectors.ErrorPermanent, "INVALID_VIDEO", errors.New("TikTok video must have supported codec, duration, dimensions and frame rate"))
	}
	if err != nil {
		if ctx.Err() != nil {
			return 0, connectors.NewClassifiedError(connectors.ErrorRetryable, "VIDEO_PROBE_INTERRUPTED", errors.New("TikTok video validation was interrupted"))
		}
		return invalid()
	}
	var meta struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			FPS    string `json:"avg_frame_rate"`
		} `json:"streams"`
	}
	if json.Unmarshal(out, &meta) != nil || len(meta.Streams) != 1 {
		return invalid()
	}
	duration, err := strconv.ParseFloat(meta.Format.Duration, 64)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 600 {
		return invalid()
	}
	v := meta.Streams[0]
	if v.Width < 360 || v.Width > 4096 || v.Height < 360 || v.Height > 4096 {
		return invalid()
	}
	switch v.Codec {
	case "h264", "hevc", "vp8", "vp9":
	default:
		return invalid()
	}
	parts := strings.Split(v.FPS, "/")
	if len(parts) != 2 {
		return invalid()
	}
	n, e := strconv.ParseFloat(parts[0], 64)
	d, e2 := strconv.ParseFloat(parts[1], 64)
	fps := n / d
	if e != nil || e2 != nil || d <= 0 || math.IsNaN(fps) || math.IsInf(fps, 0) || fps < 23 || fps > 60 {
		return invalid()
	}
	return duration, nil
}
