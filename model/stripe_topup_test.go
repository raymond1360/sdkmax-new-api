package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertStripeTopUpForTest(t *testing.T, tradeNo string, userID int, sessionID string, expectedMinor int64, livemode bool) {
	t.Helper()
	var sessionIDPtr *string
	if sessionID != "" {
		sessionIDPtr = &sessionID
	}
	topUp := &TopUp{
		UserId:              userID,
		Amount:              10,
		Money:               10,
		TradeNo:             tradeNo,
		PaymentMethod:       PaymentMethodStripe,
		PaymentProvider:     PaymentProviderStripe,
		Currency:            "USD",
		ExpectedAmountMinor: expectedMinor,
		ExpectedQuota:       int(10 * common.QuotaPerUnit),
		StripeSessionId:     sessionIDPtr,
		StripeLivemode:      &livemode,
		Status:              common.TopUpStatusPending,
		CreateTime:          time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func completeStripeTopUpForTest(tradeNo string, sessionID string, eventID string) error {
	return CompleteStripeTopUp(StripeTopUpCompletion{
		TradeNo:         tradeNo,
		SessionID:       sessionID,
		PaymentIntentID: "pi_test_123",
		EventID:         eventID,
		Livemode:        false,
		Currency:        "USD",
		PaidAmountMinor: 1000,
		CustomerID:      "cus_test_123",
		CallerIP:        "127.0.0.1",
	})
}

func TestCompleteStripeTopUp_CreditsOnceWithExactMatch(t *testing.T) {
	truncateTables(t)
	common.QuotaPerUnit = 500000
	insertUserForPaymentGuardTest(t, 701, 0)
	insertStripeTopUpForTest(t, "stripe-ok", 701, "cs_test_ok", 1000, false)

	require.NoError(t, completeStripeTopUpForTest("stripe-ok", "cs_test_ok", "evt_test_ok"))
	require.NoError(t, completeStripeTopUpForTest("stripe-ok", "cs_test_ok", "evt_test_ok"))

	topUp := GetTopUpByTradeNo("stripe-ok")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.EqualValues(t, 1000, topUp.PaidAmountMinor)
	assert.Equal(t, "pi_test_123", *topUp.StripePaymentIntentId)
	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, 701))
}

func TestCompleteStripeTopUp_BindsSessionWhenWebhookArrivesFirst(t *testing.T) {
	truncateTables(t)
	common.QuotaPerUnit = 500000
	insertUserForPaymentGuardTest(t, 706, 0)
	insertStripeTopUpForTest(t, "stripe-webhook-first", 706, "", 1000, false)

	require.NoError(t, completeStripeTopUpForTest("stripe-webhook-first", "cs_test_first", "evt_test_first"))

	topUp := GetTopUpByTradeNo("stripe-webhook-first")
	require.NotNil(t, topUp)
	require.NotNil(t, topUp.StripeSessionId)
	assert.Equal(t, "cs_test_first", *topUp.StripeSessionId)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, 706))
}

func TestCompleteStripeTopUp_RecoversFailedOrExpiredOrderAfterPaidEvent(t *testing.T) {
	for _, status := range []string{common.TopUpStatusFailed, common.TopUpStatusExpired} {
		t.Run(status, func(t *testing.T) {
			truncateTables(t)
			common.QuotaPerUnit = 500000
			insertUserForPaymentGuardTest(t, 707, 0)
			insertStripeTopUpForTest(t, "stripe-recover-"+status, 707, "cs_test_recover", 1000, false)
			require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "stripe-recover-"+status).Update("status", status).Error)

			require.NoError(t, completeStripeTopUpForTest("stripe-recover-"+status, "cs_test_recover", "evt_test_recover_"+status))

			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, "stripe-recover-"+status))
			assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, 707))
		})
	}
}

func TestCompleteStripeTopUp_RejectsMismatches(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*StripeTopUpCompletion)
		want   error
	}{
		{name: "session", mutate: func(p *StripeTopUpCompletion) { p.SessionID = "cs_test_other" }, want: ErrStripeTopUpMismatch},
		{name: "currency", mutate: func(p *StripeTopUpCompletion) { p.Currency = "CNY" }, want: ErrStripeTopUpBadCurrency},
		{name: "amount", mutate: func(p *StripeTopUpCompletion) { p.PaidAmountMinor = 999 }, want: ErrStripeTopUpBadAmount},
		{name: "livemode", mutate: func(p *StripeTopUpCompletion) { p.Livemode = true }, want: ErrStripeTopUpBadMode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 702, 0)
			insertStripeTopUpForTest(t, "stripe-"+tc.name, 702, "cs_test_match", 1000, false)
			params := StripeTopUpCompletion{
				TradeNo:         "stripe-" + tc.name,
				SessionID:       "cs_test_match",
				PaymentIntentID: "pi_test_123",
				EventID:         "evt_test_" + tc.name,
				Livemode:        false,
				Currency:        "USD",
				PaidAmountMinor: 1000,
				CallerIP:        "127.0.0.1",
			}
			tc.mutate(&params)

			err := CompleteStripeTopUp(params)
			require.ErrorIs(t, err, tc.want)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "stripe-"+tc.name))
			assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 702))
		})
	}
}

