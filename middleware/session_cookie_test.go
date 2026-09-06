package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsRequestHTTPS_NoHeaderPlainHTTP(t *testing.T) {
	// Local http://localhost:3000 development: no reverse proxy, no TLS.
	req := httptest.NewRequest(http.MethodGet, "http://localhost:3000/oauth/google", nil)
	if isRequestHTTPS(req) {
		t.Error("expected plain local HTTP request to be treated as insecure (Secure=false)")
	}
}

func TestIsRequestHTTPS_ForwardedProtoHTTPS(t *testing.T) {
	// Production: Nginx terminates TLS and forwards X-Forwarded-Proto: https.
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/oauth/google", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	if !isRequestHTTPS(req) {
		t.Error("expected X-Forwarded-Proto: https to be treated as secure (Secure=true)")
	}
}

func TestIsRequestHTTPS_ForwardedProtoHTTP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/oauth/google", nil)
	req.Header.Set("X-Forwarded-Proto", "http")
	if isRequestHTTPS(req) {
		t.Error("expected X-Forwarded-Proto: http to be treated as insecure")
	}
}

func TestIsRequestHTTPS_ForwardedProtoMultiValue(t *testing.T) {
	// Some proxies chain multiple values; only the first (closest to the
	// client-facing edge) should be trusted.
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/oauth/google", nil)
	req.Header.Set("X-Forwarded-Proto", "https, http")
	if !isRequestHTTPS(req) {
		t.Error("expected first value in X-Forwarded-Proto list to determine scheme")
	}
}

func TestIsRequestHTTPS_DirectTLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/oauth/google", nil)
	req.TLS = &tls.ConnectionState{}
	if !isRequestHTTPS(req) {
		t.Error("expected a request with a populated TLS connection state to be treated as secure")
	}
}
