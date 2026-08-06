package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestBuildOpenRouterBillingExpr(t *testing.T) {
	t.Parallel()

	got := buildOpenRouterBillingExpr("0.0000015", "0.0000075")
	want := `tier("openrouter", p * 1.5000000000 + c * 7.5000000000)`
	if got != want {
		t.Fatalf("buildOpenRouterBillingExpr() = %q, want %q", got, want)
	}
}

func TestRemoveOpenRouterManualPricingOptions(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{
		"ModelPrice":           `{"openai/gpt-4o":1.23,"keep-me":4.56}`,
		"ModelRatio":           `{"openai/gpt-4o":2,"keep-me":3}`,
		"CompletionRatio":      `{}`,
		"CacheRatio":           `{}`,
		"CreateCacheRatio":     `{}`,
		"ImageRatio":           `{}`,
		"AudioRatio":           `{}`,
		"AudioCompletionRatio": `{}`,
	}
	common.OptionMapRWMutex.Unlock()

	updates, err := RemoveOpenRouterManualPricingOptions([]string{"openai/gpt-4o"})
	if err != nil {
		t.Fatalf("RemoveOpenRouterManualPricingOptions() error = %v", err)
	}
	var priceMap map[string]float64
	if err := common.Unmarshal([]byte(updates["ModelPrice"]), &priceMap); err != nil {
		t.Fatalf("ModelPrice update is invalid JSON: %v", err)
	}
	if _, ok := priceMap["openai/gpt-4o"]; ok || priceMap["keep-me"] != 4.56 {
		t.Fatalf("ModelPrice update = %v", priceMap)
	}
	var ratioMap map[string]float64
	if err := common.Unmarshal([]byte(updates["ModelRatio"]), &ratioMap); err != nil {
		t.Fatalf("ModelRatio update is invalid JSON: %v", err)
	}
	if _, ok := ratioMap["openai/gpt-4o"]; ok || ratioMap["keep-me"] != 3 {
		t.Fatalf("ModelRatio update = %v", ratioMap)
	}
	if _, ok := updates["CompletionRatio"]; ok {
		t.Fatalf("CompletionRatio should not be rewritten when unchanged")
	}
}

func TestUnsafeOpenRouterFetchCountGuard(t *testing.T) {
	t.Parallel()

	if !isUnsafeOpenRouterFetchCount(270, 340) {
		t.Fatalf("expected 270/340 to trip the 80%% drop guard")
	}
	if isUnsafeOpenRouterFetchCount(300, 340) {
		t.Fatalf("expected 300/340 to pass the 80%% drop guard")
	}
	if isUnsafeOpenRouterFetchCount(10, 20) {
		t.Fatalf("small baselines should not trip the drop guard")
	}
}
