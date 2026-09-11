package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/thanhpk/randstr"
)

const (
	stripeWebhookMaxBodyBytes = 1 << 20
	stripeFixedCurrency       = "USD"
	stripeBindRetryAttempts   = 3
	stripeBindRetryDelay      = 150 * time.Millisecond
	stripeRecoveryAttempts    = 2
	stripeRecoveryDelay       = 100 * time.Millisecond
)

var stripeAdaptor = &StripeAdaptor{}

var createStripeCheckoutSession = session.New
var expireStripeCheckoutSession = session.Expire
var attachStripeSessionToTopUp = model.AttachStripeSessionToTopUp
var recordStripeSessionRecovery = model.RecordStripeSessionRecovery
var recordStripeOrphanSessionAudit = model.RecordStripeOrphanSessionAudit
var markStripeTopUpFailed = model.MarkStripeTopUpFailed

type StripePayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
	SuccessURL    string `json:"success_url,omitempty"`
	CancelURL     string `json:"cancel_url,omitempty"`
}

type StripeAdaptor struct{}

type stripeCheckoutResult struct {
	URL       string
	SessionID string
}

type stripeValidatedSession struct {
	TradeNo               string
	SessionID             string
	PaymentIntentID       string
	CustomerID            string
	Currency              string
	AmountTotal           int64
	Livemode              bool
	ProviderPayloadDigest string
}

type stripeWebhookResult int

const (
	stripeWebhookOK stripeWebhookResult = iota
	stripeWebhookPermanentReject
	stripeWebhookTransientFailure
)

func (*StripeAdaptor) RequestAmount(c *gin.Context, req *StripePayRequest) {
	if err := validateStripeTopUpAmount(req.Amount); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "failed to get user group"})
		return
	}
	expectedCents := getStripePaymentCents(req.Amount, group)
	if expectedCents < 1 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "topup amount is too low"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": formatStripeCents(expectedCents)})
}

func (*StripeAdaptor) RequestPay(c *gin.Context, req *StripePayRequest) {
	if req.PaymentMethod != model.PaymentMethodStripe {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "unsupported payment provider"})
		return
	}
	if err := validateStripeRuntimeConfig(true); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Stripe checkout disabled user_id=%d reason=%s", c.GetInt("id"), err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Stripe payment is not configured"})
		return
	}
	if err := validateStripeTopUpAmount(req.Amount); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if req.SuccessURL != "" && common.ValidateRedirectURL(req.SuccessURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "payment success redirect URL is not trusted", "data": ""})
		return
	}
	if req.CancelURL != "" && common.ValidateRedirectURL(req.CancelURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "payment cancel redirect URL is not trusted", "data": ""})
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "failed to get user"})
		return
	}

	creditMoneyDecimal, expectedQuota := getStripeCreditAmountAndQuota(req.Amount, user.Group)
	creditMoney := creditMoneyDecimal.InexactFloat64()
	expectedCents := getStripePaymentCents(req.Amount, user.Group)
	if expectedCents < 1 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "topup amount is too low"})
		return
	}

	reference := fmt.Sprintf("new-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))
	livemode := expectedStripeLivemode()
	topUp := &model.TopUp{
		UserId:              id,
		Amount:              req.Amount,
		Money:               creditMoney,
		TradeNo:             referenceId,
		PaymentMethod:       model.PaymentMethodStripe,
		PaymentProvider:     model.PaymentProviderStripe,
		Currency:            stripeFixedCurrency,
		ExpectedAmountMinor: expectedCents,
		ExpectedQuota:       expectedQuota,
		StripeLivemode:      &livemode,
		CreateTime:          time.Now().Unix(),
		Status:              common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe local order create failed user_id=%d trade_no=%s amount=%d currency=%s error=%q", id, referenceId, req.Amount, stripeFixedCurrency, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "failed to create order"})
		return
	}

	checkout, err := genStripeLink(referenceId, user.StripeCustomer, user.Email, expectedCents, req.Amount, creditMoney, req.SuccessURL, req.CancelURL)
	if err != nil {
		_ = markStripeTopUpFailed(referenceId, "", "", livemode, common.TopUpStatusFailed, "stripe checkout creation failed")
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe checkout create failed user_id=%d trade_no=%s amount_minor=%d currency=%s mode=%s error=%q", id, referenceId, expectedCents, stripeFixedCurrency, normalizedStripeMode(), err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "failed to create payment session"})
		return
	}
	if err := bindStripeSessionWithRecovery(c.Request.Context(), referenceId, checkout.SessionID, livemode); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe session bind failed user_id=%d trade_no=%s session_id=%s amount_minor=%d currency=%s mode=%s error=%q", id, referenceId, maskStripeID(checkout.SessionID), expectedCents, stripeFixedCurrency, normalizedStripeMode(), err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "failed to bind payment session"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Stripe topup order created user_id=%d trade_no=%s session_id=%s amount_minor=%d currency=%s mode=%s", id, referenceId, maskStripeID(checkout.SessionID), expectedCents, stripeFixedCurrency, normalizedStripeMode()))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"pay_link": checkout.URL, "trade_no": referenceId, "status": common.TopUpStatusPending}})
}

