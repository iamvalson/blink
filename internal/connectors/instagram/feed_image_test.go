package instagram

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"
)

func TestValidateFeedImage(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
		valid         bool
	}{
		{"portrait", 1080, 1350, true},
		{"square", 1080, 1080, true},
		{"landscape boundary", 955, 500, true},
		{"too wide", 956, 500, false},
		{"too tall", 1080, 1351, false},
		{"too narrow", 319, 320, false},
		{"too large", 1441, 1441, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var data bytes.Buffer
			if err := jpeg.Encode(&data, image.NewGray(image.Rect(0, 0, tt.width, tt.height)), nil); err != nil {
				t.Fatal(err)
			}
			if err := ValidateFeedImage(bytes.NewReader(data.Bytes()), int64(data.Len())); (err == nil) != tt.valid {
				t.Fatalf("valid=%t, error=%v", tt.valid, err)
			}
		})
	}
	if err := ValidateFeedImage(bytes.NewReader([]byte("not a jpeg")), 10); err == nil {
		t.Fatal("accepted invalid image bytes")
	}
	if err := ValidateFeedImage(bytes.NewReader(nil), MaxFeedImageBytes+1); err == nil {
		t.Fatal("accepted oversized upload")
	}
}

func TestValidateImageKeepsStoryAndCarouselRulesSeparate(t *testing.T) {
	var data bytes.Buffer
	if err := jpeg.Encode(&data, image.NewGray(image.Rect(0, 0, 1080, 1920)), nil); err != nil {
		t.Fatal(err)
	}
	size := int64(data.Len())
	if err := ValidateImage(bytes.NewReader(data.Bytes()), size, "STORIES"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateImage(bytes.NewReader(data.Bytes()), size, "CAROUSEL"); err == nil {
		t.Fatal("accepted an uncropped 9:16 JPEG for a carousel")
	}
}
