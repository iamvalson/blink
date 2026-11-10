package instagram

import (
	"errors"
	"image/jpeg"
	"io"
)

const MaxFeedImageBytes = 8_000_000

var ErrStoryImage = errors.New("Instagram Story photos must be valid JPEGs up to 8 MB and 320–1440 px wide")

var ErrFeedImage = errors.New("Instagram feed photos must be valid JPEGs under 8 MB, 320–1440 px wide, with an aspect ratio from 4:5 to 1.91:1")

// ValidateFeedImage checks the stored JPEG header without allocating a full image.
func ValidateFeedImage(r io.Reader, size int64) error {
	return ValidateImage(r, size, "IMAGE")
}

func ValidateImage(r io.Reader, size int64, format string) error {
	invalid := ErrFeedImage
	if format == "STORIES" {
		invalid = ErrStoryImage
	}
	if size <= 0 || size > MaxFeedImageBytes {
		return invalid
	}
	config, err := jpeg.DecodeConfig(io.LimitReader(r, MaxFeedImageBytes))
	if err != nil || config.Width < 320 || config.Width > 1440 || config.Height <= 0 ||
		(format != "STORIES" && (5*config.Width < 4*config.Height || 100*config.Width > 191*config.Height)) {
		return invalid
	}
	return nil
}