func RequestStripeAmount(c *gin.Context) {
	var req StripePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "invalid request"})
		return
	}
	stripeAdaptor.RequestAmount(c, &req)
}

func RequestStripePay(c *gin.Context) {
	var req StripePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "invalid request"})
		return
	}
	stripeAdaptor.RequestPay(c, &req)
}

func StripeWebhook(c *gin.Context) {
	ctx := c.Request.Context()
	if !isStripeWebhookEnabled() {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook rejected reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, stripeWebhookMaxBodyBytes)
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe webhook read failed path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		if common.IsRequestBodyTooLargeError(err) {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	event, err := webhook.ConstructEventWithOptions(payload, c.GetHeader("Stripe-Signature"), setting.StripeWebhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook signature failed path=%q client_ip=%s signature_present=%t error=%q", c.Request.RequestURI, c.ClientIP(), c.GetHeader("Stripe-Signature") != "", err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	result := stripeWebhookOK
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted, stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		result = handleStripeSessionPaid(ctx, event, payload, c.ClientIP())
	case stripe.EventTypeCheckoutSessionExpired:
		result = handleStripeSessionExpired(ctx, event, c.ClientIP())
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		result = handleStripeSessionFailed(ctx, event, c.ClientIP(), common.TopUpStatusFailed, "stripe async payment failed")
	default:
		logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook ignored event_id=%s event_type=%s mode=%s client_ip=%s", event.ID, event.Type, modeFromLivemode(event.Livemode), c.ClientIP()))
	}
	if result == stripeWebhookTransientFailure {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	c.Status(http.StatusOK)
}

func handleStripeSessionPaid(ctx context.Context, event stripe.Event, payload []byte, callerIP string) stripeWebhookResult {
	validated, err := validateStripeCheckoutSession(event, payload, true)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook rejected event_id=%s event_type=%s trade_no=%s session_id=%s mode=%s reason=%s", event.ID, event.Type, safeTradeNoFromEvent(event), maskStripeID(safeSessionIDFromEvent(event)), modeFromLivemode(event.Livemode), err.Error()))
		return stripeWebhookPermanentReject
	}

	LockOrder(validated.TradeNo)
	defer UnlockOrder(validated.TradeNo)

	err = model.CompleteStripeTopUp(model.StripeTopUpCompletion{
		TradeNo:               validated.TradeNo,
		SessionID:             validated.SessionID,
		PaymentIntentID:       validated.PaymentIntentID,
		EventID:               event.ID,
		Livemode:              validated.Livemode,
		Currency:              validated.Currency,
		PaidAmountMinor:       validated.AmountTotal,
		CustomerID:            validated.CustomerID,
		CallerIP:              callerIP,
		ProviderPayloadDigest: validated.ProviderPayloadDigest,
	})
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe topup completion rejected event_id=%s event_type=%s trade_no=%s session_id=%s payment_intent_id=%s amount_minor=%d currency=%s mode=%s reason=%s", event.ID, event.Type, validated.TradeNo, maskStripeID(validated.SessionID), maskStripeID(validated.PaymentIntentID), validated.AmountTotal, validated.Currency, modeFromLivemode(validated.Livemode), err.Error()))
		if model.IsPermanentStripeTopUpError(err) {
			return stripeWebhookPermanentReject
		}
		return stripeWebhookTransientFailure
	}
	if err := model.ResolveStripeOrphanSessionAudit(validated.TradeNo, validated.SessionID, "resolved by paid webhook"); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe orphan session audit resolve failed trade_no=%s session_id=%s error=%q", validated.TradeNo, maskStripeID(validated.SessionID), err.Error()))
	}
	logger.LogInfo(ctx, fmt.Sprintf("Stripe topup credited event_id=%s event_type=%s trade_no=%s session_id=%s payment_intent_id=%s amount_minor=%d currency=%s mode=%s", event.ID, event.Type, validated.TradeNo, maskStripeID(validated.SessionID), maskStripeID(validated.PaymentIntentID), validated.AmountTotal, validated.Currency, modeFromLivemode(validated.Livemode)))
	return stripeWebhookOK
}

