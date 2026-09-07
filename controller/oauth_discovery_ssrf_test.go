package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
)

func mustNewGetRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("failed to build test request for %s: %v", rawURL, err)
	}
	return req
}

// withFetchSettingAllowPrivateIp temporarily overrides the shared
// FetchSetting.AllowPrivateIp toggle (the same admin-configurable switch
// service/http_client.go's checkRedirect already relies on) and restores it
// afterward, so tests can exercise the default-off and explicit-opt-in
// paths without leaking state into other tests.
func withFetchSettingAllowPrivateIp(t *testing.T, allow bool) {
	t.Helper()
	fetchSetting := system_setting.GetFetchSetting()
	previous := fetchSetting.AllowPrivateIp
	fetchSetting.AllowPrivateIp = allow
	t.Cleanup(func() { fetchSetting.AllowPrivateIp = previous })
}

func TestValidateDiscoveryURL_AcceptsGoogleDiscovery(t *testing.T) {
	if _, err := validateDiscoveryURL("https://accounts.google.com/.well-known/openid-configuration"); err != nil {
		t.Errorf("expected Google's real discovery URL to be accepted, got error: %v", err)
	}
}

func TestValidateDiscoveryURL_AcceptsOrdinaryPublicHTTPS(t *testing.T) {
	if _, err := validateDiscoveryURL("https://login.example-enterprise.com/.well-known/openid-configuration"); err != nil {
		t.Errorf("expected an ordinary public https discovery URL to be accepted, got error: %v", err)
	}
}

func TestValidateDiscoveryURL_RejectsHTTP(t *testing.T) {
	if _, err := validateDiscoveryURL("http://accounts.google.com/.well-known/openid-configuration"); err == nil {
		t.Fatal("expected http to be rejected, got nil")
	}
}

func TestValidateDiscoveryURL_RejectsLocalhost(t *testing.T) {
	if _, err := validateDiscoveryURL("https://localhost/.well-known/openid-configuration"); err == nil {
		t.Fatal("expected localhost to be rejected, got nil")
	}
}

func TestValidateDiscoveryURL_RejectsLocalhostSubdomain(t *testing.T) {
	if _, err := validateDiscoveryURL("https://foo.localhost/.well-known/openid-configuration"); err == nil {
		t.Fatal("expected a .localhost subdomain to be rejected, got nil")
	}
}

func TestValidateDiscoveryURL_RejectsLoopbackIP(t *testing.T) {
	if _, err := validateDiscoveryURL("https://127.0.0.1/.well-known/openid-configuration"); err == nil {
		t.Fatal("expected a loopback IP literal to be rejected, got nil")
	}
}

func TestValidateDiscoveryURL_RejectsPrivateIPv4(t *testing.T) {
	for _, host := range []string{"10.0.0.1", "172.16.0.1", "192.168.1.1"} {
		if _, err := validateDiscoveryURL("https://" + host + "/.well-known/openid-configuration"); err == nil {
			t.Errorf("expected private IPv4 %s to be rejected, got nil", host)
		}
	}
}

func TestValidateDiscoveryURL_RejectsLinkLocalAndMetadata(t *testing.T) {
	// 169.254.169.254 is the common cloud metadata address (AWS/GCP/Azure);
	// it falls within the 169.254.0.0/16 link-local range already blocked.
	for _, host := range []string{"169.254.169.254", "169.254.1.1"} {
		if _, err := validateDiscoveryURL("https://" + host + "/.well-known/openid-configuration"); err == nil {
			t.Errorf("expected link-local/metadata address %s to be rejected, got nil", host)
		}
	}
}

func TestValidateDiscoveryURL_RejectsIPv6Loopback(t *testing.T) {
	if _, err := validateDiscoveryURL("https://[::1]/.well-known/openid-configuration"); err == nil {
		t.Fatal("expected ::1 to be rejected, got nil")
	}
}

func TestValidateDiscoveryURL_RejectsUserInfo(t *testing.T) {
	if _, err := validateDiscoveryURL("https://admin:pw@accounts.google.com/.well-known/openid-configuration"); err == nil {
		t.Fatal("expected a URL with embedded userinfo to be rejected, got nil")
	}
}

