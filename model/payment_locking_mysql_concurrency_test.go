package model

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// concurrencyTestMySQLDSNEnv must point to a disposable MySQL 5.7+ database
// before the tests in this file will run. They are skipped otherwise.
//
// Why these tests can't run against the package's default SQLite test DB
// (see TestMain in task_cas_test.go): that connection is forced to
// SetMaxOpenConns(1), which serializes every statement and can never
// reproduce a missing-row-lock bug — concurrent transactions physically
// cannot interleave on a single connection. These functions credit real
// money/quota based on a "SELECT ... FOR UPDATE, check status, then
// UPDATE" pattern; only a real multi-connection database can prove the
// row lock actually blocks concurrent readers. See
// docs/payment/STRIPE_PAYMENT_TEST_ENV_VALIDATION_09111049.md section 13
// for the original real-MySQL discovery of this bug class.
const concurrencyTestMySQLDSNEnv = "MYSQL_CONCURRENCY_TEST_DSN"

// concurrencyGoroutines matches the concurrency level used in the original
// bug report (section 13.1-13.4) so results are directly comparable.
const concurrencyGoroutines = 8

// setupMySQLConcurrencyTestDB swaps the package-level DB/LOG_DB to a real
// MySQL connection pool for the duration of a single test, then restores
// the previous state (mirrors the swap pattern already used by
// setupAbilityVisibilityTestDB in ability_visibility_test.go).
func setupMySQLConcurrencyTestDB(t *testing.T) {
	t.Helper()
	dsn := os.Getenv(concurrencyTestMySQLDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a disposable MySQL 5.7+ DSN to run this test, e.g. "+
			"provide the DSN only via the local environment variable and do not print it "+
			"(never point this at a real dev/prod database — tables are migrated and wiped by this test)",
			concurrencyTestMySQLDSNEnv)
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// A real pool with more than one connection is the entire point: it lets
	// concurrent transactions actually interleave instead of being
	// serialized by a single shared connection.
	sqlDB.SetMaxOpenConns(32)
	sqlDB.SetMaxIdleConns(32)

	previousDB := DB
	previousLogDB := LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	common.UsingSQLite = false
	common.UsingMySQL = true
	common.UsingPostgreSQL = false
	initCol()

	DB = db
	LOG_DB = db

	require.NoError(t, db.AutoMigrate(
		&User{},
		&TopUp{},
		&Redemption{},
		&SubscriptionPlan{},
		&SubscriptionOrder{},
		&UserSubscription{},
		&SubscriptionPreConsumeRecord{},
		&Log{},
	))

	t.Cleanup(func() {
		for _, table := range []string{
			"top_ups", "redemptions", "subscription_orders",
			"user_subscriptions", "subscription_pre_consume_records",
			"subscription_plans", "logs", "users",
		} {
			DB.Exec("DELETE FROM " + table)
		}
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		initCol()
		_ = sqlDB.Close()
	})
}

var concurrencyTestIDSeq int64 = 900000

// nextConcurrencyTestID returns a fresh id for fixtures (users, plans, ...)
// so parallel test runs / reruns never collide on explicit primary keys.
func nextConcurrencyTestID() int {
	return int(atomic.AddInt64(&concurrencyTestIDSeq, 1))
}

func insertConcurrencyTestUser(t *testing.T, quota int) int {
	t.Helper()
	id := nextConcurrencyTestID()
	user := &User{
		Id:       id,
		Username: fmt.Sprintf("concurrency_user_%d", id),
		Status:   common.UserStatusEnabled,
		Quota:    quota,
		AffCode:  fmt.Sprintf("concur%d", id),
	}
	require.NoError(t, DB.Create(user).Error)
	return id
}

func insertConcurrencyTestUserWithAffQuota(t *testing.T, quota int, affQuota int) int {
	t.Helper()
	id := nextConcurrencyTestID()
	user := &User{
		Id:       id,
		Username: fmt.Sprintf("concurrency_user_%d", id),
		Status:   common.UserStatusEnabled,
		Quota:    quota,
		AffQuota: affQuota,
		AffCode:  fmt.Sprintf("concur%d", id),
	}
	require.NoError(t, DB.Create(user).Error)
	return id
}

func getConcurrencyTestUser(t *testing.T, userID int) User {
	t.Helper()
	var user User
	require.NoError(t, DB.Where("id = ?", userID).First(&user).Error)
	return user
}

