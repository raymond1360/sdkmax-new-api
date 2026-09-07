package common

import "strings"

// NormalizeEmail returns a canonical form of an email address used purely
// for equality comparisons (e.g. "is this email already registered?"). It
// trims surrounding whitespace and lowercases ASCII letters only - it does
// NOT perform full Unicode case folding or any provider-specific alias
// normalization (e.g. Gmail's "dots don't matter" or "+tag" stripping),
// since those rules are provider-specific and would risk merging accounts
// that are legitimately distinct on other providers.
//
// This function never mutates or is used to rewrite a stored user.Email
// value - it only feeds comparisons (e.g. model.IsEmailAlreadyTaken).
func NormalizeEmail(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}