func TestValidateDiscoveryURL_RejectsMissingHost(t *testing.T) {
	if _, err := validateDiscoveryURL("https:///.well-known/openid-configuration"); err == nil {
		t.Fatal("expected a URL with no host to be rejected, got nil")
	}
}

func TestValidateDiscoveryURL_RejectsUnusualPortToPrivateAddress(t *testing.T) {
	if _, err := validateDiscoveryURL("https://10.0.0.5:8443/.well-known/openid-configuration"); err == nil {
		t.Fatal("expected a private address on a non-standard port to still be rejected, got nil")
	}
}

// P1-02 note: Google's real discovery endpoint must never be caught by
// these guardrails regardless of how strict the private-IP checks are -
// accounts.google.com is a public hostname, not an IP literal, so it is
// never evaluated against the private-IP allowlist by validateDiscoveryURL
// itself (that check only applies when the host is already a literal IP).
func TestValidateDiscoveryURL_GoogleNeverFalsePositive(t *testing.T) {
	urls := []string{
		"https://accounts.google.com/.well-known/openid-configuration",
		"https://oauth2.googleapis.com/token",
	}
	for _, u := range urls {
		if _, err := validateDiscoveryURL(u); err != nil {
			t.Errorf("expected %s to never be rejected, got error: %v", u, err)
		}
	}
}

func TestValidateDiscoveryURL_OrdinaryOAuthProviderUnaffectedByGoogleWhitelist(t *testing.T) {
	// This helper is shared by all custom OAuth providers, not just Google;
	// it must not impose any Google-specific allowlist on other issuers.
	if _, err := validateDiscoveryURL("https://login.microsoftonline.com/common/.well-known/openid-configuration"); err != nil {
		t.Errorf("expected a non-Google public OIDC issuer to be accepted, got error: %v", err)
	}
}

func TestValidateDiscoveryURL_PrivateIpAllowedWhenExplicitlyOptedIn(t *testing.T) {
	withFetchSettingAllowPrivateIp(t, true)
	if _, err := validateDiscoveryURL("https://10.0.0.5/.well-known/openid-configuration"); err != nil {
		t.Errorf("expected a private IP to be accepted once AllowPrivateIp is explicitly enabled, got error: %v", err)
	}
}

func TestValidateDiscoveryURL_PrivateIpBlockedByDefault(t *testing.T) {
	withFetchSettingAllowPrivateIp(t, false)
	if _, err := validateDiscoveryURL("https://10.0.0.5/.well-known/openid-configuration"); err == nil {
		t.Fatal("expected a private IP to be rejected by default (AllowPrivateIp=false), got nil")
	}
}

func TestDiscoveryCheckRedirect_RejectsRedirectToPrivateAddress(t *testing.T) {
	req := mustNewGetRequest(t, "https://10.0.0.5/.well-known/openid-configuration")
	if err := discoveryCheckRedirect(req, nil); err == nil {
		t.Fatal("expected a redirect to a private address to be rejected, got nil")
	}
}

func TestDiscoveryCheckRedirect_AllowsRedirectToPublicHTTPS(t *testing.T) {
	req := mustNewGetRequest(t, "https://accounts.google.com/.well-known/openid-configuration")
	if err := discoveryCheckRedirect(req, nil); err != nil {
		t.Errorf("expected a redirect to a public https URL to be allowed, got error: %v", err)
	}
}

func TestDiscoveryCheckRedirect_RejectsTooManyRedirects(t *testing.T) {
	req := mustNewGetRequest(t, "https://accounts.google.com/.well-known/openid-configuration")
	via := make([]*http.Request, discoveryMaxRedirects)
	for i := range via {
		via[i] = mustNewGetRequest(t, "https://accounts.google.com/.well-known/openid-configuration")
	}
	if err := discoveryCheckRedirect(req, via); err == nil {
		t.Fatal("expected exceeding the redirect cap to be rejected, got nil")
	}
}
