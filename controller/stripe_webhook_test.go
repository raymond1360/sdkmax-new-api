package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
	"gorm.io/gorm"
)

func ginCreateTestContext(w *httptest.ResponseRecorder, method string, target string, body *bytes.Reader) (*gin.Context, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	c, r := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c, r
}

func setupStripeControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	originalRedis := common.RedisEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.CheckStripeTopUpSchemaReadiness(db))
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		common.RedisEnabled = originalRedis
		model.SetStripeTopUpSchemaReadinessForTest(false, "not checked")
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func setStripeSchemaReadyForTest(t *testing.T) {
	t.Helper()
	model.SetStripeTopUpSchemaReadinessForTest(true, "ready")
	t.Cleanup(func() { model.SetStripeTopUpSchemaReadinessForTest(false, "not checked") })
}

func stripeSessionEventPayload(t *testing.T, overrides map[string]any) []byte {
	t.Helper()
	return stripeSessionEventPayloadForType(t, string(stripe.EventTypeCheckoutSessionCompleted), false, overrides)
}

func stripeSessionEventPayloadForType(t *testing.T, eventType string, livemode bool, overrides map[string]any) []byte {
	t.Helper()
	obj := map[string]any{
		"id":                  "cs_test_safe",
		"object":              "checkout.session",
		"client_reference_id": "ref_safe",
		"metadata": map[string]string{
			"trade_no": "ref_safe",
		},
		"mode":           "payment",
		"status":         "complete",
		"payment_status": "paid",
		"amount_total":   1000,
		"currency":       "usd",
		"livemode":       livemode,
		"payment_intent": "pi_test_safe",
		"customer":       "cus_test_safe",
	}
	for k, v := range overrides {
		obj[k] = v
	}
	payload := map[string]any{
		"id":       "evt_test_safe",
		"object":   "event",
		"type":     eventType,
		"livemode": livemode,
		"data": map[string]any{
			"object": obj,
		},
	}
	data, err := common.Marshal(payload)
	require.NoError(t, err)
	return data
}

func signedStripeWebhookRequest(t *testing.T, payload []byte, secret string) (*bytes.Reader, string) {
	t.Helper()
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload:   payload,
		Secret:    secret,
		Timestamp: time.Now(),
	})
	return bytes.NewReader(signed.Payload), signed.Header
}

