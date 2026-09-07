package oauth

import (
	"strings"
	"testing"
)

func TestRedactTokenResponseForLog_MasksSensitiveFields(t *testing.T) {
	body := `{"access_token":"secret-access","id_token":"secret-id","refresh_token":"secret-refresh","token_type":"Bearer","scope":"openid email"}`
	redacted := redactTokenResponseForLog(body)

	for _, secret := range []string{"secret-access", "secret-id", "secret-refresh"} {
		if strings.Contains(redacted, secret) {
			t.Errorf("redacted log still contains sensitive value %q: %s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "Bearer") || !strings.Contains(redacted, "openid email") {
		t.Errorf("redaction should preserve non-sensitive fields: %s", redacted)
	}
}

func TestRedactTokenResponseForLog_NonJSONBody(t *testing.T) {
	redacted := redactTokenResponseForLog("not json at all")
	if strings.Contains(redacted, "not json") {
		t.Errorf("non-JSON body should be omitted, not echoed back: %s", redacted)
	}
}
