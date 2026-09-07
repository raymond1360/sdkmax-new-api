package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
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

// trustedProtoProxyCIDRs lists the CIDR ranges whose immediate TCP peer
// address is trusted to set X-Forwarded-Proto. That address comes from
// http.Request.RemoteAddr, which Go's own listener records from the raw TCP
// connection before any application or proxy code runs - unlike header
// values, it cannot be set by a client. Defaults to loopback only: SDKMAX's
// documented production topology (docs/deployment) runs Nginx on the same
// host, reverse-proxying to 127.0.0.1, with Nginx terminating TLS. Override
// via the TRUSTED_PROXY_CIDRS env var (comma-separated CIDRs or bare IPs)
// only if the reverse proxy reaches this process over a different, still
// non-public, network path (e.g. a private container network).
var trustedProtoProxyCIDRs = parseTrustedProxyCIDRs(common.GetEnvOrDefaultString("TRUSTED_PROXY_CIDRS", "127.0.0.1/32,::1/128"))

func parseTrustedProxyCIDRs(raw string) []*net.IPNet {
	var nets []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ipNet, err := net.ParseCIDR(part); err == nil {
			nets = append(nets, ipNet)
			continue
		}
		if ip := net.ParseIP(part); ip != nil {
			nets = append(nets, singleIPNet(ip))
		}
	}
	return nets
}

func singleIPNet(ip net.IP) *net.IPNet {
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
}

// isTrustedProxyPeer reports whether remoteAddr (http.Request.RemoteAddr)
// falls within trustedProtoProxyCIDRs.
func isTrustedProxyPeer(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, ipNet := range trustedProtoProxyCIDRs {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// isRequestHTTPS detects the real scheme of the client-facing request. It
// only honors X-Forwarded-Proto when the immediate TCP peer is a trusted
// reverse proxy (see isTrustedProxyPeer). X-Forwarded-Proto is an ordinary
// HTTP header any client can set; trusting it unconditionally would let a
// request reaching this process directly (bypassing Nginx, e.g. if the
// deployment's firewall does not actually block the app's port - see
// docs/deployment) claim to be HTTPS and receive a Secure session cookie
// over a connection that was never encrypted. When the peer is not
// trusted, this falls back to r.TLS != nil, true only if this process
// terminated TLS itself, which it does not in the documented production
// deployment (Nginx does).
func isRequestHTTPS(r *http.Request) bool {
	if isTrustedProxyPeer(r.RemoteAddr) {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			first := strings.TrimSpace(strings.Split(proto, ",")[0])
			return strings.EqualFold(first, "https")
		}
	}
	if r.TLS != nil {
		return true
	}
	return false
}
