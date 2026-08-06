package model

import "testing"

func TestCalculateOpenRouterSalePrice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		rawPrice        string
		global          float64
		modelMultiplier float64
		want            string
	}{
		{
			name:            "default multiplier keeps raw price",
			rawPrice:        "0.0000015",
			global:          1,
			modelMultiplier: 1,
			want:            "0.0000015000",
		},
		{
			name:            "global and model multiplier are both applied",
			rawPrice:        "0.0000015",
			global:          1.10,
			modelMultiplier: 1.30,
			want:            "0.0000021450",
		},
		{
			name:            "invalid price becomes zero",
			rawPrice:        "not-a-number",
			global:          1,
			modelMultiplier: 1,
			want:            "0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := CalculateOpenRouterSalePrice(tt.rawPrice, tt.global, tt.modelMultiplier); got != tt.want {
				t.Fatalf("CalculateOpenRouterSalePrice() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenRouterModelFillPriceFieldsFromLegacy(t *testing.T) {
	row := OpenRouterModel{
		PromptPrice:     "0.0000015",
		CompletionPrice: "0.0000075",
	}

	row.FillPriceFieldsFromLegacy(1.10, 1.00)

	if row.OpenRouterInputPrice != "0.000001500000" {
		t.Fatalf("OpenRouterInputPrice = %q", row.OpenRouterInputPrice)
	}
	if row.OpenRouterOutputPrice != "0.000007500000" {
		t.Fatalf("OpenRouterOutputPrice = %q", row.OpenRouterOutputPrice)
	}
	if row.SDKMAXInputPrice != "0.0000016500" {
		t.Fatalf("SDKMAXInputPrice = %q", row.SDKMAXInputPrice)
	}
	if row.SDKMAXOutputPrice != "0.0000082500" {
		t.Fatalf("SDKMAXOutputPrice = %q", row.SDKMAXOutputPrice)
	}
}
