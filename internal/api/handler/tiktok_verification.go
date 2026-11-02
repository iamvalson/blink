package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var verificationFilename = regexp.MustCompile(`(?i)^tiktok[a-z0-9_-]+\.txt$`)

// WithTikTokVerification exposes exactly the dashboard-provided public file.
// It must wrap signed media delivery so verification never needs a media token.
// The file is read once at startup; no directory or arbitrary path is exposed.
func WithTikTokVerification(next http.Handler, file, publicPath string) (http.Handler, error) {
	if file == "" && publicPath == "" {
		return next, nil
	}
	if file == "" || publicPath == "" {
		return nil, errors.New("both TIKTOK_VERIFICATION_FILE and TIKTOK_VERIFICATION_PATH must be configured")
	}
	name := filepath.Base(file)
	if !verificationFilename.MatchString(name) || (publicPath != "/"+name && publicPath != "/tiktok-media/"+name) || path.Clean(publicPath) != publicPath || strings.ContainsAny(publicPath, "?#%") {
		return nil, errors.New("TikTok verification requires its original tiktok*.txt filename at /filename or /tiktok-media/filename")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.New("could not open TikTok verification file")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 64*1024 {
		return nil, errors.New("TikTok verification file must be a nonempty regular file of at most 64 KiB")
	}
	body, err := io.ReadAll(io.LimitReader(f, 64*1024+1))
	if err != nil || len(body) == 0 || len(body) > 64*1024 {
		return nil, errors.New("could not read TikTok verification file")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != publicPath || r.URL.RawPath != "" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, bytes.NewReader(body))
		}
	}), nil
}
