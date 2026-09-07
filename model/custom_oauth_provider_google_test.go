package model

import "testing"

// These cover the server-side guardrails in validateCustomOAuthProvider that
// are specific to Google-type rows: provider_type is restricted to a known
// set of values, and scopes are restricted to the minimal identity set so an
// admin cannot accidentally wire this login integration up to request
// Gmail/Drive/Calendar or other unrelated Google API access.

func baseGoogleProvider() *CustomOAuthProvider {
	return &CustomOAuthProvider{
		Name:                  "Google",
		Slug:                  "google",
		ClientId:              "client-id",
		ClientSecret:          "client-secret",
		AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint:         "https://oauth2.googleapis.com/token",
		UserInfoEndpoint:      "https://openidconnect.googleapis.com/v1/userinfo",
		ProviderType:          "google",
	}
}

func TestValidateCustomOAuthProvider_GoogleDefaultScopes(t *testing.T) {
	p := baseGoogleProvider()
	if err := validateCustomOAuthProvider(p); err != nil {
		t.Fatalf("expected valid Google provider, got error: %v", err)
	}
	if p.Scopes != "openid profile email" {
		t.Errorf("expected default scopes to be applied, got %q", p.Scopes)
	}
}

func TestValidateCustomOAuthProvider_GoogleRejectsUnrelatedScopes(t *testing.T) {
	p := baseGoogleProvider()
	p.Scopes = "openid email https://www.googleapis.com/auth/drive"
	if err := validateCustomOAuthProvider(p); err == nil {
		t.Fatal("expected an error for a Drive scope on a Google provider, got nil")
	}
}

func TestValidateCustomOAuthProvider_GoogleAllowsMinimalScopes(t *testing.T) {
	p := baseGoogleProvider()
	p.Scopes = "openid email profile"
	if err := validateCustomOAuthProvider(p); err != nil {
		t.Errorf("expected openid/email/profile to be allowed, got error: %v", err)
	}
}

func TestValidateCustomOAuthProvider_RejectsUnknownProviderType(t *testing.T) {
	p := baseGoogleProvider()
	p.ProviderType = "microsoft"
	if err := validateCustomOAuthProvider(p); err == nil {
		t.Fatal("expected an error for an unsupported provider_type, got nil")
	}
}

func TestValidateCustomOAuthProvider_EmptyProviderTypeIsGeneric(t *testing.T) {
	p := baseGoogleProvider()
	p.ProviderType = ""
	if err := validateCustomOAuthProvider(p); err != nil {
		t.Errorf("expected empty provider_type (generic) to be valid, got error: %v", err)
	}
}

// P1-02: a provider_type=google row must only ever point at Google's
// official endpoints. These guardrails apply at Create/Update time; the
// corresponding runtime enforcement (which cannot be bypassed by editing
// the database directly) lives in oauth.GoogleOAuthProvider.effectiveConfig
// and is covered by oauth/google_test.go.

func TestValidateCustomOAuthProvider_GoogleCanonicalEndpointsAccepted(t *testing.T) {
	p := baseGoogleProvider()
	if err := validateCustomOAuthProvider(p); err != nil {
		t.Fatalf("expected canonical Google endpoints to be accepted, got error: %v", err)
	}
}

func TestValidateCustomOAuthProvider_GoogleRejectsNonGoogleAuthorizationEndpoint(t *testing.T) {
	p := baseGoogleProvider()
	p.AuthorizationEndpoint = "https://accounts.evil.example/o/oauth2/v2/auth"
	if err := validateCustomOAuthProvider(p); err == nil {
		t.Fatal("expected an error for a non-Google authorization_endpoint, got nil")
	}
}

func TestValidateCustomOAuthProvider_GoogleRejectsNonGoogleTokenEndpoint(t *testing.T) {
	p := baseGoogleProvider()
	p.TokenEndpoint = "https://oauth2.evil.example/token"
	if err := validateCustomOAuthProvider(p); err == nil {
		t.Fatal("expected an error for a non-Google token_endpoint, got nil")
	}
}

func TestValidateCustomOAuthProvider_GoogleRejectsHTTPEndpoint(t *testing.T) {
	p := baseGoogleProvider()
	p.TokenEndpoint = "http://oauth2.googleapis.com/token"
	if err := validateCustomOAuthProvider(p); err == nil {
		t.Fatal("expected an error for an http (non-https) Google token_endpoint, got nil")
	}
}

func TestValidateCustomOAuthProvider_GoogleRejectsLookalikeHost(t *testing.T) {
	p := baseGoogleProvider()
	// A host that merely contains "google.com" as a substring/subdomain
	// trick, not the real host.
	p.TokenEndpoint = "https://oauth2.googleapis.com.evil.example/token"
	if err := validateCustomOAuthProvider(p); err == nil {
		t.Fatal("expected an error for a lookalike token_endpoint host, got nil")
	}
}

func TestValidateCustomOAuthProvider_GoogleAllowsEmptyWellKnown(t *testing.T) {
	p := baseGoogleProvider()
	p.WellKnown = ""
	if err := validateCustomOAuthProvider(p); err != nil {
		t.Errorf("expected empty well_known to be allowed for Google, got error: %v", err)
	}
}

func TestValidateCustomOAuthProvider_GoogleRejectsNonCanonicalWellKnown(t *testing.T) {
	p := baseGoogleProvider()
	p.WellKnown = "https://accounts.evil.example/.well-known/openid-configuration"
	if err := validateCustomOAuthProvider(p); err == nil {
		t.Fatal("expected an error for a non-canonical well_known on a Google provider, got nil")
	}
}

func TestValidateCustomOAuthProvider_NonGoogleProviderEndpointsUnrestricted(t *testing.T) {
	p := baseGoogleProvider()
	p.ProviderType = ""
	p.AuthorizationEndpoint = "https://login.example-enterprise.com/oauth/authorize"
	p.TokenEndpoint = "https://login.example-enterprise.com/oauth/token"
	p.WellKnown = "https://login.example-enterprise.com/.well-known/openid-configuration"
	if err := validateCustomOAuthProvider(p); err != nil {
		t.Errorf("expected a generic (non-Google) provider to keep using its own endpoints, got error: %v", err)
	}
}
