package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCustomOAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CustomOAuthProvider{}, &model.UserOAuthBinding{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

// This suite covers 三.解绑保护 (unbind protection for the user's sole
// sign-in method): a user must always retain at least one working way to
// sign back in — a password, a built-in OAuth identity, or another custom
// OAuth binding — before a custom OAuth binding (Google or otherwise) can be
// removed.
func TestHasOtherLoginMethod(t *testing.T) {
	db := setupCustomOAuthTestDB(t)

	affCodeSeq := 0
	mustCreateUser := func(t *testing.T, u *model.User) int {
		t.Helper()
		affCodeSeq++
		u.AffCode = fmt.Sprintf("aff-%d", affCodeSeq)
		require.NoError(t, db.Create(u).Error)
		return u.Id
	}

	t.Run("password only, no bindings -> has other method", func(t *testing.T) {
		userId := mustCreateUser(t, &model.User{Username: "pw-user", Password: "hashed", Status: common.UserStatusEnabled})
		ok, err := hasOtherLoginMethod(userId, 999)
		require.NoError(t, err)
		require.True(t, ok, "user with a password should always be unbindable")
	})

	t.Run("no password, built-in GitHub id -> has other method", func(t *testing.T) {
		userId := mustCreateUser(t, &model.User{Username: "gh-user", Password: "", GitHubId: "gh-123", Status: common.UserStatusEnabled})
		ok, err := hasOtherLoginMethod(userId, 999)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("no password, one other custom binding -> has other method", func(t *testing.T) {
		userId := mustCreateUser(t, &model.User{Username: "multi-custom", Password: "", Status: common.UserStatusEnabled})
		require.NoError(t, db.Create(&model.UserOAuthBinding{UserId: userId, ProviderId: 1, ProviderUserId: "sub-1"}).Error)
		require.NoError(t, db.Create(&model.UserOAuthBinding{UserId: userId, ProviderId: 2, ProviderUserId: "sub-2"}).Error)

		ok, err := hasOtherLoginMethod(userId, 2) // about to unbind provider 2
		require.NoError(t, err)
		require.True(t, ok, "provider 1 binding should count as another method")
	})

	t.Run("no password, no built-in id, sole custom binding -> blocked", func(t *testing.T) {
		userId := mustCreateUser(t, &model.User{Username: "google-only", Password: "", Status: common.UserStatusEnabled})
		require.NoError(t, db.Create(&model.UserOAuthBinding{UserId: userId, ProviderId: 1, ProviderUserId: "google-sub"}).Error)

		ok, err := hasOtherLoginMethod(userId, 1)
		require.NoError(t, err)
		require.False(t, ok, "the only Google binding must not be removable")
	})
}
