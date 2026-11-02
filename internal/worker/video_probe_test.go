package worker

import (
	"context"
	"errors"
	"github.com/iamvalson/blink/internal/connectors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTikTokProbeValidation(t *testing.T) {
	for _, tc := range []struct {
		name, json string
		valid      bool
	}{
		{"supported", `{"format":{"duration":"30"},"streams":[{"codec_name":"h264","width":1080,"height":1920,"avg_frame_rate":"30000/1001"}]}`, true},
		{"bad_codec", `{"format":{"duration":"30"},"streams":[{"codec_name":"mpeg2video","width":1080,"height":1920,"avg_frame_rate":"30/1"}]}`, false},
		{"bad_dimensions", `{"format":{"duration":"30"},"streams":[{"codec_name":"h264","width":100,"height":1920,"avg_frame_rate":"30/1"}]}`, false},
		{"bad_fps", `{"format":{"duration":"30"},"streams":[{"codec_name":"h264","width":1080,"height":1920,"avg_frame_rate":"0/0"}]}`, false},
		{"bad_duration", `{"format":{"duration":"NaN"},"streams":[{"codec_name":"h264","width":1080,"height":1920,"avg_frame_rate":"30/1"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "ffprobe")
			script := "#!/bin/sh\nprintf '%s' '" + tc.json + "'\n"
			if err := os.WriteFile(path, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			duration, err := probeTikTokVideo(context.Background(), "test-file")
			if (err == nil) != tc.valid {
				t.Fatalf("%v", err)
			}
			if tc.valid && duration != 30 {
				t.Fatal("wrong duration")
			}
		})
	}
}

func TestMissingTikTokProbeHasActionableRetryableError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := probeTikTokVideo(context.Background(), "unused")
	var classified *connectors.ClassifiedError
	if !errors.As(err, &classified) || classified.Code != "VIDEO_PROBE_UNAVAILABLE" || classified.Class != connectors.ErrorRetryable {
		t.Fatalf("wrong missing-dependency error: %v", err)
	}
}
func TestTikTokProbeWithRealVideo(t *testing.T) {
	binary, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}
	video := filepath.Join(t.TempDir(), "test.mp4")
	output, err := exec.Command(binary, "-v", "error", "-f", "lavfi", "-i", "color=c=black:s=360x640:r=30", "-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", video).CombinedOutput()
	if err != nil {
		t.Fatalf("generate test video: %v %s", err, output)
	}
	duration, err := probeTikTokVideo(context.Background(), video)
	if err != nil || duration < 0.9 || duration > 1.1 {
		t.Fatalf("probe real video: duration=%v err=%v", duration, err)
	}
}
