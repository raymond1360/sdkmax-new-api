package oauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/golang-jwt/jwt/v5"
)

const testGoogleClientID = "test-client-id.apps.googleusercontent.com"
const testGoogleKeyID = "test-key-1"

// fakeGoogleIssuer stands in for accounts.google.com in tests: it serves an
// OIDC discovery document and a JWKS endpoint backed by a locally generated
// RSA key, so we can mint ID Tokens with controlled claims without any
// network dependency on real Google infrastructure.
type fakeGoogleIssuer struct {
	server     *httptest.Server
	privateKey *rsa.PrivateKey
}

func newFakeGoogleIssuer(t *testing.T) *fakeGoogleIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	f := &fakeGoogleIssuer{privateKey: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 f.server.URL,
			"authorization_endpoint": f.server.URL + "/o/oauth2/v2/auth",
			"token_endpoint":         f.server.URL + "/token",
			"jwks_uri":               f.server.URL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"use": "sig",
					"alg": "RS256",
					"kid": testGoogleKeyID,
					"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(bigIntToBytes(int64(key.PublicKey.E))),
				},
			},
		})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func bigIntToBytes(v int64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(v))
	// Trim leading zero bytes (standard JWK 'e' encoding, e.g. 65537 -> "AQAB")
	i := 0
	for i < len(buf)-1 && buf[i] == 0 {
		i++
	}
	return buf[i:]
}

func (f *fakeGoogleIssuer) wellKnownURL() string {
	return f.server.URL + "/.well-known/openid-configuration"
}

type testClaims struct {
	Issuer        string `json:"iss,omitempty"`
	Audience      string `json:"aud,omitempty"`
	Subject       string `json:"sub,omitempty"`
	Email         string `json:"email,omitempty"`
	EmailVerified *bool  `json:"email_verified,omitempty"`
	Name          string `json:"name,omitempty"`
	Nonce         string `json:"nonce,omitempty"`
	jwt.RegisteredClaims
}

func boolPtr(b bool) *bool { return &b }

func (f *fakeGoogleIssuer) sign(t *testing.T, claims testClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testGoogleKeyID
	signed, err := token.SignedString(f.privateKey)
	if err != nil {
		t.Fatalf("failed to sign test ID token: %v", err)
	}
	return signed
}

func newTestGoogleProvider(issuer *fakeGoogleIssuer) *GoogleOAuthProvider {
	config := &model.CustomOAuthProvider{
		Id:           1,
		Name:         "Google",
		Slug:         "google",
		ClientId:     testGoogleClientID,
		ProviderType: GoogleProviderType,
		WellKnown:    issuer.wellKnownURL(),
	}
	return NewGoogleOAuthProvider(config)
}

func ctxWithNonce(nonce string) context.Context {
	return context.WithValue(context.Background(), NonceContextKey, nonce)
}

