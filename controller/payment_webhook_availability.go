package controller

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func isPaymentComplianceConfirmed() bool {
	return operation_setting.IsPaymentComplianceConfirmed()
}

func isStripeTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return validateStripeRuntimeConfig(true) == nil
}

func isStripeWebhookConfigured() bool {
	return strings.TrimSpace(setting.StripeWebhookSecret) != ""
}

func isStripeWebhookEnabled() bool {
	return isStripeTopUpEnabled()
}

func normalizedStripeMode() string {
	mode := strings.ToLower(strings.TrimSpace(setting.StripeMode))
	if mode != "live" {
		return "test"
	}
	return "live"
}

func expectedStripeLivemode() bool {
	return normalizedStripeMode() == "live"
}

func validateStripeRuntimeConfig(requireWebhookSecret bool) error {
	key := strings.TrimSpace(setting.StripeApiSecret)
	if key == "" {
		return fmt.Errorf("stripe api key is not configured")
	}
	mode := normalizedStripeMode()
	switch mode {
	case "test":
		if !strings.HasPrefix(key, "sk_test_") {
			return fmt.Errorf("stripe test mode requires sk_test key")
		}
	case "live":
		if !strings.HasPrefix(key, "sk_live_") {
			return fmt.Errorf("stripe live mode requires sk_live key")
		}
	}
	if requireWebhookSecret {
		webhookSecret := strings.TrimSpace(setting.StripeWebhookSecret)
		if webhookSecret == "" {
			return fmt.Errorf("stripe webhook secret is not configured")
		}
		if !strings.HasPrefix(webhookSecret, "whsec_") {
			return fmt.Errorf("stripe webhook secret must start with whsec_")
		}
	}
	if setting.StripeUnitPrice <= 0 {
		return fmt.Errorf("stripe unit price must be positive")
	}
	if !model.IsStripeTopUpSchemaReady() {
		return fmt.Errorf("stripe database migration is not ready: %s", model.StripeTopUpSchemaReadinessInfo())
	}
	return nil
}

func isCreemTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	products := strings.TrimSpace(setting.CreemProducts)
	return strings.TrimSpace(setting.CreemApiKey) != "" &&
		products != "" &&
		products != "[]"
}

func isCreemWebhookConfigured() bool {
	return strings.TrimSpace(setting.CreemWebhookSecret) != ""
}

func isCreemWebhookEnabled() bool {
	return isCreemTopUpEnabled() && isCreemWebhookConfigured()
}

func isWaffoTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	if !setting.WaffoEnabled {
		return false
	}

	return isWaffoWebhookConfigured()
}

func isWaffoWebhookConfigured() bool {
	if setting.WaffoSandbox {
		return strings.TrimSpace(setting.WaffoSandboxApiKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPrivateKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPublicCert) != ""
	}

	return strings.TrimSpace(setting.WaffoApiKey) != "" &&
		strings.TrimSpace(setting.WaffoPrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPublicCert) != ""
}

func isWaffoWebhookEnabled() bool {
	return isWaffoTopUpEnabled()
}

func isWaffoPancakeTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	// Presence-of-credentials = enabled. Webhook public keys ship inside
	// the SDK; mode (test/prod) is read from each event.
	return strings.TrimSpace(setting.WaffoPancakeMerchantID) != "" &&
		strings.TrimSpace(setting.WaffoPancakePrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPancakeProductID) != ""
}

func isWaffoPancakeWebhookConfigured() bool {
	return isWaffoPancakeTopUpEnabled()
}

func isWaffoPancakeWebhookEnabled() bool {
	return isWaffoPancakeTopUpEnabled()
}

func isEpayTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return isEpayWebhookConfigured() && len(operation_setting.PayMethods) > 0
}

func isEpayWebhookConfigured() bool {
	return strings.TrimSpace(operation_setting.PayAddress) != "" &&
		strings.TrimSpace(operation_setting.EpayId) != "" &&
		strings.TrimSpace(operation_setting.EpayKey) != ""
}

func isEpayWebhookEnabled() bool {
	return isEpayTopUpEnabled()
}
