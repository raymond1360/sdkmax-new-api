package controller

import (
	"fmt"
	"strings"
	"sync"
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

	// Force every query/transaction through a single connection so
	// concurrent goroutines (see TestFindOrCreateOAuthUser_ConcurrentFirstLogin_SameIdentity)
	// queue on Go's database/sql connection pool instead of racing for
	// SQLite's single writer lock, which would otherwise surface as a
	// SQLITE_BUSY "database is locked" error rather than the clean unique-
	// constraint violation the concurrency test is specifically about.
	if sqlDB, sqlErr := db.DB(); sqlErr == nil {
		sqlDB.SetMaxOpenConns(1)
	}

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

func newGoogleProviderForConflictTests(t *testing.T, db *gorm.DB) oauth.Provider {
	t.Helper()
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
	return oauth.NewGoogleOAuthProvider(providerConfig)
}

// P1-01: a conflict must be detected even when the OAuth email only differs
// from the existing account's stored email by ASCII case.
func TestFindOrCreateOAuthUser_EmailConflict_DifferentCase(t *testing.T) {
	db := setupOAuthFindOrCreateTestDB(t)
	require.NoError(t, db.Create(&model.User{Username: "existing-user", Password: "hashed", Email: "Conflict@Example.com", AffCode: "aff-1", Status: common.UserStatusEnabled}).Error)

	googleProvider := newGoogleProviderForConflictTests(t, db)
	oauthUser := &oauth.OAuthUser{ProviderUserID: "google-sub-case", Email: "conflict@example.com"}

	_, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, newFakeSession())
	require.Error(t, err)
	_, isConflict := err.(*OAuthEmailConflictError)
	require.True(t, isConflict, "expected *OAuthEmailConflictError, got %T: %v", err, err)
}

// P1-01: a conflict must be detected even when the OAuth email only differs
// from the existing account's stored email by surrounding whitespace.
func TestFindOrCreateOAuthUser_EmailConflict_LeadingTrailingWhitespace(t *testing.T) {
	db := setupOAuthFindOrCreateTestDB(t)
	require.NoError(t, db.Create(&model.User{Username: "existing-user", Password: "hashed", Email: "conflict@example.com", AffCode: "aff-1", Status: common.UserStatusEnabled}).Error)

	googleProvider := newGoogleProviderForConflictTests(t, db)
	oauthUser := &oauth.OAuthUser{ProviderUserID: "google-sub-ws", Email: "  conflict@example.com  "}

	_, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, newFakeSession())
	require.Error(t, err)
	_, isConflict := err.(*OAuthEmailConflictError)
	require.True(t, isConflict, "expected *OAuthEmailConflictError, got %T: %v", err, err)
}

// P1-01: if historical data already contains more than one row for the
// same normalized email, the conflict check must still fire (this is
// exactly the case the old RowsAffected==1 comparison got wrong).
func TestFindOrCreateOAuthUser_EmailConflict_MultipleExistingRowsSameEmail(t *testing.T) {
	db := setupOAuthFindOrCreateTestDB(t)
	require.NoError(t, db.Create(&model.User{Username: "existing-user-1", Password: "hashed", Email: "dup@example.com", AffCode: "aff-1", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.User{Username: "existing-user-2", Password: "hashed", Email: "dup@example.com", AffCode: "aff-2", Status: common.UserStatusEnabled}).Error)

	googleProvider := newGoogleProviderForConflictTests(t, db)
	oauthUser := &oauth.OAuthUser{ProviderUserID: "google-sub-dup", Email: "dup@example.com"}

	_, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, newFakeSession())
	require.Error(t, err)
	_, isConflict := err.(*OAuthEmailConflictError)
	require.True(t, isConflict, "expected *OAuthEmailConflictError, got %T: %v", err, err)
}

// An already-bound Google identity logging in again must never be affected
// by the email-conflict check, even though its own stored email would
// technically "match itself" - findOrCreateOAuthUser must return via the
// existing-binding path before the conflict check is ever reached.
func TestFindOrCreateOAuthUser_EmailConflict_DoesNotAffectAlreadyBoundUser(t *testing.T) {
	db := setupOAuthFindOrCreateTestDB(t)
	googleProvider := newGoogleProviderForConflictTests(t, db)

	oauthUser := &oauth.OAuthUser{ProviderUserID: "google-sub-selfmatch", Email: "self@example.com"}
	first, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, newFakeSession())
	require.NoError(t, err)

	// Same identity, same email, logging in again - must succeed and return
	// the same user, not an email conflict against its own row.
	second, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, newFakeSession())
	require.NoError(t, err)
	require.Equal(t, first.Id, second.Id)
}

// A brand-new OAuth login must never create a Root/admin account: the new
// user's role must always be the ordinary common-user role.
func TestFindOrCreateOAuthUser_NewUserNeverAutoElevated(t *testing.T) {
	db := setupOAuthFindOrCreateTestDB(t)
	googleProvider := newGoogleProviderForConflictTests(t, db)

	oauthUser := &oauth.OAuthUser{ProviderUserID: "google-sub-role-check", Email: "rolecheck@example.com"}
	user, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, newFakeSession())
	require.NoError(t, err)
	require.Equal(t, common.RoleCommonUser, user.Role)
	require.NotEqual(t, common.RoleRootUser, user.Role)
	require.NotEqual(t, common.RoleAdminUser, user.Role)
}

// P2-04: two concurrent first-time logins for the same brand-new Google
// identity must both succeed and resolve to the same user, instead of one
// winning and the other surfacing a raw database error. The unique index
// on (provider_id, provider_user_id) is what actually prevents a duplicate
// binding; findOrCreateOAuthUser must recover from that race instead of
// propagating it.
func TestFindOrCreateOAuthUser_ConcurrentFirstLogin_SameIdentity(t *testing.T) {
	db := setupOAuthFindOrCreateTestDB(t)
	googleProvider := newGoogleProviderForConflictTests(t, db)

	oauthUser := &oauth.OAuthUser{ProviderUserID: "google-sub-concurrent", Email: "concurrent@example.com", DisplayName: "Concurrent User"}

	const goroutines = 8
	var wg sync.WaitGroup
	userIDs := make([]int, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			u, err := findOrCreateOAuthUser(nil, googleProvider, oauthUser, newFakeSession())
			errs[idx] = err
			if u != nil {
				userIDs[idx] = u.Id
			}
		}(i)
	}
	wg.Wait()

	firstID := 0
	for i := 0; i < goroutines; i++ {
		require.NoError(t, errs[i], "goroutine %d should not surface a raw error from the race", i)
		require.NotZero(t, userIDs[i], "goroutine %d should get a valid user", i)
		if firstID == 0 {
			firstID = userIDs[i]
		} else {
			require.Equal(t, firstID, userIDs[i], "goroutine %d resolved to a different user than the others", i)
		}
	}

	var userCount int64
	require.NoError(t, db.Model(&model.User{}).Where("username LIKE ?", "google_%").Count(&userCount).Error)
	require.EqualValues(t, 1, userCount, "exactly one user row should exist after the race, not one per goroutine")

	var bindingCount int64
	require.NoError(t, db.Model(&model.UserOAuthBinding{}).Where("provider_user_id = ?", "google-sub-concurrent").Count(&bindingCount).Error)
	require.EqualValues(t, 1, bindingCount, "exactly one binding row should exist after the race")
}
