package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-contrib/sessions"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeSession is a minimal in-memory implementation of sessions.Session so
// findOrCreateOAuthUser can be exercised without a real HTTP request/gin
// context.
type fakeSession struct {
	values map[interface{}]interface{}
}

func newFakeSession() *fakeSession { return &fakeSession{values: map[interface{}]interface{}{}} }

func (s *fakeSession) ID() string                      { return "test-session" }
func (s *fakeSession) Get(key interface{}) interface{} { return s.values[key] }
func (s *fakeSession) Set(key interface{}, val interface{}) {
	s.values[key] = val
}
func (s *fakeSession) Delete(key interface{})                     { delete(s.values, key) }
func (s *fakeSession) Clear()                                     { s.values = map[interface{}]interface{}{} }
func (s *fakeSession) AddFlash(value interface{}, vars ...string) {}
func (s *fakeSession) Flashes(vars ...string) []interface{}       { return nil }
func (s *fakeSession) Options(_ sessions.Options)                 {}
func (s *fakeSession) Save() error                                { return nil }

func setupOAuthFindOrCreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	previousRegisterEnabled := common.RegisterEnabled
	previousQuotaForNewUser := common.QuotaForNewUser
	common.RegisterEnabled = true
	common.QuotaForNewUser = 0

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CustomOAuthProvider{}, &model.UserOAuthBinding{}, &model.Log{}))

	t.Cleanup(func() {
		common.RegisterEnabled = previousRegisterEnabled
		common.QuotaForNewUser = previousQuotaForNewUser
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

// Regression test for the type-assertion bug this feature surfaced:
// findOrCreateOAuthUser used to type-assert on the concrete
// *oauth.GenericOAuthProvider type, which oauth.GoogleOAuthProvider does not
// satisfy even though it embeds it (it's a distinct concrete type). That
// silently routed new Google users through the built-in-provider branch,
// which has no column to persist a Google identity in, so no
// user_oauth_bindings row was ever created. This test creates a new user via
// a real oauth.GoogleOAuthProvider-shaped ConfigurableOAuthProvider and
// asserts a binding row exists afterward.
func TestFindOrCreateOAuthUser_GoogleNewUser_CreatesBinding(t *testing.T) {
	db := setupOAuthFindOrCreateTestDB(t)

	providerConfig := &model.CustomOAuthProvider{
		Name:                  "Google",
		Slug:                  "google",
		ClientId:              "client-id",
		AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint:         "https://oauth2.googleapis.com/token",
		UserInfoEndpoint:      "https://openidconnect.googleapis.com/v1/userinfo",
		ProviderType:          oauth.GoogleProviderType,
		Enabled:               true,
	}
	require.NoError(t, db.Create(providerConfig).Error)

	googleProvider := oauth.NewGoogleOAuthProvider(providerConfig)
	oauthUser := &oauth.OAuthUser{
		ProviderUserID: "google-sub-12345",
		Email:          "newgoogleuser@example.com",
		DisplayName:    "New Google User",
	}

	session := newFakeSession()
	user, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, session)
	require.NoError(t, err)
	require.NotZero(t, user.Id)
	require.Equal(t, common.RoleCommonUser, user.Role)
	require.Equal(t, common.UserStatusEnabled, user.Status)

	var binding model.UserOAuthBinding
	require.NoError(t, db.Where("user_id = ? AND provider_id = ?", user.Id, providerConfig.Id).First(&binding).Error)
	require.Equal(t, "google-sub-12345", binding.ProviderUserId)

	// Second login with the same Google identity must reuse the same user,
	// not create a duplicate.
	sameUser, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, newFakeSession())
	require.NoError(t, err)
	require.Equal(t, user.Id, sameUser.Id)
}

// Requirement 二: an OAuth email that matches an existing (unlinked) account
// must not silently create a duplicate account.
func TestFindOrCreateOAuthUser_EmailConflict_RefusesToCreateDuplicate(t *testing.T) {
	db := setupOAuthFindOrCreateTestDB(t)

	require.NoError(t, db.Create(&model.User{Username: "existing-user", Password: "hashed", Email: "conflict@example.com", AffCode: "aff-1", Status: common.UserStatusEnabled}).Error)

	providerConfig := &model.CustomOAuthProvider{
		Name:                  "Google",
		Slug:                  "google",
		ClientId:              "client-id",
		AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint:         "https://oauth2.googleapis.com/token",
		UserInfoEndpoint:      "https://openidconnect.googleapis.com/v1/userinfo",
		ProviderType:          oauth.GoogleProviderType,
		Enabled:               true,
	}
	require.NoError(t, db.Create(providerConfig).Error)
	googleProvider := oauth.NewGoogleOAuthProvider(providerConfig)

	oauthUser := &oauth.OAuthUser{
		ProviderUserID: "google-sub-conflict",
		Email:          "conflict@example.com",
	}

	_, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, newFakeSession())
	require.Error(t, err)
	_, isConflict := err.(*OAuthEmailConflictError)
	require.True(t, isConflict, "expected *OAuthEmailConflictError, got %T: %v", err, err)

	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("email = ?", "conflict@example.com").Count(&count).Error)
	require.EqualValues(t, 1, count, "no duplicate account should have been created")
}

// Requirement 二: when registration is disabled, a brand-new Google identity
// must not be able to create an account, but an already-bound Google user
// must still be able to log in.
func TestFindOrCreateOAuthUser_RegistrationDisabled(t *testing.T) {
	db := setupOAuthFindOrCreateTestDB(t)

	providerConfig := &model.CustomOAuthProvider{
		Name:                  "Google",
		Slug:                  "google",
		ClientId:              "client-id",
		AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint:         "https://oauth2.googleapis.com/token",
		UserInfoEndpoint:      "https://openidconnect.googleapis.com/v1/userinfo",
		ProviderType:          oauth.GoogleProviderType,
		Enabled:               true,
	}
	require.NoError(t, db.Create(providerConfig).Error)
	googleProvider := oauth.NewGoogleOAuthProvider(providerConfig)

	// Bind an existing Google identity first, with registration enabled.
	boundUser := &oauth.OAuthUser{ProviderUserID: "already-bound-sub", Email: "bound@example.com"}
	existing, err := findOrCreateOAuthUser(nil, googleProvider, boundUser, newFakeSession())
	require.NoError(t, err)

	// Now disable registration and confirm: new identity is rejected,
	// already-bound identity still logs in.
	common.RegisterEnabled = false

	_, err = findOrCreateOAuthUser(nil, googleProvider, &oauth.OAuthUser{ProviderUserID: "brand-new-sub", Email: "brandnew@example.com"}, newFakeSession())
	require.Error(t, err)
	_, isDisabled := err.(*OAuthRegistrationDisabledError)
	require.True(t, isDisabled, "expected *OAuthRegistrationDisabledError, got %T: %v", err, err)

	loggedIn, err := findOrCreateOAuthUser(nil, googleProvider, boundUser, newFakeSession())
	require.NoError(t, err)
	require.Equal(t, existing.Id, loggedIn.Id)
}