func concurrencyErrorStats(errs []error) (successCount int, failureCount int, deadlockCount int, lockWaitTimeoutCount int) {
	for _, err := range errs {
		if err == nil {
			successCount++
			continue
		}
		failureCount++
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "deadlock") {
			deadlockCount++
		}
		if strings.Contains(message, "lock wait timeout") {
			lockWaitTimeoutCount++
		}
	}
	return successCount, failureCount, deadlockCount, lockWaitTimeoutCount
}

func logConcurrencyResult(t *testing.T, name string, initialQuota int, expectedQuota int, actualQuota int, finalState string, errs []error) {
	t.Helper()
	successCount, failureCount, deadlockCount, lockWaitTimeoutCount := concurrencyErrorStats(errs)
	t.Logf("%s: concurrency=%d initial_quota=%d expected_quota=%d actual_quota=%d final_state=%s success=%d failure=%d deadlocks=%d lock_wait_timeouts=%d",
		name, concurrencyGoroutines, initialQuota, expectedQuota, actualQuota, finalState, successCount, failureCount, deadlockCount, lockWaitTimeoutCount)
}

func assertDryRunSelectForUpdate(t *testing.T, name string) {
	t.Helper()
	tx := DB.Session(&gorm.Session{DryRun: true}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", 1).
		First(&User{})
	sql := tx.Statement.SQL.String()
	require.Contains(t, strings.ToUpper(sql), "FOR UPDATE")
	t.Logf("%s generated locking SQL: %s", name, sql)
}

// runConcurrently calls fn with indices [0, n) from n goroutines started as
// close together as possible and returns each goroutine's error in order.
func runConcurrently(n int, fn func(idx int) error) []error {
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			errs[idx] = fn(idx)
		}(i)
	}
	close(start)
	wg.Wait()
	return errs
}

func TestMySQLConcurrency_GORMV2LockingClauseGeneratesForUpdate(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)
	assertDryRunSelectForUpdate(t, "gorm_v2_clause_locking")
}

// TestMySQLConcurrency_CompleteStripeTopUp_SameEventCreditedOnce reproduces
// scenario 1 from the STRIPE_PAYMENT_TEST_ENV_VALIDATION report: N
// concurrent webhook replays of the exact same trade_no/session_id/event_id
// must credit quota exactly once, not N times.
func TestMySQLConcurrency_CompleteStripeTopUp_SameEventCreditedOnce(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)
	common.QuotaPerUnit = 500000

	userID := insertConcurrencyTestUser(t, 0)
	livemode := false
	topUp := &TopUp{
		UserId:              userID,
		Amount:              10,
		Money:               10,
		TradeNo:             "mysql-conc-stripe-same-event",
		PaymentMethod:       PaymentMethodStripe,
		PaymentProvider:     PaymentProviderStripe,
		Currency:            "USD",
		ExpectedAmountMinor: 1000,
		ExpectedQuota:       int(10 * common.QuotaPerUnit), // 5,000,000
		StripeLivemode:      &livemode,
		Status:              common.TopUpStatusPending,
		CreateTime:          time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	params := StripeTopUpCompletion{
		TradeNo:         topUp.TradeNo,
		SessionID:       "cs_conc_same_event",
		PaymentIntentID: "pi_conc_same_event",
		EventID:         "evt_conc_same_event",
		Livemode:        false,
		Currency:        "USD",
		PaidAmountMinor: 1000,
		CallerIP:        "127.0.0.1",
	}

	errs := runConcurrently(concurrencyGoroutines, func(int) error {
		return CompleteStripeTopUp(params)
	})
	for i, err := range errs {
		require.NoErrorf(t, err, "goroutine %d", i)
	}

	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, userID),
		"expected exactly one credit despite %d concurrent identical webhook replays", concurrencyGoroutines)
	status := getTopUpStatusForPaymentGuardTest(t, topUp.TradeNo)
	assert.Equal(t, common.TopUpStatusSuccess, status)
	logConcurrencyResult(t, "stripe_same_order_same_event", 0, 5000000, getUserQuotaForPaymentGuardTest(t, userID), status, errs)
}