func TestStripeWebhookRejectsWhenSecretEmpty(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalKey := setting.StripeApiSecret
	originalSecret := setting.StripeWebhookSecret
	originalMode := setting.StripeMode
	originalUnitPrice := setting.StripeUnitPrice
	t.Cleanup(func() {
		setting.StripeApiSecret = originalKey
		setting.StripeWebhookSecret = originalSecret
		setting.StripeMode = originalMode
		setting.StripeUnitPrice = originalUnitPrice
	})

	setting.StripeApiSecret = "sk_test_local"
	setting.StripeWebhookSecret = ""
	setting.StripeMode = "test"
	setting.StripeUnitPrice = 1

	w := httptest.NewRecorder()
	c, _ := ginCreateTestContext(w, "POST", "/api/stripe/webhook", bytes.NewReader(stripeSessionEventPayload(t, nil)))
	StripeWebhook(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestStripeWebhookRejectsBadSignature(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	setStripeSchemaReadyForTest(t)
	originalKey := setting.StripeApiSecret
	originalSecret := setting.StripeWebhookSecret
	originalMode := setting.StripeMode
	originalUnitPrice := setting.StripeUnitPrice
	t.Cleanup(func() {
		setting.StripeApiSecret = originalKey
		setting.StripeWebhookSecret = originalSecret
		setting.StripeMode = originalMode
		setting.StripeUnitPrice = originalUnitPrice
	})

	setting.StripeApiSecret = "sk_test_local"
	setting.StripeWebhookSecret = "whsec_local"
	setting.StripeMode = "test"
	setting.StripeUnitPrice = 1

	w := httptest.NewRecorder()
	c, _ := ginCreateTestContext(w, "POST", "/api/stripe/webhook", bytes.NewReader(stripeSessionEventPayload(t, nil)))
	c.Request.Header.Set("Stripe-Signature", "t=1,v1=bad")
	StripeWebhook(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestStripeWebhookRejectsBodyLargerThanOneMB(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	setStripeSchemaReadyForTest(t)
	originalKey := setting.StripeApiSecret
	originalSecret := setting.StripeWebhookSecret
	originalMode := setting.StripeMode
	originalUnitPrice := setting.StripeUnitPrice
	t.Cleanup(func() {
		setting.StripeApiSecret = originalKey
		setting.StripeWebhookSecret = originalSecret
		setting.StripeMode = originalMode
		setting.StripeUnitPrice = originalUnitPrice
	})

	setting.StripeApiSecret = "sk_test_local"
	setting.StripeWebhookSecret = "whsec_local"
	setting.StripeMode = "test"
	setting.StripeUnitPrice = 1

	oversized := bytes.NewReader(bytes.Repeat([]byte("x"), stripeWebhookMaxBodyBytes+1))
	w := httptest.NewRecorder()
	c, _ := ginCreateTestContext(w, "POST", "/api/stripe/webhook", oversized)
	c.Request.Header.Set("Stripe-Signature", "t=1,v1=bad")
	StripeWebhook(c)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestValidateStripeCheckoutSession(t *testing.T) {
	originalMode := setting.StripeMode
	t.Cleanup(func() { setting.StripeMode = originalMode })
	setting.StripeMode = "test"

	payload := stripeSessionEventPayload(t, nil)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload:   payload,
		Secret:    "whsec_local",
		Timestamp: time.Now(),
	})
	event, err := webhook.ConstructEventWithOptions(signed.Payload, signed.Header, "whsec_local", webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	require.NoError(t, err)
	event.Type = stripe.EventTypeCheckoutSessionCompleted

	validated, err := validateStripeCheckoutSession(event, payload, true)
	require.NoError(t, err)
	assert.Equal(t, "ref_safe", validated.TradeNo)
	assert.Equal(t, "cs_test_safe", validated.SessionID)
	assert.Equal(t, "USD", validated.Currency)
	assert.EqualValues(t, 1000, validated.AmountTotal)

	for name, override := range map[string]map[string]any{
		"currency":       {"currency": "cny"},
		"amount":         {"amount_total": 0},
		"payment_status": {"payment_status": "unpaid"},
		"mode":           {"mode": "subscription"},
		"status":         {"status": "open"},
		"livemode":       {"livemode": true},
		"client_ref":     {"client_reference_id": "ref_other"},
		"metadata":       {"metadata": map[string]string{"trade_no": "ref_other"}},
	} {
		t.Run(fmt.Sprintf("rejects_%s", name), func(t *testing.T) {
			badPayload := stripeSessionEventPayload(t, override)
			badSigned := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
				Payload:   badPayload,
				Secret:    "whsec_local",
				Timestamp: time.Now(),
			})
			badEvent, err := webhook.ConstructEventWithOptions(badSigned.Payload, badSigned.Header, "whsec_local", webhook.ConstructEventOptions{
				IgnoreAPIVersionMismatch: true,
			})
			require.NoError(t, err)
			badEvent.Type = stripe.EventTypeCheckoutSessionCompleted
			_, err = validateStripeCheckoutSession(badEvent, badPayload, true)
			require.Error(t, err)
		})
	}
}

func TestStripeWebhookReturnsServiceUnavailableForTransientUpdateFailure(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	setStripeSchemaReadyForTest(t)
	originalKey := setting.StripeApiSecret
	originalSecret := setting.StripeWebhookSecret
	originalMode := setting.StripeMode
	originalUnitPrice := setting.StripeUnitPrice
	originalMark := markStripeTopUpFailed
	t.Cleanup(func() {
		setting.StripeApiSecret = originalKey
		setting.StripeWebhookSecret = originalSecret
		setting.StripeMode = originalMode
		setting.StripeUnitPrice = originalUnitPrice
		markStripeTopUpFailed = originalMark
	})

	setting.StripeApiSecret = "sk_test_local"
	setting.StripeWebhookSecret = "whsec_local"
	setting.StripeMode = "test"
	setting.StripeUnitPrice = 1
	markStripeTopUpFailed = func(tradeNo string, sessionID string, eventID string, livemode bool, targetStatus string, reason string) error {
		return errors.New("temporary database outage")
	}

	payload := stripeSessionEventPayloadForType(t, string(stripe.EventTypeCheckoutSessionExpired), false, map[string]any{"status": "expired"})
	body, signature := signedStripeWebhookRequest(t, payload, "whsec_local")
	w := httptest.NewRecorder()
	c, _ := ginCreateTestContext(w, "POST", "/api/stripe/webhook", body)
	c.Request.Header.Set("Stripe-Signature", signature)
	StripeWebhook(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestStripeWebhookReturnsOKForPermanentBusinessReject(t *testing.T) {
	setupStripeControllerTestDB(t)
	confirmPaymentComplianceForTest(t)
	originalKey := setting.StripeApiSecret
	originalSecret := setting.StripeWebhookSecret
	originalMode := setting.StripeMode
	originalUnitPrice := setting.StripeUnitPrice
	t.Cleanup(func() {
		setting.StripeApiSecret = originalKey
		setting.StripeWebhookSecret = originalSecret
		setting.StripeMode = originalMode
		setting.StripeUnitPrice = originalUnitPrice
	})

	setting.StripeApiSecret = "sk_test_local"
	setting.StripeWebhookSecret = "whsec_local"
	setting.StripeMode = "test"
	setting.StripeUnitPrice = 1

	payload := stripeSessionEventPayload(t, nil)
	body, signature := signedStripeWebhookRequest(t, payload, "whsec_local")
	w := httptest.NewRecorder()
	c, _ := ginCreateTestContext(w, "POST", "/api/stripe/webhook", body)
	c.Request.Header.Set("Stripe-Signature", signature)
	StripeWebhook(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBindStripeSessionWithRecoveryRetriesThenSucceeds(t *testing.T) {
	originalAttach := attachStripeSessionToTopUp
	originalRecord := recordStripeSessionRecovery
	originalExpire := expireStripeCheckoutSession
	t.Cleanup(func() {
		attachStripeSessionToTopUp = originalAttach
		recordStripeSessionRecovery = originalRecord
		expireStripeCheckoutSession = originalExpire
	})

	attempts := 0
	attachStripeSessionToTopUp = func(tradeNo string, sessionID string) error {
		attempts++
		if attempts < stripeBindRetryAttempts {
			return errors.New("temporary database outage")
		}
		return nil
	}
	recordStripeSessionRecovery = func(tradeNo string, sessionID string, reason string) error {
		t.Fatalf("recovery record should not be called after retry success")
		return nil
	}
	expireStripeCheckoutSession = func(id string, params *stripe.CheckoutSessionExpireParams) (*stripe.CheckoutSession, error) {
		t.Fatalf("expire should not be called after retry success")
		return nil, nil
	}

	require.NoError(t, bindStripeSessionWithRecovery(context.Background(), "ref_retry", "cs_test_retry", false))
	assert.Equal(t, stripeBindRetryAttempts, attempts)
}

func TestBindStripeSessionWithRecoveryExpiresOnlyAfterConfirmedExpire(t *testing.T) {
	originalAttach := attachStripeSessionToTopUp
	originalRecord := recordStripeSessionRecovery
	originalExpire := expireStripeCheckoutSession
	originalMark := markStripeTopUpFailed
	t.Cleanup(func() {
		attachStripeSessionToTopUp = originalAttach
		recordStripeSessionRecovery = originalRecord
		expireStripeCheckoutSession = originalExpire
		markStripeTopUpFailed = originalMark
	})

	recoverySessionID := ""
	markedSessionID := ""
	attachStripeSessionToTopUp = func(tradeNo string, sessionID string) error {
		return errors.New("temporary database outage")
	}
	recordStripeSessionRecovery = func(tradeNo string, sessionID string, reason string) error {
		recoverySessionID = sessionID
		return nil
	}
	expireStripeCheckoutSession = func(id string, params *stripe.CheckoutSessionExpireParams) (*stripe.CheckoutSession, error) {
		return &stripe.CheckoutSession{Status: stripe.CheckoutSessionStatusExpired}, nil
	}
	markStripeTopUpFailed = func(tradeNo string, sessionID string, eventID string, livemode bool, targetStatus string, reason string) error {
		markedSessionID = sessionID
		assert.Equal(t, common.TopUpStatusExpired, targetStatus)
		return nil
	}

	err := bindStripeSessionWithRecovery(context.Background(), "ref_expire", "cs_test_expire", false)
	require.Error(t, err)
	assert.Equal(t, "cs_test_expire", recoverySessionID)
	assert.Equal(t, "cs_test_expire", markedSessionID)
}

func TestBindStripeSessionWithRecoveryKeepsRecoverableWhenExpireFails(t *testing.T) {
	originalAttach := attachStripeSessionToTopUp
	originalRecord := recordStripeSessionRecovery
	originalExpire := expireStripeCheckoutSession
	originalMark := markStripeTopUpFailed
	t.Cleanup(func() {
		attachStripeSessionToTopUp = originalAttach
		recordStripeSessionRecovery = originalRecord
		expireStripeCheckoutSession = originalExpire
		markStripeTopUpFailed = originalMark
	})

	recoverySessionID := ""
	attachStripeSessionToTopUp = func(tradeNo string, sessionID string) error {
		return errors.New("temporary database outage")
	}
	recordStripeSessionRecovery = func(tradeNo string, sessionID string, reason string) error {
		recoverySessionID = sessionID
		return nil
	}
	expireStripeCheckoutSession = func(id string, params *stripe.CheckoutSessionExpireParams) (*stripe.CheckoutSession, error) {
		return nil, errors.New("stripe api unavailable")
	}
	markStripeTopUpFailed = func(tradeNo string, sessionID string, eventID string, livemode bool, targetStatus string, reason string) error {
		t.Fatalf("local order must not be terminal when expire is uncertain")
		return nil
	}

	err := bindStripeSessionWithRecovery(context.Background(), "ref_recover", "cs_test_recover", false)
	require.Error(t, err)
	assert.Equal(t, "cs_test_recover", recoverySessionID)
}

func TestBindStripeSessionWithRecoveryRecordFirstFailsThenSucceeds(t *testing.T) {
	originalAttach := attachStripeSessionToTopUp
	originalRecord := recordStripeSessionRecovery
	originalExpire := expireStripeCheckoutSession
	originalMark := markStripeTopUpFailed
	t.Cleanup(func() {
		attachStripeSessionToTopUp = originalAttach
		recordStripeSessionRecovery = originalRecord
		expireStripeCheckoutSession = originalExpire
		markStripeTopUpFailed = originalMark
	})

	recordAttempts := 0
	attachStripeSessionToTopUp = func(tradeNo string, sessionID string) error {
		return errors.New("database unavailable")
	}
	recordStripeSessionRecovery = func(tradeNo string, sessionID string, reason string) error {
		recordAttempts++
		if recordAttempts == 1 {
			return errors.New("first recovery save failed")
		}
		return nil
	}
	expireStripeCheckoutSession = func(id string, params *stripe.CheckoutSessionExpireParams) (*stripe.CheckoutSession, error) {
		return &stripe.CheckoutSession{Status: stripe.CheckoutSessionStatusExpired}, nil
	}
	markStripeTopUpFailed = func(tradeNo string, sessionID string, eventID string, livemode bool, targetStatus string, reason string) error {
		return nil
	}

	err := bindStripeSessionWithRecovery(context.Background(), "ref_recovery_retry", "cs_test_retry_recovery", false)
	require.Error(t, err)
	assert.Equal(t, 2, recordAttempts)
}

func TestBindStripeSessionWithRecoveryRecordKeepsFailing(t *testing.T) {
	originalAttach := attachStripeSessionToTopUp
	originalRecord := recordStripeSessionRecovery
	originalExpire := expireStripeCheckoutSession
	originalMark := markStripeTopUpFailed
	t.Cleanup(func() {
		attachStripeSessionToTopUp = originalAttach
		recordStripeSessionRecovery = originalRecord
		expireStripeCheckoutSession = originalExpire
		markStripeTopUpFailed = originalMark
	})

	recordAttempts := 0
	attachStripeSessionToTopUp = func(tradeNo string, sessionID string) error {
		return errors.New("database unavailable")
	}
	recordStripeSessionRecovery = func(tradeNo string, sessionID string, reason string) error {
		recordAttempts++
		return errors.New("recovery save failed")
	}
	expireStripeCheckoutSession = func(id string, params *stripe.CheckoutSessionExpireParams) (*stripe.CheckoutSession, error) {
		return &stripe.CheckoutSession{Status: stripe.CheckoutSessionStatusExpired}, nil
	}
	markStripeTopUpFailed = func(tradeNo string, sessionID string, eventID string, livemode bool, targetStatus string, reason string) error {
		return nil
	}

	err := bindStripeSessionWithRecovery(context.Background(), "ref_recovery_fail", "cs_test_recovery_fail", false)
	require.Error(t, err)
	assert.Equal(t, stripeRecoveryAttempts, recordAttempts)
}

func TestBindStripeSessionWithRecoverySaveFailsExpireSucceeds(t *testing.T) {
	originalAttach := attachStripeSessionToTopUp
	originalRecord := recordStripeSessionRecovery
	originalExpire := expireStripeCheckoutSession
	originalMark := markStripeTopUpFailed
	t.Cleanup(func() {
		attachStripeSessionToTopUp = originalAttach
		recordStripeSessionRecovery = originalRecord
		expireStripeCheckoutSession = originalExpire
		markStripeTopUpFailed = originalMark
	})

	markedExpired := false
	attachStripeSessionToTopUp = func(tradeNo string, sessionID string) error {
		return errors.New("database unavailable")
	}
	recordStripeSessionRecovery = func(tradeNo string, sessionID string, reason string) error {
		return errors.New("recovery save failed")
	}
	expireStripeCheckoutSession = func(id string, params *stripe.CheckoutSessionExpireParams) (*stripe.CheckoutSession, error) {
		return &stripe.CheckoutSession{Status: stripe.CheckoutSessionStatusExpired}, nil
	}
	markStripeTopUpFailed = func(tradeNo string, sessionID string, eventID string, livemode bool, targetStatus string, reason string) error {
		markedExpired = true
		assert.Equal(t, common.TopUpStatusExpired, targetStatus)
		return nil
	}

	err := bindStripeSessionWithRecovery(context.Background(), "ref_save_fail_expire_ok", "cs_test_save_fail_expire_ok", false)
	require.Error(t, err)
	assert.True(t, markedExpired)
}

func TestBindStripeSessionWithRecoverySaveFailsExpireFailsDoesNotMarkTerminal(t *testing.T) {
	originalAttach := attachStripeSessionToTopUp
	originalRecord := recordStripeSessionRecovery
	originalExpire := expireStripeCheckoutSession
	originalMark := markStripeTopUpFailed
	t.Cleanup(func() {
		attachStripeSessionToTopUp = originalAttach
		recordStripeSessionRecovery = originalRecord
		expireStripeCheckoutSession = originalExpire
		markStripeTopUpFailed = originalMark
	})

	attachStripeSessionToTopUp = func(tradeNo string, sessionID string) error {
		return errors.New("database unavailable")
	}
	recordStripeSessionRecovery = func(tradeNo string, sessionID string, reason string) error {
		return errors.New("recovery save failed")
	}
	expireStripeCheckoutSession = func(id string, params *stripe.CheckoutSessionExpireParams) (*stripe.CheckoutSession, error) {
		return nil, errors.New("stripe expire failed")
	}
	markStripeTopUpFailed = func(tradeNo string, sessionID string, eventID string, livemode bool, targetStatus string, reason string) error {
		t.Fatalf("double failure must not mark or credit the local order")
		return nil
	}

	err := bindStripeSessionWithRecovery(context.Background(), "ref_double_fail", "cs_test_double_fail", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expire uncertain")
}

func TestStripeAmountCalculation(t *testing.T) {
	originalUnitPrice := setting.StripeUnitPrice
	originalQuotaPerUnit := common.QuotaPerUnit
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalDiscount := paymentSetting.AmountDiscount
	t.Cleanup(func() {
		setting.StripeUnitPrice = originalUnitPrice
		common.QuotaPerUnit = originalQuotaPerUnit
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
		paymentSetting.AmountDiscount = originalDiscount
	})

	setting.StripeUnitPrice = 1
	common.QuotaPerUnit = 500000
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))
	paymentSetting.AmountDiscount = map[int]float64{}

	for _, tc := range []struct {
		amount int64
		cents  int64
	}{
		{1, 100},
		{5, 500},
		{10, 1000},
		{100, 10000},
		{10000, 1000000},
		{0, 0},
		{-1, 0},
		{10001, 0},
		{1<<62 - 1, 0},
	} {
		t.Run(fmt.Sprintf("amount_%d", tc.amount), func(t *testing.T) {
			assert.Equal(t, tc.cents, getStripePaymentCents(tc.amount, "default"))
		})
	}

	paymentSetting.AmountDiscount = map[int]float64{10: 0.9}
	cents := getStripePaymentCents(10, "default")
	credit, quota := getStripeCreditAmountAndQuota(10, "default")
	assert.EqualValues(t, 900, cents)
	assert.Equal(t, "10", credit.String())
	assert.Equal(t, 5000000, quota)

	paymentSetting.AmountDiscount = map[int]float64{}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":1.5,"fractional":1.333333}`))
	assert.EqualValues(t, 1500, getStripePaymentCents(10, "vip"))
	credit, quota = getStripeCreditAmountAndQuota(3, "fractional")
	assert.Equal(t, "3.999999", credit.String())
	assert.Equal(t, 2000000, quota)

	setting.StripeUnitPrice = 0.3333
	assert.EqualValues(t, 33, getStripePaymentCents(1, "default"))
	setting.StripeUnitPrice = 0.335
	assert.EqualValues(t, 34, getStripePaymentCents(1, "default"))
}

func TestGenStripeLinkUsesServerGeneratedPriceData(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	setStripeSchemaReadyForTest(t)
	originalKey := setting.StripeApiSecret
	originalSecret := setting.StripeWebhookSecret
	originalMode := setting.StripeMode
	originalUnitPrice := setting.StripeUnitPrice
	originalCreate := createStripeCheckoutSession
	t.Cleanup(func() {
		setting.StripeApiSecret = originalKey
		setting.StripeWebhookSecret = originalSecret
		setting.StripeMode = originalMode
		setting.StripeUnitPrice = originalUnitPrice
		createStripeCheckoutSession = originalCreate
	})

	setting.StripeApiSecret = "sk_test_local"
	setting.StripeWebhookSecret = "whsec_local"
	setting.StripeMode = "test"
	setting.StripeUnitPrice = 1
	createStripeCheckoutSession = func(params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
		require.Len(t, params.LineItems, 1)
		priceData := params.LineItems[0].PriceData
		require.NotNil(t, priceData)
		require.NotNil(t, priceData.UnitAmount)
		require.NotNil(t, priceData.Currency)
		assert.EqualValues(t, 900, *priceData.UnitAmount)
		assert.Equal(t, "usd", *priceData.Currency)
		assert.Equal(t, string(stripe.CheckoutSessionModePayment), *params.Mode)
		assert.NotNil(t, params.AllowPromotionCodes)
		assert.False(t, *params.AllowPromotionCodes)
		return &stripe.CheckoutSession{ID: "cs_test_generated", URL: "https://checkout.stripe.test/session"}, nil
	}

	result, err := genStripeLink("ref_generated", "", "user@example.test", 900, 10, 10, "", "")
	require.NoError(t, err)
	assert.Equal(t, "cs_test_generated", result.SessionID)
}

func TestStripeRequestIgnoresClientUnitAmountAndCurrency(t *testing.T) {
	req := StripePayRequest{Amount: 10, PaymentMethod: model.PaymentMethodStripe}
	payload := []byte(`{"amount":10,"payment_method":"stripe","unit_amount":1,"currency":"cny"}`)
	require.NoError(t, common.Unmarshal(payload, &req))
	assert.EqualValues(t, 10, req.Amount)
	assert.Equal(t, model.PaymentMethodStripe, req.PaymentMethod)
}

func TestStripeLogRedactionHelpers(t *testing.T) {
	signature := "t=123456789,v1=abcdef1234567890abcdef1234567890"
	body := `{"id":"evt_test_secret","data":{"object":{"customer_email":"buyer@example.test","amount_total":900}}}`
	apiSecret := "sk_test_sensitive_example"
	webhookSecret := "whsec_sensitive_example"

	message := fmt.Sprintf(
		"signature_present=%t session_id=%s payment_intent_id=%s digest=%s",
		signature != "",
		maskStripeID("cs_test_1234567890"),
		maskStripeID("pi_test_1234567890"),
		payloadDigest([]byte(body)),
	)

	assert.NotContains(t, message, signature)
	assert.NotContains(t, message, body)
	assert.NotContains(t, message, apiSecret)
	assert.NotContains(t, message, webhookSecret)
	assert.NotContains(t, message, "buyer@example.test")
	assert.Contains(t, message, "cs_tes...7890")
}
