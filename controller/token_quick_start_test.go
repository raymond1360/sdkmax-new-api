package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestAddTokenReturnsMaskedTokenMetadata(t *testing.T) {
	setupTokenControllerTestDB(t)

	body := map[string]any{
		"name":                 "quick-start-token",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "default",
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", body, 1)

	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got %s", recorder.Body.String())
	}

	var token tokenResponseItem
	if err := common.Unmarshal(response.Data, &token); err != nil {
		t.Fatalf("failed to decode created token data: %v", err)
	}
	if token.ID == 0 {
		t.Fatal("expected created token id in response")
	}
	if token.Name != "quick-start-token" {
		t.Fatalf("expected token name quick-start-token, got %q", token.Name)
	}
	if token.Key == "" || !strings.Contains(token.Key, "*") {
		t.Fatalf("expected masked token key in response, got %q", token.Key)
	}
	if strings.Contains(recorder.Body.String(), "sk-") {
		t.Fatalf("create response should not include sk-prefixed full key: %s", recorder.Body.String())
	}
}
