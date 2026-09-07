package oauth

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
)

// Compile-time check: GoogleOAuthProvider must satisfy ConfigurableOAuthProvider
// so controller.findOrCreateOAuthUser/handleOAuthBind route it through the
// user_oauth_bindings persistence path instead of the built-in-provider path
// (which has nowhere to store a Google identity). See registry.go.
var _ ConfigurableOAuthProvider = (*GoogleOAuthProvider)(nil)

// GoogleProviderType marks a custom_oauth_providers row as using Google's
// ID Token verification flow (issuer/audience/signature/exp/nonce checked by
// a maintained OIDC library) instead of the generic userinfo-endpoint flow.
const GoogleProviderType = "google"

// GoogleDefaultIssuer is Google's OIDC issuer. It is the ONLY issuer a
// provider_type=google row will ever be verified against - see issuer()
// below, which deliberately ignores the row's WellKnown field.
const GoogleDefaultIssuer = "https://accounts.google.com"

// GoogleAuthorizationEndpoint, GoogleTokenEndpoint and GoogleDiscoveryURL
// are Google's official OAuth/OIDC endpoints. A provider_type=google row is
// never trusted to supply its own values for these at runtime (see
// effectiveConfig below): the custom_oauth_providers table is admin/API
// writable, and model.validateCustomOAuthProvider rejecting bad input at
// write time is not sufficient on its own, since a row can also be edited
// directly in the database, bypassing that validation entirely. Google
// itself does not rotate these URLs; if it ever does, update here.
const (
	GoogleAuthorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
	GoogleTokenEndpoint         = "https://oauth2.googleapis.com/token"
	GoogleDiscoveryURL          = GoogleDefaultIssuer + "/.well-known/openid-configuration"
)

// googleIssuerOverrideForTests lets tests point Google ID Token discovery at
// a local fake issuer instead of the real accounts.google.com. It is never
// set from database-controlled configuration (custom_oauth_providers.well_known
// has no effect on which issuer a provider_type=google row actually
// verifies against - see issuer() below), only from Go test code in this
// package, so a tampered or misconfigured database row can never redirect
// verification to a non-Google host in production.
var googleIssuerOverrideForTests string

// GoogleOAuthProvider signs users in with Google using the ID Token returned
// alongside the access token, rather than calling a REST userinfo endpoint.
// Token exchange, provider-id binding and username generation are inherited
// unchanged from GenericOAuthProvider; only GetUserInfo is overridden so
// that identity comes from a cryptographically verified ID Token instead of
// an unauthenticated HTTP response body.
type GoogleOAuthProvider struct {
	*GenericOAuthProvider

	verifierOnce sync.Once
	verifier     *oidc.IDTokenVerifier
	verifierErr  error
}

// NewGoogleOAuthProvider creates a Google OAuth provider from a
// custom_oauth_providers row. The row is expected to have ProviderType set
// to GoogleProviderType.
func NewGoogleOAuthProvider(config *model.CustomOAuthProvider) *GoogleOAuthProvider {
	return &GoogleOAuthProvider{GenericOAuthProvider: NewGenericOAuthProvider(config)}
}

// issuer returns the OIDC issuer to run discovery against. This is
// deliberately hardcoded to GoogleDefaultIssuer and does NOT read
// p.GetConfig().WellKnown: a provider_type=google row's well_known column is
// admin/API writable (and can also be edited directly in the database,
// bypassing model.validateCustomOAuthProvider entirely), so letting it
// influence which issuer we trust would let a misconfigured or tampered row
// silently downgrade Google's OIDC security guarantees to those of an
// arbitrary third-party issuer (P1-02). The only supported way to point
// discovery elsewhere is googleIssuerOverrideForTests, which only Go test
// code in this package can set.
func (p *GoogleOAuthProvider) issuer() string {
	if googleIssuerOverrideForTests != "" {
		return googleIssuerOverrideForTests
	}
	return GoogleDefaultIssuer
}

// effectiveConfig returns a copy of the provider's stored configuration with
// every network-reachable OAuth endpoint forced to Google's official
// values, regardless of what is stored in custom_oauth_providers. This is
// the defense-in-depth layer behind P1-02: even a directly-edited/tampered
// database row cannot redirect token exchange (which carries the Client
// Secret) to a non-Google host. AuthStyle is also forced to the params
// style Google's token endpoint expects, independent of what an admin may
// have configured for a different provider before switching provider_type.
func (p *GoogleOAuthProvider) effectiveConfig() *model.CustomOAuthProvider {
	safe := *p.GetConfig()
	safe.AuthorizationEndpoint = GoogleAuthorizationEndpoint
	safe.TokenEndpoint = GoogleTokenEndpoint
	safe.WellKnown = GoogleDiscoveryURL
	safe.AuthStyle = AuthStyleInParams
	return &safe
}

