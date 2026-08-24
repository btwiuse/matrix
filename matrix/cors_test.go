package matrix_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gearshell/inject-proxy/matrix"
)

// TestCORSAllowsAllOrigins checks the preflight and actual-response headers.
func TestCORSAllowsAllOrigins(t *testing.T) {
	var hitInner bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitInner = true
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(matrix.NewCORS(inner))
	defer srv.Close()

	// Preflight: 204, never reaches the inner handler.
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/deploy", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", res.StatusCode)
	}
	if hitInner {
		t.Error("preflight reached the inner handler")
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}

	// Actual request carries the header too.
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/deploy", nil)
	res2, err := srv.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if got := res2.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if !hitInner {
		t.Error("actual request did not reach the inner handler")
	}
}