func TestCompleteStripeTopUp_RejectsNonStripeOrder(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 703, 0)
	insertTopUpForPaymentGuardTest(t, "stripe-cross-provider", 703, PaymentProviderEpay)

	err := completeStripeTopUpForTest("stripe-cross-provider", "cs_test_ok", "evt_test_cross")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "stripe-cross-provider"))
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 703))
}

func TestCompleteStripeTopUp_RejectsEventIDReplayAcrossOrders(t *testing.T) {
	truncateTables(t)
	common.QuotaPerUnit = 500000
	insertUserForPaymentGuardTest(t, 708, 0)
	insertUserForPaymentGuardTest(t, 709, 0)
	insertStripeTopUpForTest(t, "stripe-event-a", 708, "cs_test_a", 1000, false)
	insertStripeTopUpForTest(t, "stripe-event-b", 709, "cs_test_b", 1000, false)

	require.NoError(t, completeStripeTopUpForTest("stripe-event-a", "cs_test_a", "evt_test_shared"))
	err := completeStripeTopUpForTest("stripe-event-b", "cs_test_b", "evt_test_shared")

	require.ErrorIs(t, err, ErrStripeEventReplayed)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "stripe-event-b"))
	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, 708))
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 709))
}

func TestCompleteStripeTopUp_SameSessionDifferentEventCreditsOnce(t *testing.T) {
	truncateTables(t)
	common.QuotaPerUnit = 500000
	insertUserForPaymentGuardTest(t, 710, 0)
	insertStripeTopUpForTest(t, "stripe-session-replay", 710, "cs_test_once", 1000, false)

	require.NoError(t, completeStripeTopUpForTest("stripe-session-replay", "cs_test_once", "evt_test_once_1"))
	require.NoError(t, completeStripeTopUpForTest("stripe-session-replay", "cs_test_once", "evt_test_once_2"))

	assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, "stripe-session-replay"))
	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, 710))
}

func TestCompleteStripeTopUp_CreditsOnlyOrderOwner(t *testing.T) {
	truncateTables(t)
	common.QuotaPerUnit = 500000
	insertUserForPaymentGuardTest(t, 711, 0)
	insertUserForPaymentGuardTest(t, 712, 0)
	insertStripeTopUpForTest(t, "stripe-owner-only", 711, "cs_test_owner", 1000, false)

	require.NoError(t, completeStripeTopUpForTest("stripe-owner-only", "cs_test_owner", "evt_test_owner"))

	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, 711))
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 712))
}

func TestCompleteStripeTopUp_UsesStoredExpectedQuota(t *testing.T) {
	truncateTables(t)
	common.QuotaPerUnit = 500000
	insertUserForPaymentGuardTest(t, 714, 0)
	insertStripeTopUpForTest(t, "stripe-stored-quota", 714, "cs_test_stored_quota", 400, false)
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "stripe-stored-quota").Updates(map[string]any{
		"money":          3.999999,
		"expected_quota": 2000000,
	}).Error)

	require.NoError(t, CompleteStripeTopUp(StripeTopUpCompletion{
		TradeNo:         "stripe-stored-quota",
		SessionID:       "cs_test_stored_quota",
		PaymentIntentID: "pi_test_stored_quota",
		EventID:         "evt_test_stored_quota",
		Livemode:        false,
		Currency:        "USD",
		PaidAmountMinor: 400,
		CallerIP:        "127.0.0.1",
	}))

	assert.Equal(t, 2000000, getUserQuotaForPaymentGuardTest(t, 714))
}

func TestCompleteStripeTopUp_ConcurrentReplayCreditsOnce(t *testing.T) {
	truncateTables(t)
	common.QuotaPerUnit = 500000
	insertUserForPaymentGuardTest(t, 704, 0)
	insertStripeTopUpForTest(t, "stripe-race", 704, "cs_test_race", 1000, false)

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_ = completeStripeTopUpForTest("stripe-race", "cs_test_race", "evt_test_race")
		}()
	}
	wg.Wait()

	assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, "stripe-race"))
	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, 704))
}

func TestMarkStripeTopUpFailed_RequiresMatchingSessionAndMode(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 705, 0)
	insertStripeTopUpForTest(t, "stripe-expired", 705, "cs_test_expired", 1000, false)

	require.ErrorIs(t, MarkStripeTopUpFailed("stripe-expired", "cs_test_other", "evt_bad", false, common.TopUpStatusExpired, "expired"), ErrStripeTopUpMismatch)
	require.ErrorIs(t, MarkStripeTopUpFailed("stripe-expired", "cs_test_expired", "evt_bad_mode", true, common.TopUpStatusExpired, "expired"), ErrStripeTopUpBadMode)
	require.NoError(t, MarkStripeTopUpFailed("stripe-expired", "cs_test_expired", "evt_expired", false, common.TopUpStatusExpired, "expired"))
	assert.Equal(t, common.TopUpStatusExpired, getTopUpStatusForPaymentGuardTest(t, "stripe-expired"))
}

func TestManualCompleteTopUpRejectsStripeOrders(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 713, 0)
	insertStripeTopUpForTest(t, "stripe-manual-block", 713, "cs_test_manual", 1000, false)

	err := ManualCompleteTopUp("stripe-manual-block", "127.0.0.1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Stripe")
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "stripe-manual-block"))
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 713))
}