// ExchangeToken overrides GenericOAuthProvider.ExchangeToken so the token
// exchange request (which carries the Client Secret) always goes to
// Google's official token endpoint, never to whatever is stored in
// p.GetConfig().TokenEndpoint. See effectiveConfig.
func (p *GoogleOAuthProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	return NewGenericOAuthProvider(p.effectiveConfig()).ExchangeToken(ctx, code, c)
}

// getVerifier lazily builds an ID Token verifier via OIDC discovery. It is
// built once per provider instance (the registry creates a fresh instance on
// every config change, see RegisterOrUpdateCustomProvider) and cached, since
// discovery requires a network round trip.
func (p *GoogleOAuthProvider) getVerifier(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	p.verifierOnce.Do(func() {
		oidcProvider, err := oidc.NewProvider(ctx, p.issuer())
		if err != nil {
			p.verifierErr = fmt.Errorf("failed to initialize Google OIDC discovery: %w", err)
			return
		}
		p.verifier = oidcProvider.Verifier(&oidc.Config{ClientID: p.GetConfig().ClientId})
	})
	return p.verifier, p.verifierErr
}

// googleIDTokenClaims mirrors the subset of Google's ID Token claims this
// provider relies on. Google always encodes email_verified as a JSON
// boolean in ID Tokens (unlike some userinfo endpoints of other providers
// that use a string), so a bool field is correct here.
type googleIDTokenClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Nonce         string `json:"nonce"`
}

// validateGoogleClaims checks the post-verification claims on a Google ID
// Token: nonce (bound to the session that started the flow), email_verified,
// a non-empty email, and a non-empty sub. It is deliberately a pure function
// of already-verified claims (issuer/audience/signature/expiry are checked
// by go-oidc before this ever runs, see GetUserInfo) so it can be unit
// tested without any network dependency or ID Token signing.
func validateGoogleClaims(claims googleIDTokenClaims, expectedNonce string, providerName string) error {
	if expectedNonce == "" || claims.Nonce == "" || claims.Nonce != expectedNonce {
		return NewOAuthError(i18n.MsgOAuthStateInvalid, nil)
	}

	if !claims.EmailVerified {
		return NewOAuthError(i18n.MsgOAuthEmailNotVerified, map[string]any{"Provider": providerName})
	}

	// Google normally always returns email for the openid+email scopes this
	// integration requests. An email_verified=true claim with an empty email
	// is not a state Google's own token endpoint should ever produce, so
	// treat it the same as any other malformed/untrustworthy response rather
	// than letting it flow into user creation or the email-conflict check.
	if strings.TrimSpace(claims.Email) == "" {
		return NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": providerName})
	}

	if claims.Sub == "" {
		return NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": providerName})
	}

	return nil
}

// GetUserInfo verifies the ID Token returned by the token endpoint instead
// of calling a REST userinfo endpoint. It checks (via the go-oidc library,
// not hand-rolled cryptography): issuer, audience (ClientID), signature and
// expiry. It additionally checks the nonce (bound to the session that
// started the flow, passed in via ctx) and requires email_verified=true.
func (p *GoogleOAuthProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	slug := p.GetConfig().Slug

	if token.IDToken == "" {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Google-%s] GetUserInfo failed: missing id_token", slug))
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": p.GetName()})
	}

	verifier, err := p.getVerifier(ctx)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Google-%s] GetUserInfo verifier init error: %s", slug, err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": p.GetName()}, err.Error())
	}

	// Verify() checks issuer, audience, signature and expiry using the
	// maintained coreos/go-oidc library; this code never parses or verifies
	// JWT signatures itself. Never log the raw ID Token, only the outcome.
	idToken, err := verifier.Verify(ctx, token.IDToken)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Google-%s] GetUserInfo id_token verification failed: %s", slug, err.Error()))
		return nil, NewOAuthError(i18n.MsgOAuthGetUserErr, map[string]any{"Provider": p.GetName()})
	}

	var claims googleIDTokenClaims
	if err := idToken.Claims(&claims); err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Google-%s] GetUserInfo failed to parse claims: %s", slug, err.Error()))
		return nil, NewOAuthError(i18n.MsgOAuthGetUserErr, map[string]any{"Provider": p.GetName()})
	}

	expectedNonce, _ := ctx.Value(NonceContextKey).(string)
	if validationErr := validateGoogleClaims(claims, expectedNonce, p.GetName()); validationErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-Google-%s] GetUserInfo rejected: %s", slug, validationErr.Error()))
		return nil, validationErr
	}

	logger.LogDebug(ctx, "[OAuth-Google-%s] GetUserInfo success: sub=%s, email_verified=true", slug, claims.Sub)

	return &OAuthUser{
		ProviderUserID: claims.Sub,
		DisplayName:    claims.Name,
		Email:          claims.Email,
		Extra: map[string]any{
			"provider": slug,
		},
	}, nil
}
