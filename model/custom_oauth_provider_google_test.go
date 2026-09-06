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
