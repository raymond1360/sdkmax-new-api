package oauth

// contextKey is a private type so keys stored in a context.Context by this
// package cannot collide with keys defined by other packages.
type contextKey string

// NonceContextKey carries the OAuth nonce that was issued for the current
// session into ExchangeToken/GetUserInfo. Providers that verify ID Tokens
// (e.g. Google) read it from ctx to bind the token to the request that
// started the flow, without needing to store per-request state on the
// provider instance itself (provider instances are shared across concurrent
// requests, so any such state would be a race condition).
const NonceContextKey contextKey = "oauth_nonce"
