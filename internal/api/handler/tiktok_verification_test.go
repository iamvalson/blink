package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTikTokVerificationServesExactFileOnly(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tiktokTest_123.txt")
	body := "dashboard-provided-signature\n"
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "signed media authorization required", 403)
	})
	for _, publicPath := range []string{"/tiktokTest_123.txt", "/tiktok-media/tiktokTest_123.txt"} {
		h, err := WithTikTokVerification(next, file, publicPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, method := range []string{"GET", "HEAD", "POST"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, "https://example.test"+publicPath, nil))
			if method == "POST" {
				if w.Code != 405 {
					t.Fatal("POST accepted")
				}
				continue
			}
			if w.Code != 200 || w.Header().Get("Location") != "" || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
				t.Fatalf("invalid verification response: %d", w.Code)
			}
			if method == "GET" && w.Body.String() != body {
				t.Fatal("signature contents changed")
			}
			if method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD returned a body")
			}
		}
		for _, other := range []string{"/tiktok-media/private-video", "/.env", publicPath + "/extra"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", other, nil))
			if w.Code != 403 {
				t.Fatal("verification bypass leaked to another path")
			}
		}
	}
}
func TestTikTokVerificationRejectsUnsafeConfiguration(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tiktokTest.txt")
	if err := os.WriteFile(file, []byte("signature"), 0600); err != nil {
		t.Fatal(err)
	}
	next := http.NotFoundHandler()
	for _, tc := range []struct{ file, path string }{{file, ""}, {"", "/tiktokTest.txt"}, {file, "/tiktok-media/../tiktokTest.txt"}, {file, "/wrong.txt"}, {filepath.Join(dir, ".env"), "/.env"}, {filepath.Join(dir, "tiktokMissing.txt"), "/tiktokMissing.txt"}} {
		if _, err := WithTikTokVerification(next, tc.file, tc.path); err == nil {
			t.Errorf("accepted invalid configuration: %s", tc.path)
		}
	}
	if err := os.WriteFile(file, []byte(strings.Repeat("x", 65537)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := WithTikTokVerification(next, file, "/tiktokTest.txt"); err == nil {
		t.Fatal("oversized signature accepted")
	}
}