// TestMySQLConcurrency_CompleteStripeTopUp_SameSessionDifferentEventsCreditedOnce
// proves Stripe event fan-out for the same Checkout Session remains
// idempotent after the first successful completion.
func TestMySQLConcurrency_CompleteStripeTopUp_SameSessionDifferentEventsCreditedOnce(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)
	common.QuotaPerUnit = 500000

	userID := insertConcurrencyTestUser(t, 0)
	livemode := false
	topUp := &TopUp{
		UserId:              userID,
		Amount:              10,
		Money:               10,
		TradeNo:             "mysql-conc-stripe-same-session",
		PaymentMethod:       PaymentMethodStripe,
		PaymentProvider:     PaymentProviderStripe,
		Currency:            "USD",
		ExpectedAmountMinor: 1000,
		ExpectedQuota:       int(10 * common.QuotaPerUnit),
		StripeSessionId:     stringPtr("cs_conc_same_session"),
		StripeLivemode:      &livemode,
		Status:              common.TopUpStatusPending,
		CreateTime:          time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	errs := runConcurrently(concurrencyGoroutines, func(idx int) error {
		return CompleteStripeTopUp(StripeTopUpCompletion{
			TradeNo:         topUp.TradeNo,
			SessionID:       "cs_conc_same_session",
			PaymentIntentID: "pi_conc_same_session",
			EventID:         fmt.Sprintf("evt_conc_same_session_%d", idx),
			Livemode:        false,
			Currency:        "USD",
			PaidAmountMinor: 1000,
			CallerIP:        "127.0.0.1",
		})
	})
	for i, err := range errs {
		require.NoErrorf(t, err, "goroutine %d", i)
	}

	status := getTopUpStatusForPaymentGuardTest(t, topUp.TradeNo)
	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, userID))
	assert.Equal(t, common.TopUpStatusSuccess, status)
	logConcurrencyResult(t, "stripe_same_session_different_events", 0, 5000000, getUserQuotaForPaymentGuardTest(t, userID), status, errs)
}

// TestMySQLConcurrency_CompleteStripeTopUp_SameEventDifferentOrdersCreditedOnce
// proves a single Stripe event id cannot be replayed across multiple local
// orders to create more than one credit.
func TestMySQLConcurrency_CompleteStripeTopUp_SameEventDifferentOrdersCreditedOnce(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)
	common.QuotaPerUnit = 500000

	userID1 := insertConcurrencyTestUser(t, 0)
	userID2 := insertConcurrencyTestUser(t, 0)
	livemode := false
	tradeNos := []string{"mysql-conc-stripe-event-order-a", "mysql-conc-stripe-event-order-b"}
	userIDs := []int{userID1, userID2}
	for idx, tradeNo := range tradeNos {
		topUp := &TopUp{
			UserId:              userIDs[idx],
			Amount:              10,
			Money:               10,
			TradeNo:             tradeNo,
			PaymentMethod:       PaymentMethodStripe,
			PaymentProvider:     PaymentProviderStripe,
			Currency:            "USD",
			ExpectedAmountMinor: 1000,
			ExpectedQuota:       int(10 * common.QuotaPerUnit),
			StripeSessionId:     stringPtr(fmt.Sprintf("cs_conc_event_order_%d", idx)),
			StripeLivemode:      &livemode,
			Status:              common.TopUpStatusPending,
			CreateTime:          time.Now().Unix(),
		}
		require.NoError(t, topUp.Insert())
	}

	errs := runConcurrently(concurrencyGoroutines, func(idx int) error {
		orderIdx := idx % len(tradeNos)
		return CompleteStripeTopUp(StripeTopUpCompletion{
			TradeNo:         tradeNos[orderIdx],
			SessionID:       fmt.Sprintf("cs_conc_event_order_%d", orderIdx),
			PaymentIntentID: "pi_conc_same_event_two_orders",
			EventID:         "evt_conc_replayed_across_orders",
			Livemode:        false,
			Currency:        "USD",
			PaidAmountMinor: 1000,
			CallerIP:        "127.0.0.1",
		})
	})

	totalQuota := getUserQuotaForPaymentGuardTest(t, userID1) + getUserQuotaForPaymentGuardTest(t, userID2)
	assert.Equal(t, 5000000, totalQuota, "only one of the two orders may be credited for a single Stripe event id")
	statusA := getTopUpStatusForPaymentGuardTest(t, tradeNos[0])
	statusB := getTopUpStatusForPaymentGuardTest(t, tradeNos[1])
	successCount, _, _, _ := concurrencyErrorStats(errs)
	assert.GreaterOrEqual(t, successCount, 1)
	assert.Contains(t, []string{common.TopUpStatusSuccess, common.TopUpStatusPending}, statusA)
	assert.Contains(t, []string{common.TopUpStatusSuccess, common.TopUpStatusPending}, statusB)
	logConcurrencyResult(t, "stripe_same_event_different_orders", 0, 5000000, totalQuota, statusA+"/"+statusB, errs)
}