func handleStripeSessionExpired(ctx context.Context, event stripe.Event, callerIP string) stripeWebhookResult {
	s, err := parseStripeSession(event)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout expired parse failed event_id=%s event_type=%s mode=%s reason=%s", event.ID, event.Type, modeFromLivemode(event.Livemode), err.Error()))
		return stripeWebhookPermanentReject
	}
	tradeNo := stripeSessionTradeNo(s)
	if tradeNo == "" || string(s.Status) != "expired" {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout expired rejected event_id=%s trade_no=%s session_id=%s mode=%s reason=invalid_expired_session", event.ID, tradeNo, maskStripeID(s.ID), modeFromLivemode(event.Livemode)))
		return stripeWebhookPermanentReject
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	if err := markStripeTopUpFailed(tradeNo, s.ID, event.ID, event.Livemode, common.TopUpStatusExpired, "stripe checkout expired"); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout expired update rejected event_id=%s trade_no=%s session_id=%s mode=%s reason=%s", event.ID, tradeNo, maskStripeID(s.ID), modeFromLivemode(event.Livemode), err.Error()))
		if model.IsPermanentStripeTopUpError(err) {
			return stripeWebhookPermanentReject
		}
		return stripeWebhookTransientFailure
	}
	logger.LogInfo(ctx, fmt.Sprintf("Stripe checkout expired event_id=%s trade_no=%s session_id=%s mode=%s client_ip=%s", event.ID, tradeNo, maskStripeID(s.ID), modeFromLivemode(event.Livemode), callerIP))
	return stripeWebhookOK
}

func handleStripeSessionFailed(ctx context.Context, event stripe.Event, callerIP string, targetStatus string, reason string) stripeWebhookResult {
	s, err := parseStripeSession(event)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout failure parse failed event_id=%s event_type=%s mode=%s reason=%s", event.ID, event.Type, modeFromLivemode(event.Livemode), err.Error()))
		return stripeWebhookPermanentReject
	}
	tradeNo := stripeSessionTradeNo(s)
	if tradeNo == "" {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout failure rejected event_id=%s session_id=%s mode=%s reason=missing_trade_no", event.ID, maskStripeID(s.ID), modeFromLivemode(event.Livemode)))
		return stripeWebhookPermanentReject
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	if err := markStripeTopUpFailed(tradeNo, s.ID, event.ID, event.Livemode, targetStatus, reason); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout failure update rejected event_id=%s trade_no=%s session_id=%s mode=%s reason=%s", event.ID, tradeNo, maskStripeID(s.ID), modeFromLivemode(event.Livemode), err.Error()))
		if model.IsPermanentStripeTopUpError(err) {
			return stripeWebhookPermanentReject
		}
		return stripeWebhookTransientFailure
	}
	logger.LogInfo(ctx, fmt.Sprintf("Stripe checkout failed event_id=%s trade_no=%s session_id=%s mode=%s client_ip=%s", event.ID, tradeNo, maskStripeID(s.ID), modeFromLivemode(event.Livemode), callerIP))
	return stripeWebhookOK
}

