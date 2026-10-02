package connectors

import "testing"

func TestHTTPErrorClassification(t *testing.T) {
	if got := ClassifyError(HTTPError(503, "outage")); got != ErrorRetryable {
		t.Fatalf("503 classified as %s, want %s", got, ErrorRetryable)
	}
	if got := ClassifyError(HTTPError(400, "invalid")); got != ErrorPermanent {
		t.Fatalf("400 classified as %s, want %s", got, ErrorPermanent)
	}
}