// TestMySQLConcurrency_ManualCompleteTopUp_IdempotentForEpayStyleOrder covers
// the admin/epay-style manual completion path (non-Stripe provider).
func TestMySQLConcurrency_ManualCompleteTopUp_IdempotentForEpayStyleOrder(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)
	common.QuotaPerUnit = 500000

	userID := insertConcurrencyTestUser(t, 0)
	tradeNo := "mysql-conc-manual-epay"
	topUp := &TopUp{
		UserId:          userID,
		Amount:          10,
		Money:           10,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodBalance,
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	errs := runConcurrently(concurrencyGoroutines, func(int) error {
		return ManualCompleteTopUp(tradeNo, "127.0.0.1")
	})
	for i, err := range errs {
		require.NoErrorf(t, err, "goroutine %d", i)
	}

	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, userID),
		"expected exactly one credit despite %d concurrent admin manual-complete calls", concurrencyGoroutines)
	status := getTopUpStatusForPaymentGuardTest(t, tradeNo)
	assert.Equal(t, common.TopUpStatusSuccess, status)
	logConcurrencyResult(t, "epay_manual_complete_same_order", 0, 5000000, getUserQuotaForPaymentGuardTest(t, userID), status, errs)
}

// TestMySQLConcurrency_RechargeWaffo_IdempotentSameOrder covers the Waffo
// top-up completion path.
func TestMySQLConcurrency_RechargeWaffo_IdempotentSameOrder(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)
	common.QuotaPerUnit = 500000

	userID := insertConcurrencyTestUser(t, 0)
	tradeNo := "mysql-conc-waffo"
	topUp := &TopUp{
		UserId:          userID,
		Amount:          10,
		Money:           10,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodWaffo,
		PaymentProvider: PaymentProviderWaffo,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	errs := runConcurrently(concurrencyGoroutines, func(int) error {
		return RechargeWaffo(tradeNo, "127.0.0.1")
	})
	for i, err := range errs {
		require.NoErrorf(t, err, "goroutine %d", i)
	}

	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, userID),
		"expected exactly one credit despite %d concurrent Waffo callback replays", concurrencyGoroutines)
	status := getTopUpStatusForPaymentGuardTest(t, tradeNo)
	assert.Equal(t, common.TopUpStatusSuccess, status)
	logConcurrencyResult(t, "waffo_same_order", 0, 5000000, getUserQuotaForPaymentGuardTest(t, userID), status, errs)
}

// TestMySQLConcurrency_RechargeCreem_IdempotentSameOrder covers the Creem
// top-up completion path (quota = Amount directly, no multiplier).
func TestMySQLConcurrency_RechargeCreem_IdempotentSameOrder(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)

	userID := insertConcurrencyTestUser(t, 0)
	tradeNo := "mysql-conc-creem"
	topUp := &TopUp{
		UserId:          userID,
		Amount:          4200000,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodCreem,
		PaymentProvider: PaymentProviderCreem,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	errs := runConcurrently(concurrencyGoroutines, func(int) error {
		return RechargeCreem(tradeNo, "", "", "127.0.0.1")
	})
	successCount, failureCount, _, _ := concurrencyErrorStats(errs)

	assert.Equal(t, 4200000, getUserQuotaForPaymentGuardTest(t, userID),
		"expected exactly one credit despite %d concurrent Creem callback replays", concurrencyGoroutines)
	status := getTopUpStatusForPaymentGuardTest(t, tradeNo)
	assert.Equal(t, common.TopUpStatusSuccess, status)
	assert.Equal(t, 1, successCount, "exactly one Creem callback should perform the state transition")
	assert.Equal(t, concurrencyGoroutines-1, failureCount, "all other Creem callback replays should be rejected after the order is completed")
	logConcurrencyResult(t, "creem_same_order", 0, 4200000, getUserQuotaForPaymentGuardTest(t, userID), status, errs)
}

// TestMySQLConcurrency_RechargeWaffoPancake_IdempotentSameOrder covers the
// Waffo Pancake top-up completion path.
func TestMySQLConcurrency_RechargeWaffoPancake_IdempotentSameOrder(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)
	common.QuotaPerUnit = 500000

	userID := insertConcurrencyTestUser(t, 0)
	tradeNo := "mysql-conc-waffo-pancake"
	topUp := &TopUp{
		UserId:          userID,
		Amount:          10,
		Money:           10,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodWaffoPancake,
		PaymentProvider: PaymentProviderWaffoPancake,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	errs := runConcurrently(concurrencyGoroutines, func(int) error {
		return RechargeWaffoPancake(tradeNo)
	})
	for i, err := range errs {
		require.NoErrorf(t, err, "goroutine %d", i)
	}

	assert.Equal(t, 5000000, getUserQuotaForPaymentGuardTest(t, userID),
		"expected exactly one credit despite %d concurrent Waffo Pancake callback replays", concurrencyGoroutines)
	status := getTopUpStatusForPaymentGuardTest(t, tradeNo)
	assert.Equal(t, common.TopUpStatusSuccess, status)
	logConcurrencyResult(t, "waffo_pancake_same_order", 0, 5000000, getUserQuotaForPaymentGuardTest(t, userID), status, errs)
}