func validateStripeCheckoutSession(event stripe.Event, payload []byte, requirePaid bool) (stripeValidatedSession, error) {
	s, err := parseStripeSession(event)
	if err != nil {
		return stripeValidatedSession{}, err
	}
	tradeNo := stripeSessionTradeNo(s)
	if tradeNo == "" {
		return stripeValidatedSession{}, errors.New("missing_trade_no")
	}
	if s.ClientReferenceID != tradeNo || s.Metadata["trade_no"] != tradeNo {
		return stripeValidatedSession{}, errors.New("trade_no_mismatch")
	}
	if s.ID == "" {
		return stripeValidatedSession{}, errors.New("missing_session_id")
	}
	if event.Livemode != expectedStripeLivemode() || s.Livemode != event.Livemode {
		return stripeValidatedSession{}, errors.New("livemode_mismatch")
	}
	if string(s.Mode) != string(stripe.CheckoutSessionModePayment) {
		return stripeValidatedSession{}, errors.New("mode_mismatch")
	}
	if string(s.Status) != string(stripe.CheckoutSessionStatusComplete) {
		return stripeValidatedSession{}, errors.New("session_status_not_complete")
	}
	if requirePaid && string(s.PaymentStatus) != string(stripe.CheckoutSessionPaymentStatusPaid) {
		return stripeValidatedSession{}, errors.New("payment_status_not_paid")
	}
	currency := strings.ToUpper(string(s.Currency))
	if currency != stripeFixedCurrency {
		return stripeValidatedSession{}, errors.New("currency_mismatch")
	}
	if s.AmountTotal <= 0 {
		return stripeValidatedSession{}, errors.New("invalid_amount_total")
	}
	return stripeValidatedSession{
		TradeNo:               tradeNo,
		SessionID:             s.ID,
		PaymentIntentID:       stripePaymentIntentID(s.PaymentIntent),
		CustomerID:            stripeCustomerID(s.Customer),
		Currency:              currency,
		AmountTotal:           s.AmountTotal,
		Livemode:              event.Livemode,
		ProviderPayloadDigest: payloadDigest(payload),
	}, nil
}

func parseStripeSession(event stripe.Event) (stripe.CheckoutSession, error) {
	if event.Data == nil || len(event.Data.Raw) == 0 {
		return stripe.CheckoutSession{}, errors.New("missing_event_object")
	}
	var s stripe.CheckoutSession
	if err := common.Unmarshal(event.Data.Raw, &s); err != nil {
		return stripe.CheckoutSession{}, err
	}
	return s, nil
}

func stripeSessionTradeNo(s stripe.CheckoutSession) string {
	if s.Metadata != nil && s.Metadata["trade_no"] != "" {
		return s.Metadata["trade_no"]
	}
	return s.ClientReferenceID
}

func safeTradeNoFromEvent(event stripe.Event) string {
	s, err := parseStripeSession(event)
	if err != nil {
		return ""
	}
	return stripeSessionTradeNo(s)
}

func safeSessionIDFromEvent(event stripe.Event) string {
	s, err := parseStripeSession(event)
	if err != nil {
		return ""
	}
	return s.ID
}

