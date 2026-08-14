package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func baseAvailability() ModelAvailability {
	return ModelAvailability{
		ModelID:              "openai/example",
		AdminEnabled:         true,
		ConnectivityStatus:   ConnectivityConnected,
		SDKMAXSupportStatus:  SupportSupported,
		APIMode:              APIModeRealtime,
		CustomerVisible:      true,
		VisibilityReason:     VisibilityVisible,
		ConsecutiveSuccesses: 2,
	}
}

func TestModelAvailabilityNewOpenRouterModelDefaultsHidden(t *testing.T) {
	upstream := ModelAvailability{
		ModelID:             "openai/new-model",
		AdminEnabled:        true,
		ConnectivityStatus:  ConnectivityUntested,
		SDKMAXSupportStatus: SupportSupported,
		APIMode:             APIModeRealtime,
	}

	visible, reason := ResolveModelVisibility(upstream)

	if visible {
		t.Fatal("new untested realtime model should be hidden")
	}
	if reason != VisibilityNewModelUntested {
		t.Fatalf("reason = %q, want %q", reason, VisibilityNewModelUntested)
	}
}

func TestModelAvailabilityRealtimeFailureThreshold(t *testing.T) {
	current := baseAvailability()

	first := ApplyHealthResultToAvailability(current, ModelHealthCheckResult{ConnectivityStatus: ConnectivityUpstreamError, FailureReason: "UPSTREAM_ERROR"})
	if !first.CustomerVisible || first.ConnectivityStatus != ConnectivityDegraded {
		t.Fatalf("first failure should stay visible and degraded: %+v", first)
	}
	second := ApplyHealthResultToAvailability(first, ModelHealthCheckResult{ConnectivityStatus: ConnectivityTimeout, FailureReason: "TIMEOUT"})
	if !second.CustomerVisible || second.ConnectivityStatus != ConnectivityDegraded {
		t.Fatalf("second failure should stay visible and degraded: %+v", second)
	}
	third := ApplyHealthResultToAvailability(second, ModelHealthCheckResult{ConnectivityStatus: ConnectivityFailed, FailureReason: "UPSTREAM_ERROR"})
	if third.CustomerVisible || third.ConnectivityStatus != ConnectivityFailed || third.VisibilityReason != VisibilityConnectivityFailed {
		t.Fatalf("third failure should hide model: %+v", third)
	}
}

func TestModelAvailabilitySpecialProtocolsHidden(t *testing.T) {
	cases := []struct {
		name   string
		row    ModelAvailability
		reason string
	}{
		{
			name: "batch unsupported",
			row: ModelAvailability{
				AdminEnabled:        true,
				ConnectivityStatus:  ConnectivityBatchRequired,
				SDKMAXSupportStatus: SupportUnsupported,
				APIMode:             APIModeBatch,
			},
			reason: VisibilityBatchNotSupported,
		},
		{
			name: "image unverified",
			row: ModelAvailability{
				AdminEnabled:        true,
				ConnectivityStatus:  ConnectivityUntested,
				SDKMAXSupportStatus: SupportUnverified,
				APIMode:             APIModeImage,
			},
			reason: VisibilitySdkmaxUnverified,
		},
		{
			name: "wrong endpoint",
			row: ModelAvailability{
				AdminEnabled:        true,
				ConnectivityStatus:  ConnectivityWrongEndpoint,
				SDKMAXSupportStatus: SupportSupported,
				APIMode:             APIModeRealtime,
			},
			reason: VisibilityWrongEndpoint,
		},
		{
			name: "wrong payload",
			row: ModelAvailability{
				AdminEnabled:        true,
				ConnectivityStatus:  ConnectivityWrongPayload,
				SDKMAXSupportStatus: SupportSupported,
				APIMode:             APIModeRealtime,
			},
			reason: VisibilityWrongPayload,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			visible, reason := ResolveModelVisibility(tt.row)
			if visible || reason != tt.reason {
				t.Fatalf("ResolveModelVisibility() = (%v, %q), want (false, %q)", visible, reason, tt.reason)
			}
		})
	}
}

func TestModelAvailabilityRecoveryNeedsTwoSuccesses(t *testing.T) {
	current := baseAvailability()
	current.CustomerVisible = false
	current.ConnectivityStatus = ConnectivityFailed
	current.ConsecutiveFailures = 3
	current.ConsecutiveSuccesses = 0

	first := ApplyHealthResultToAvailability(current, ModelHealthCheckResult{Success: true})
	if first.CustomerVisible || first.VisibilityReason != VisibilityRecoveryPending {
		t.Fatalf("first recovery success should stay hidden: %+v", first)
	}
	second := ApplyHealthResultToAvailability(first, ModelHealthCheckResult{Success: true})
	if !second.CustomerVisible || second.VisibilityReason != VisibilityVisible {
		t.Fatalf("second recovery success should show model: %+v", second)
	}
}

func TestModelAvailabilityAdminDisabledWins(t *testing.T) {
	current := baseAvailability()
	current.AdminEnabled = false
	current.CustomerVisible = false

	next := ApplyHealthResultToAvailability(current, ModelHealthCheckResult{Success: true})
	if next.CustomerVisible || next.VisibilityReason != VisibilityAdminDisabled {
		t.Fatalf("admin disabled model must not auto recover: %+v", next)
	}
}

func TestModelAvailabilityRateLimitAndNoProviderStayDegraded(t *testing.T) {
	for _, status := range []string{ConnectivityRateLimited, ConnectivityNoProvider} {
		current := baseAvailability()
		next := ApplyHealthResultToAvailability(current, ModelHealthCheckResult{ConnectivityStatus: status, FailureReason: status, HTTPStatus: 429})
		if !next.CustomerVisible || next.ConnectivityStatus != ConnectivityDegraded {
			t.Fatalf("%s should stay visible and degraded: %+v", status, next)
		}
	}
}

func TestModelAutoVisibilityDefaultEnabled(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	old := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = old
		common.OptionMapRWMutex.Unlock()
	})

	if !ModelAutoVisibilityEnabled() {
		t.Fatal("auto visibility should default to enabled")
	}
}
