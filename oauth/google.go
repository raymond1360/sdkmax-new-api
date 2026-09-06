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

// GoogleDefaultIssuer is Google's OIDC issuer, used for discovery when the
// provider config does not specify a well-known URL. Tests can point
// WellKnown at a local fake issuer to avoid depending on the network.
const GoogleDefaultIssuer = "https://accounts.google.com"

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

// issuer returns the OIDC issuer to run discovery against. It is derived
// from the configured Well-Known URL so tests (and, in principle, Google
// issuer rotations) can point at a different discovery document; it falls
// back to Google's public issuer.
func (p *GoogleOAuthProvider) issuer() string {
	wellKnown := strings.TrimSpace(p.GetConfig().WellKnown)
	if wellKnown == "" {
		return GoogleDefaultIssuer
	}
	issuer := strings.TrimSuffix(wellKnown, "/.well-known/openid-configuration")
	issuer = strings.TrimSuffix(issuer, "/")
	if issuer == "" {
		return GoogleDefaultIssuer
	}
	return issuer
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
	if expectedNonce == "" || claims.Nonce == "" || claims.Nonce != expectedNonce {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-Google-%s] GetUserInfo rejected: nonce mismatch", slug))
		return nil, NewOAuthError(i18n.MsgOAuthStateInvalid, nil)
	}

	if !claims.EmailVerified {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-Google-%s] GetUserInfo rejected: email_verified=false", slug))
		return nil, NewOAuthError(i18n.MsgOAuthEmailNotVerified, map[string]any{"Provider": p.GetName()})
	}

	if claims.Sub == "" {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Google-%s] GetUserInfo failed: empty sub", slug))
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": p.GetName()})
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