func genStripeLink(referenceId string, customerId string, email string, expectedAmountMinor int64, requestedAmount int64, creditMoney float64, successURL string, cancelURL string) (stripeCheckoutResult, error) {
	if err := validateStripeRuntimeConfig(true); err != nil {
		return stripeCheckoutResult{}, err
	}
	stripe.Key = setting.StripeApiSecret

	if successURL == "" {
		successURL = paymentReturnPath("/console/topup")
	}
	if cancelURL == "" {
		cancelURL = paymentReturnPath("/console/topup")
	}

	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(successURL),
		CancelURL:         stripe.String(cancelURL),
		PaymentMethodTypes: []*string{
			stripe.String("card"),
		},
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(strings.ToLower(stripeFixedCurrency)),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("SDKMAX balance top-up"),
					},
					UnitAmount: stripe.Int64(expectedAmountMinor),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:                stripe.String(string(stripe.CheckoutSessionModePayment)),
		AllowPromotionCodes: stripe.Bool(false),
		Metadata: map[string]string{
			"trade_no":              referenceId,
			"payment_provider":      model.PaymentProviderStripe,
			"requested_amount":      strconv.FormatInt(requestedAmount, 10),
			"credit_money":          strconv.FormatFloat(creditMoney, 'f', 2, 64),
			"expected_amount_minor": strconv.FormatInt(expectedAmountMinor, 10),
			"currency":              stripeFixedCurrency,
			"mode":                  normalizedStripeMode(),
		},
	}
	if customerId == "" {
		if email != "" {
			params.CustomerEmail = stripe.String(email)
		}
		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	result, err := createStripeCheckoutSession(params)
	if err != nil {
		return stripeCheckoutResult{}, err
	}
	if result == nil || result.URL == "" || result.ID == "" {
		return stripeCheckoutResult{}, errors.New("stripe returned incomplete checkout session")
	}
	return stripeCheckoutResult{URL: result.URL, SessionID: result.ID}, nil
}

func bindStripeSessionWithRecovery(ctx context.Context, tradeNo string, sessionID string, livemode bool) error {
	var lastErr error
	for attempt := 1; attempt <= stripeBindRetryAttempts; attempt++ {
		if err := attachStripeSessionToTopUp(tradeNo, sessionID); err != nil {
			lastErr = err
			logger.LogWarn(ctx, fmt.Sprintf("Stripe session bind retry trade_no=%s session_id=%s attempt=%d error=%q", tradeNo, maskStripeID(sessionID), attempt, err.Error()))
			if attempt < stripeBindRetryAttempts {
				time.Sleep(stripeBindRetryDelay)
			}
			continue
		}
		return nil
	}

	reason := "stripe session binding pending recovery"
	if lastErr != nil {
		reason = "stripe session binding pending recovery: " + lastErr.Error()
	}
	recoveryErr := recordStripeSessionRecoveryWithRetry(ctx, tradeNo, sessionID, reason)
	if recoveryErr != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe session recovery record failed trade_no=%s session_id=%s error=%q", tradeNo, maskStripeID(sessionID), recoveryErr.Error()))
	}

	expired, err := expireStripeCheckoutSession(sessionID, nil)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe session expire failed after bind retries trade_no=%s session_id=%s error=%q", tradeNo, maskStripeID(sessionID), err.Error()))
		recordStripeOrphanAuditAfterTripleFailure(ctx, tradeNo, sessionID, livemode, lastErr, recoveryErr, err)
		return fmt.Errorf("stripe session binding failed and expire uncertain: %w", lastErr)
	}
	if expired == nil || string(expired.Status) != string(stripe.CheckoutSessionStatusExpired) {
		status := ""
		if expired != nil {
			status = string(expired.Status)
		}
		logger.LogError(ctx, fmt.Sprintf("Stripe session expire uncertain after bind retries trade_no=%s session_id=%s returned_status=%s", tradeNo, maskStripeID(sessionID), status))
		recordStripeOrphanAuditAfterTripleFailure(ctx, tradeNo, sessionID, livemode, lastErr, recoveryErr, fmt.Errorf("expire status uncertain: %s", status))
		return fmt.Errorf("stripe session binding failed and expire status uncertain: %w", lastErr)
	}
	if err := markStripeTopUpFailed(tradeNo, sessionID, "", livemode, common.TopUpStatusExpired, "stripe session expired after local binding failure"); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe local order expire mark failed trade_no=%s session_id=%s error=%q", tradeNo, maskStripeID(sessionID), err.Error()))
		return err
	}
	return fmt.Errorf("stripe session binding failed and session expired: %w", lastErr)
}

