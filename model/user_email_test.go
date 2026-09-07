package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// P1-01: IsEmailAlreadyTaken must treat case, surrounding whitespace, and
// (if it somehow exists) more than one pre-existing row for the same
// normalized email all as "taken" - never rely on an exact RowsAffected==1
// match, which silently stops protecting once a second stale/duplicate row
// exists.

func TestIsEmailAlreadyTaken_ExactMatch(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Username: "u1", Password: "x", Email: "person@example.com", Status: 1}).Error)

	require.True(t, IsEmailAlreadyTaken("person@example.com"))
}

func TestIsEmailAlreadyTaken_DifferentCase(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Username: "u1", Password: "x", Email: "Person@Example.com", Status: 1}).Error)

	require.True(t, IsEmailAlreadyTaken("person@example.com"), "lowercase query should match a mixed-case stored email")
	require.True(t, IsEmailAlreadyTaken("PERSON@EXAMPLE.COM"), "uppercase query should match a mixed-case stored email")
}

func TestIsEmailAlreadyTaken_LeadingTrailingWhitespace(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Username: "u1", Password: "x", Email: "person@example.com", Status: 1}).Error)

	require.True(t, IsEmailAlreadyTaken("  person@example.com  "), "query with surrounding whitespace should match")
}

func TestIsEmailAlreadyTaken_StoredValueHasWhitespace(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Username: "u1", Password: "x", Email: " person@example.com ", Status: 1}).Error)

	require.True(t, IsEmailAlreadyTaken("person@example.com"), "a stored email with whitespace should still be found by a clean query")
}

func TestIsEmailAlreadyTaken_EmptyQueryIsNeverTaken(t *testing.T) {
	truncateTables(t)
	require.False(t, IsEmailAlreadyTaken(""))
	require.False(t, IsEmailAlreadyTaken("   "))
}

func TestIsEmailAlreadyTaken_NoMatch(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Username: "u1", Password: "x", Email: "person@example.com", Status: 1}).Error)

	require.False(t, IsEmailAlreadyTaken("someoneelse@example.com"))
}

// Historical data can already contain more than one row for the same
// normalized email (this is exactly the scenario RowsAffected==1 handled
// wrong: it would report "not taken" once a second row existed).
func TestIsEmailAlreadyTaken_MultipleExistingRowsSameEmail(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Username: "u1", Password: "x", Email: "dup@example.com", AffCode: "aff-u1", Status: 1}).Error)
	require.NoError(t, DB.Create(&User{Username: "u2", Password: "x", Email: "dup@example.com", AffCode: "aff-u2", Status: 1}).Error)

	require.True(t, IsEmailAlreadyTaken("dup@example.com"))
}

func TestIsEmailAlreadyTaken_DoesNotMutateStoredEmail(t *testing.T) {
	truncateTables(t)
	original := "  MixedCase@Example.com  "
	require.NoError(t, DB.Create(&User{Username: "u1", Password: "x", Email: original, Status: 1}).Error)

	require.True(t, IsEmailAlreadyTaken("mixedcase@example.com"))

	var stored User
	require.NoError(t, DB.Where("username = ?", "u1").First(&stored).Error)
	require.Equal(t, original, stored.Email, "IsEmailAlreadyTaken must never rewrite an existing user's stored email")
}