func validClaims(issuer *fakeGoogleIssuer) testClaims {
	now := time.Now()
	return testClaims{
		Issuer:        issuer.server.URL,
		Audience:      testGoogleClientID,
		Subject:       "1234567890",
		Email:         "user@example.com",
		EmailVerified: boolPtr(true),
		Name:          "Test User",
		Nonce:         "expected-nonce",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
}

func TestGoogleOAuthProvider_GetUserInfo_Success(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	idToken := issuer.sign(t, validClaims(issuer))
	user, err := provider.GetUserInfo(ctxWithNonce("expected-nonce"), &OAuthToken{IDToken: idToken})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if user.ProviderUserID != "1234567890" {
		t.Errorf("expected sub 1234567890, got %s", user.ProviderUserID)
	}
	if user.Email != "user@example.com" {
		t.Errorf("expected email user@example.com, got %s", user.Email)
	}
	if user.DisplayName != "Test User" {
		t.Errorf("expected name Test User, got %s", user.DisplayName)
	}
}

func TestGoogleOAuthProvider_GetUserInfo_MissingIDToken(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	_, err := provider.GetUserInfo(ctxWithNonce("expected-nonce"), &OAuthToken{IDToken: ""})
	if err == nil {
		t.Fatal("expected error for missing id_token, got nil")
	}
}

func TestGoogleOAuthProvider_GetUserInfo_WrongIssuer(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	claims := validClaims(issuer)
	claims.Issuer = "https://not-really-google.example"
	idToken := issuer.sign(t, claims)

	_, err := provider.GetUserInfo(ctxWithNonce("expected-nonce"), &OAuthToken{IDToken: idToken})
	if err == nil {
		t.Fatal("expected error for mismatched issuer, got nil")
	}
}

func TestGoogleOAuthProvider_GetUserInfo_WrongAudience(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	claims := validClaims(issuer)
	claims.Audience = "some-other-client-id.apps.googleusercontent.com"
	idToken := issuer.sign(t, claims)

	_, err := provider.GetUserInfo(ctxWithNonce("expected-nonce"), &OAuthToken{IDToken: idToken})
	if err == nil {
		t.Fatal("expected error for mismatched audience, got nil")
	}
}

func TestGoogleOAuthProvider_GetUserInfo_Expired(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	claims := validClaims(issuer)
	past := time.Now().Add(-1 * time.Hour)
	claims.ExpiresAt = jwt.NewNumericDate(past)
	claims.IssuedAt = jwt.NewNumericDate(past.Add(-5 * time.Minute))
	idToken := issuer.sign(t, claims)

	_, err := provider.GetUserInfo(ctxWithNonce("expected-nonce"), &OAuthToken{IDToken: idToken})
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestGoogleOAuthProvider_GetUserInfo_EmailNotVerified(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	claims := validClaims(issuer)
	claims.EmailVerified = boolPtr(false)
	idToken := issuer.sign(t, claims)

	_, err := provider.GetUserInfo(ctxWithNonce("expected-nonce"), &OAuthToken{IDToken: idToken})
	if err == nil {
		t.Fatal("expected error for email_verified=false, got nil")
	}
}

func TestGoogleOAuthProvider_GetUserInfo_NonceMismatch(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	idToken := issuer.sign(t, validClaims(issuer))
	_, err := provider.GetUserInfo(ctxWithNonce("some-other-nonce"), &OAuthToken{IDToken: idToken})
	if err == nil {
		t.Fatal("expected error for nonce mismatch, got nil")
	}
}

func TestGoogleOAuthProvider_GetUserInfo_MissingNonceInContext(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	idToken := issuer.sign(t, validClaims(issuer))
	// No nonce in context at all (simulates a forged/replayed callback that
	// never went through GenerateOAuthCode).
	_, err := provider.GetUserInfo(context.Background(), &OAuthToken{IDToken: idToken})
	if err == nil {
		t.Fatal("expected error when no nonce is present in context, got nil")
	}
}

func TestGoogleOAuthProvider_GetUserInfo_MissingNonceInToken(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	claims := validClaims(issuer)
	claims.Nonce = ""
	idToken := issuer.sign(t, claims)

	_, err := provider.GetUserInfo(ctxWithNonce("expected-nonce"), &OAuthToken{IDToken: idToken})
	if err == nil {
		t.Fatal("expected error when token carries no nonce, got nil")
	}
}

func TestGoogleOAuthProvider_GetUserInfo_EmptySubject(t *testing.T) {
	issuer := newFakeGoogleIssuer(t)
	provider := newTestGoogleProvider(issuer)

	claims := validClaims(issuer)
	claims.Subject = ""
	idToken := issuer.sign(t, claims)

	_, err := provider.GetUserInfo(ctxWithNonce("expected-nonce"), &OAuthToken{IDToken: idToken})
	if err == nil {
		t.Fatal("expected error for empty subject, got nil")
	}
}

func TestGoogleOAuthProvider_Issuer_DefaultsToGoogle(t *testing.T) {
	provider := NewGoogleOAuthProvider(&model.CustomOAuthProvider{ClientId: "x"})
	if got := provider.issuer(); got != GoogleDefaultIssuer {
		t.Errorf("expected default issuer %s, got %s", GoogleDefaultIssuer, got)
	}
}

func TestGoogleOAuthProvider_Issuer_DerivedFromWellKnown(t *testing.T) {
	provider := NewGoogleOAuthProvider(&model.CustomOAuthProvider{
		ClientId:  "x",
		WellKnown: "https://accounts.example.com/.well-known/openid-configuration",
	})
	want := "https://accounts.example.com"
	if got := provider.issuer(); got != want {
		t.Errorf("expected issuer %s, got %s", want, got)
	}
}