func recordStripeSessionRecoveryWithRetry(ctx context.Context, tradeNo string, sessionID string, reason string) error {
	var lastErr error
	for attempt := 1; attempt <= stripeRecoveryAttempts; attempt++ {
		if err := recordStripeSessionRecovery(tradeNo, sessionID, reason); err != nil {
			lastErr = err
			logger.LogWarn(ctx, fmt.Sprintf("Stripe session recovery record retry trade_no=%s session_id=%s attempt=%d error=%q", tradeNo, maskStripeID(sessionID), attempt, err.Error()))
			if attempt < stripeRecoveryAttempts {
				time.Sleep(stripeRecoveryDelay)
			}
			continue
		}
		return nil
	}
	return lastErr
}

func recordStripeOrphanAuditAfterTripleFailure(ctx context.Context, tradeNo string, sessionID string, livemode bool, attachErr error, recoveryErr error, expireErr error) {
	if recoveryErr == nil {
		return
	}
	if err := recordStripeOrphanSessionAudit(model.StripeOrphanSessionAuditInput{
		TradeNo:           tradeNo,
		StripeSessionId:   sessionID,
		StripeLivemode:    livemode,
		FailureStage:      model.StripeOrphanSessionFailureStageAttachRecoveryExpire,
		AttachErrorCode:   errorString(attachErr),
		RecoveryErrorCode: errorString(recoveryErr),
		ExpireErrorCode:   errorString(expireErr),
	}); err != nil {
		logger.LogError(ctx, fmt.Sprintf("SECURITY_ALERT stripe orphan session audit write failed trade_no=%s session_id=%s error=%q", tradeNo, maskStripeID(sessionID), err.Error()))
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func GetChargedAmount(count float64, user model.User) float64 {
	topUpGroupRatio := common.GetTopupGroupRatio(user.Group)
	if topUpGroupRatio == 0 {
		topUpGroupRatio = 1
	}
	return count * topUpGroupRatio
}

func validateStripeTopUpAmount(amount int64) error {
	if amount < getStripeMinTopup() {
		return fmt.Errorf("topup amount cannot be less than %d", getStripeMinTopup())
	}
	if amount > 10000 {
		return errors.New("topup amount cannot exceed 10000")
	}
	return nil
}

func getStripeCreditAmountAndQuota(amount int64, group string) (decimal.Decimal, int) {
	dAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount = dAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	creditAmount := dAmount.Mul(decimal.NewFromFloat(topupGroupRatio))
	return creditAmount, int(creditAmount.Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Round(0).IntPart())
}

func getStripePaymentCents(amount int64, group string) int64 {
	if amount <= 0 || amount > 10000 {
		return 0
	}
	dAmount := decimal.NewFromInt(amount)
	originalAmount := amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount = dAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(originalAmount)]; ok && ds > 0 {
		discount = ds
	}
	// Base rule: without a discount, Stripe collects 1 USD for each 1 USD-equivalent
	// SDKMAX credit unit. Discount is a marketing subsidy: it reduces Stripe cash
	// collection only, while the credited SDKMAX amount remains pre-discount.
	return dAmount.
		Mul(decimal.NewFromFloat(setting.StripeUnitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(discount)).
		Mul(decimal.NewFromInt(100)).
		Round(0).
		IntPart()
}

func getStripeMinTopup() int64 {
	minTopup := setting.StripeMinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		minTopup = minTopup * int(common.QuotaPerUnit)
	}
	return int64(minTopup)
}

func formatStripeCents(cents int64) string {
	return decimal.NewFromInt(cents).Div(decimal.NewFromInt(100)).StringFixed(2)
}

func modeFromLivemode(livemode bool) string {
	if livemode {
		return "live"
	}
	return "test"
}

func stripePaymentIntentID(paymentIntent *stripe.PaymentIntent) string {
	if paymentIntent == nil {
		return ""
	}
	return paymentIntent.ID
}

func stripeCustomerID(customer *stripe.Customer) string {
	if customer == nil {
		return ""
	}
	return customer.ID
}

func payloadDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func maskStripeID(id string) string {
	return model.MaskStripeIdentifier(id)
}
