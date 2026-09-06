package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// sessionCookieMaxAge and sessionCookiePath mirror the defaults previously
// hardcoded in main.go's session store setup; kept here so both the store's
// baseline options and this middleware's per-request override agree.
const (
	sessionCookieMaxAge = 2592000 // 30 days
	sessionCookiePath   = "/"
)

// DynamicSessionCookieOptions makes the session cookie's Secure attribute
// track the real scheme of the incoming request instead of a hardcoded
// value. The backend always terminates plain HTTP (Nginx/other reverse
// proxies terminate TLS in front of it), so a static Secure=true would break
// every deployment and a static Secure=false would ship an insecure cookie
// in production. Each request gets its own gin-contrib/sessions Session
// instance (see sessions.Sessions middleware), so overriding Options() here
// is per-request and race-free — it does not mutate shared store state.
//
// SameSite is set to Lax rather than Strict: Strict cookies are not sent on
// the top-level cross-site GET navigation that Google (and any OAuth
// provider) uses to redirect back to our callback URL, which would drop the
// session carrying oauth_state/oauth_nonce before we can validate them. Lax
// still blocks cross-site POST/embedded requests, so CSRF protection is
// materially unchanged for this app's use of cookies.
func DynamicSessionCookieOptions() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		session.Options(sessions.Options{
			Path:     sessionCookiePath,
			MaxAge:   sessionCookieMaxAge,
			HttpOnly: true,
			Secure:   isRequestHTTPS(c.Request),
			SameSite: http.SameSiteLaxMode,
		})
		c.Next()
	}
}

// isRequestHTTPS detects the real scheme of the client-facing request,
// honoring X-Forwarded-Proto set by a reverse proxy terminating TLS.
func isRequestHTTPS(r *http.Request) bool {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		first := strings.TrimSpace(strings.Split(proto, ",")[0])
		return strings.EqualFold(first, "https")
	}
	if r.TLS != nil {
		return true
	}
	return false
}
