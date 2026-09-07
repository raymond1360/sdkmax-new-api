package middleware

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newRequestWithRemoteAddr(t *testing.T, remoteAddr string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	return req
}

func TestIsRequestHTTPS_TrustedProxyHonorsHeader(t *testing.T) {
	req := newRequestWithRemoteAddr(t, "127.0.0.1:54321")
	req.Header.Set("X-Forwarded-Proto", "https")
	if !isRequestHTTPS(req) {
		t.Error("expected HTTPS to be honored from a trusted loopback peer")
	}
}

func TestIsRequestHTTPS_TrustedProxyHonorsHTTPHeader(t *testing.T) {
	req := newRequestWithRemoteAddr(t, "127.0.0.1:54321")
	req.Header.Set("X-Forwarded-Proto", "http")
	if isRequestHTTPS(req) {
		t.Error("expected http from a trusted loopback peer to not be treated as HTTPS")
	}
}

func TestIsRequestHTTPS_TrustedProxyIPv6Loopback(t *testing.T) {
	req := newRequestWithRemoteAddr(t, "[::1]:54321")
	req.Header.Set("X-Forwarded-Proto", "https")
	if !isRequestHTTPS(req) {
		t.Error("expected HTTPS to be honored from a trusted IPv6 loopback peer")
	}
}

// This is the core of P2-01: a request that reaches this process directly
// (bypassing the trusted reverse proxy) must not be able to spoof
// X-Forwarded-Proto: https and receive a Secure session cookie over a
// connection that was never actually encrypted.
func TestIsRequestHTTPS_UntrustedPeerHeaderIgnored(t *testing.T) {
	req := newRequestWithRemoteAddr(t, "203.0.113.5:54321")
	req.Header.Set("X-Forwarded-Proto", "https")
	if isRequestHTTPS(req) {
		t.Error("expected X-Forwarded-Proto from an untrusted peer to be ignored")
	}
}

func TestIsRequestHTTPS_UntrustedPeerWithDirectTLSStillHTTPS(t *testing.T) {
	req := newRequestWithRemoteAddr(t, "203.0.113.5:54321")
	req.TLS = &tls.ConnectionState{}
	if !isRequestHTTPS(req) {
		t.Error("expected a directly TLS-terminated request to be treated as HTTPS regardless of peer trust")
	}
}

func TestIsRequestHTTPS_NoHeaderNoTLS(t *testing.T) {
	req := newRequestWithRemoteAddr(t, "127.0.0.1:54321")
	if isRequestHTTPS(req) {
		t.Error("expected no header and no TLS to be treated as not HTTPS")
	}
}

func TestIsRequestHTTPS_MalformedRemoteAddrTreatedAsUntrusted(t *testing.T) {
	// No port, and not a bare IP either - must not panic and must not be
	// trusted.
	req := newRequestWithRemoteAddr(t, "not-an-address")
	req.Header.Set("X-Forwarded-Proto", "https")
	if isRequestHTTPS(req) {
		t.Error("expected a malformed RemoteAddr to be treated as untrusted, not to honor the header")
	}
}

func TestIsTrustedProxyPeer_CustomCIDR(t *testing.T) {
	previous := trustedProtoProxyCIDRs
	trustedProtoProxyCIDRs = parseTrustedProxyCIDRs("10.0.0.0/8,127.0.0.1/32")
	t.Cleanup(func() { trustedProtoProxyCIDRs = previous })

	if !isTrustedProxyPeer("10.1.2.3:1234") {
		t.Error("expected 10.1.2.3 to be trusted under a custom 10.0.0.0/8 override")
	}
	if isTrustedProxyPeer("203.0.113.5:1234") {
		t.Error("expected a public IP to remain untrusted even with a custom CIDR override")
	}
}

func TestParseTrustedProxyCIDRs_AcceptsBareIP(t *testing.T) {
	nets := parseTrustedProxyCIDRs("192.168.1.1")
	if len(nets) != 1 {
		t.Fatalf("expected exactly one parsed net, got %d", len(nets))
	}
	if !nets[0].Contains(net.ParseIP("192.168.1.1")) {
		t.Error("expected the bare IP to parse as an exact-match /32")
	}
	if nets[0].Contains(net.ParseIP("192.168.1.2")) {
		t.Error("expected a bare IP to only match itself, not neighboring addresses")
	}
}

func TestParseTrustedProxyCIDRs_IgnoresInvalidEntries(t *testing.T) {
	nets := parseTrustedProxyCIDRs("not-a-cidr, , 127.0.0.1/32")
	if len(nets) != 1 {
		t.Fatalf("expected only the valid entry to be parsed, got %d nets", len(nets))
	}
}
