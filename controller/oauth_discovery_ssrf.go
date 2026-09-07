package controller

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// discoveryMaxRedirects caps how many redirects FetchCustomOAuthDiscovery
// will follow. Each hop is independently re-validated by
// discoveryCheckRedirect below, so this is a depth limit, not the only
// protection against a redirect chain that walks into a private network.
const discoveryMaxRedirects = 3

// discoveryMaxResponseBytes caps how much of a discovery document response
// body is ever read, regardless of what Content-Length claims, so a
// malicious or misbehaving endpoint cannot exhaust memory on this Root-only
// route.
const discoveryMaxResponseBytes = 1 << 20 // 1 MiB

// validateDiscoveryURL enforces the parts of the SSRF policy that can be
// checked from the URL text alone, before any DNS resolution or network
// call: https only (never plain http, regardless of the FetchSetting SSRF
// toggle below - a discovery endpoint always carries no user secrets over
// http and there is no legitimate reason for an OIDC discovery document to
// be fetched unencrypted), a host must be present, and no embedded
// userinfo (https://user:pass@host/..., a classic SSRF/parser-confusion
// trick).
func validateDiscoveryURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("Discovery URL 无效: %v", err)
	}
	if parsed.Scheme != "https" {
		return nil, errors.New("Discovery URL 仅支持 https")
	}
	if parsed.Host == "" {
		return nil, errors.New("Discovery URL 缺少主机名")
	}
	if parsed.User != nil {
		return nil, errors.New("Discovery URL 不允许包含用户信息")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, errors.New("Discovery URL 不允许使用 localhost")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !discoverySSRFProtection().IsIPAccessAllowed(ip) {
			return nil, fmt.Errorf("Discovery URL 不允许指向内网/保留地址: %s", ip.String())
		}
	}
	return parsed, nil
}

// discoverySSRFProtection builds the IP allow/deny policy this helper
// enforces from the same admin-configurable FetchSetting used elsewhere in
// the app for outbound requests (webhooks, downloads, video proxy - see
// service/http_client.go). Reusing it means private/internal discovery
// targets are blocked by default (AllowPrivateIp defaults to false) but
// remain available, as an explicit opt-in an admin can already reach today,
// for legitimate on-prem/private OIDC deployments that genuinely need it -
// rather than this helper inventing a second, parallel toggle.
func discoverySSRFProtection() *common.SSRFProtection {
	fetchSetting := system_setting.GetFetchSetting()
	return &common.SSRFProtection{
		AllowPrivateIp: fetchSetting.AllowPrivateIp,
	}
}

// discoveryCheckRedirect re-validates every redirect hop against the same
// policy as the initial request (scheme, host shape, private/reserved IP)
// and caps the redirect chain length, so a discovery endpoint cannot use an
// HTTP redirect to walk this Root-only route into an internal address after
// the initial URL passed validation.
func discoveryCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= discoveryMaxRedirects {
		return fmt.Errorf("重定向次数超过限制(%d)", discoveryMaxRedirects)
	}
	if _, err := validateDiscoveryURL(req.URL.String()); err != nil {
		return fmt.Errorf("重定向目标被拒绝: %w", err)
	}
	return nil
}

// discoverySafeDialContext resolves the target host itself (rather than
// letting the transport's default dialer resolve it separately from the
// validation step) and checks every resolved address against the SSRF
// policy immediately before connecting to it. This closes the DNS-rebinding
// gap that a pure pre-flight ValidateURL check has: a name that resolved to
// a public IP when the URL was first validated could resolve to a private
// one by the time the actual TCP connection is dialed. Every candidate
// address is tried in order; the first one that both passes the policy and
// successfully connects is used.
func discoverySafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	protection := discoverySSRFProtection()
	dialer := &net.Dialer{Timeout: 5 * time.Second}

	if ip := net.ParseIP(host); ip != nil {
		if !protection.IsIPAccessAllowed(ip) {
			return nil, fmt.Errorf("blocked address: %s", ip.String())
		}
		return dialer.DialContext(ctx, network, addr)
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, ip := range ips {
		if !protection.IsIPAccessAllowed(ip) {
			lastErr = fmt.Errorf("blocked address: %s (resolved from %s)", ip.String(), host)
			continue
		}
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no addresses resolved for host %s", host)
	}
	return nil, lastErr
}

// newDiscoveryHTTPClient builds an http.Client scoped to a single discovery
// fetch: a safe dial path (discoverySafeDialContext), bounded connection,
// TLS handshake, response-header and overall timeouts, and a redirect
// policy that re-validates every hop (discoveryCheckRedirect). It is
// intentionally not a shared/package-level client, since FetchSetting (and
// therefore the SSRF policy it encodes) can change at runtime via the admin
// panel.
func newDiscoveryHTTPClient() *http.Client {
	transport := &http.Transport{
		DialContext:           discoverySafeDialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       10 * time.Second,
	}
	return &http.Client{
		Transport:     transport,
		Timeout:       20 * time.Second,
		CheckRedirect: discoveryCheckRedirect,
	}
}