// TestMySQLConcurrency_Redeem_IdempotentSameCode proves a redemption code
// can only be consumed once even when many requests race to redeem it
// simultaneously (e.g. a user double-submitting or a retried request).
func TestMySQLConcurrency_Redeem_IdempotentSameCode(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)

	userID := insertConcurrencyTestUser(t, 0)
	redemption := &Redemption{
		Key:    "mysql-conc-redeem-samecode",
		Status: common.RedemptionCodeStatusEnabled,
		Quota:  1000,
	}
	require.NoError(t, redemption.Insert())

	var successCount int32
	errs := runConcurrently(concurrencyGoroutines, func(int) error {
		_, err := Redeem(redemption.Key, userID)
		if err == nil {
			atomic.AddInt32(&successCount, 1)
		}
		return err
	})

	failureCount := 0
	for _, err := range errs {
		if err != nil {
			failureCount++
		}
	}

	assert.Equal(t, 1, int(successCount), "exactly one concurrent redemption should succeed")
	assert.Equal(t, concurrencyGoroutines-1, failureCount, "all other concurrent attempts should be rejected as already-used")
	assert.Equal(t, 1000, getUserQuotaForPaymentGuardTest(t, userID),
		"expected exactly one credit despite %d concurrent redemptions of the same code", concurrencyGoroutines)
	logConcurrencyResult(t, "redemption_same_code", 0, 1000, getUserQuotaForPaymentGuardTest(t, userID), "used", errs)
}

// TestMySQLConcurrency_CompleteSubscriptionOrder_IdempotentSameOrder proves
// concurrent webhook replays for the same subscription order create exactly
// one UserSubscription, not one per replay.
func TestMySQLConcurrency_CompleteSubscriptionOrder_IdempotentSameOrder(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)

	userID := insertConcurrencyTestUser(t, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, nextConcurrencyTestID())
	tradeNo := "mysql-conc-sub-order"
	insertSubscriptionOrderForPaymentGuardTest(t, tradeNo, userID, plan.Id, PaymentProviderStripe)

	errs := runConcurrently(concurrencyGoroutines, func(int) error {
		return CompleteSubscriptionOrder(tradeNo, "", "", "")
	})
	for i, err := range errs {
		require.NoErrorf(t, err, "goroutine %d", i)
	}

	assert.EqualValues(t, 1, countUserSubscriptionsForPaymentGuardTest(t, userID),
		"expected exactly one UserSubscription despite %d concurrent order completions", concurrencyGoroutines)

	order := GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	logConcurrencyResult(t, "subscription_same_order", 0, 0, getUserQuotaForPaymentGuardTest(t, userID), order.Status, errs)
}

// TestMySQLConcurrency_TransferAffQuotaToQuota_IdempotentSameUser proves the
// affiliate-balance transfer path uses a real row lock and re-checks the
// balance under that lock before moving quota.
func TestMySQLConcurrency_TransferAffQuotaToQuota_IdempotentSameUser(t *testing.T) {
	setupMySQLConcurrencyTestDB(t)
	common.QuotaPerUnit = 1000

	initialAffQuota := 8000
	userID := insertConcurrencyTestUserWithAffQuota(t, 0, initialAffQuota)

	var successCount int32
	errs := runConcurrently(concurrencyGoroutines, func(int) error {
		user := &User{Id: userID}
		err := user.TransferAffQuotaToQuota(initialAffQuota)
		if err == nil {
			atomic.AddInt32(&successCount, 1)
		}
		return err
	})

	finalUser := getConcurrencyTestUser(t, userID)
	assert.Equal(t, 1, int(successCount), "exactly one concurrent affiliate quota transfer should succeed")
	assert.Equal(t, initialAffQuota, finalUser.Quota, "quota should increase exactly once")
	assert.Equal(t, 0, finalUser.AffQuota, "affiliate quota should be fully transferred once and never go negative")
	assert.GreaterOrEqual(t, finalUser.AffQuota, 0)
	logConcurrencyResult(t, "aff_quota_transfer_same_user", 0, initialAffQuota, finalUser.Quota, fmt.Sprintf("aff_quota=%d", finalUser.AffQuota), errs)
}
