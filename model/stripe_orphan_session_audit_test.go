package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordStripeOrphanSessionAuditStoresFullSessionID(t *testing.T) {
	truncateTables(t)
	userID := 9201
	insertUserForPaymentGuardTest(t, userID, 0)
	livemode := false
	topUp := &TopUp{
		UserId:              userID,
		Amount:              10,
		Money:               10,
		TradeNo:             "ref_orphan_full_session",
		PaymentMethod:       PaymentMethodStripe,
		PaymentProvider:     PaymentProviderStripe,
		Currency:            "USD",
		ExpectedAmountMinor: 1000,
		ExpectedQuota:       5000000,
		StripeLivemode:      &livemode,
		Status:              common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	require.NoError(t, RecordStripeOrphanSessionAudit(StripeOrphanSessionAuditInput{
		TradeNo:           topUp.TradeNo,
		StripeSessionId:   "cs_test_orphan_full_session_1234567890",
		StripeLivemode:    false,
		FailureStage:      StripeOrphanSessionFailureStageAttachRecoveryExpire,
		AttachErrorCode:   "attach failed",
		RecoveryErrorCode: "recovery failed",
		ExpireErrorCode:   "expire failed",
	}))

	var audit StripeOrphanSessionAudit
	require.NoError(t, DB.Where("trade_no = ?", topUp.TradeNo).First(&audit).Error)
	assert.Equal(t, "cs_test_orphan_full_session_1234567890", audit.StripeSessionId)
	assert.Equal(t, userID, audit.UserId)
	assert.False(t, audit.Resolved)
}

func TestRecordStripeOrphanSessionAuditDeduplicatesUnresolvedPath(t *testing.T) {
	truncateTables(t)

	input := StripeOrphanSessionAuditInput{
		TradeNo:           "ref_orphan_dedup",
		StripeSessionId:   "cs_test_orphan_dedup_1234567890",
		FailureStage:      StripeOrphanSessionFailureStageAttachRecoveryExpire,
		AttachErrorCode:   "attach failed first",
		RecoveryErrorCode: "recovery failed first",
		ExpireErrorCode:   "expire failed first",
	}
	require.NoError(t, RecordStripeOrphanSessionAudit(input))
	input.ExpireErrorCode = "expire failed second"
	require.NoError(t, RecordStripeOrphanSessionAudit(input))

	var count int64
	require.NoError(t, DB.Model(&StripeOrphanSessionAudit{}).Where("trade_no = ?", input.TradeNo).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	var audit StripeOrphanSessionAudit
	require.NoError(t, DB.Where("trade_no = ?", input.TradeNo).First(&audit).Error)
	assert.Equal(t, "expire failed second", audit.ExpireErrorCode)
}

func TestStripeOrphanSessionAuditListMasksSessionID(t *testing.T) {
	truncateTables(t)
	sessionID := "cs_test_masked_orphan_session_1234567890"
	require.NoError(t, RecordStripeOrphanSessionAudit(StripeOrphanSessionAuditInput{
		TradeNo:           "ref_orphan_masked",
		StripeSessionId:   sessionID,
		FailureStage:      StripeOrphanSessionFailureStageAttachRecoveryExpire,
		AttachErrorCode:   "attach failed",
		RecoveryErrorCode: "recovery failed",
		ExpireErrorCode:   "expire failed",
	}))

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	audits, total, err := GetStripeOrphanSessionAudits(pageInfo, "", nil, 0, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, audits, 1)
	assert.Empty(t, audits[0].StripeSessionId)
	assert.Equal(t, MaskStripeIdentifier(sessionID), audits[0].MaskedStripeSession)
	assert.NotContains(t, audits[0].MaskedStripeSession, "session_123456")
}

func TestStripeOrphanSessionAuditSanitizesSensitiveText(t *testing.T) {
	truncateTables(t)

	require.NoError(t, RecordStripeOrphanSessionAudit(StripeOrphanSessionAuditInput{
		TradeNo:           "ref_orphan_sanitize",
		StripeSessionId:   "cs_test_orphan_sanitize_1234567890",
		FailureStage:      StripeOrphanSessionFailureStageAttachRecoveryExpire,
		AttachErrorCode:   "sk_test_sensitive Stripe-Signature buyer@example.test card 4242",
		RecoveryErrorCode: "whsec_sensitive webhook body",
		ExpireErrorCode:   strings.Repeat("x", 300),
	}))

	var audit StripeOrphanSessionAudit
	require.NoError(t, DB.Where("trade_no = ?", "ref_orphan_sanitize").First(&audit).Error)
	combined := audit.AttachErrorCode + " " + audit.RecoveryErrorCode + " " + audit.ExpireErrorCode
	assert.NotContains(t, combined, "sk_test")
	assert.NotContains(t, combined, "whsec_")
	assert.NotContains(t, combined, "Stripe-Signature")
	assert.NotContains(t, combined, "webhook body")
	assert.NotContains(t, combined, "buyer@example.test")
	assert.NotContains(t, combined, "card")
	assert.LessOrEqual(t, len(audit.ExpireErrorCode), 128)
}

func TestResolveStripeOrphanSessionAudit(t *testing.T) {
	truncateTables(t)
	input := StripeOrphanSessionAuditInput{
		TradeNo:           "ref_orphan_resolve",
		StripeSessionId:   "cs_test_orphan_resolve_1234567890",
		FailureStage:      StripeOrphanSessionFailureStageAttachRecoveryExpire,
		AttachErrorCode:   "attach failed",
		RecoveryErrorCode: "recovery failed",
		ExpireErrorCode:   "expire failed",
	}
	require.NoError(t, RecordStripeOrphanSessionAudit(input))

	require.NoError(t, ResolveStripeOrphanSessionAudit(input.TradeNo, input.StripeSessionId, "resolved by webhook"))

	var audit StripeOrphanSessionAudit
	require.NoError(t, DB.Where("trade_no = ?", input.TradeNo).First(&audit).Error)
	assert.True(t, audit.Resolved)
	assert.NotZero(t, audit.ResolvedAt)
	assert.Equal(t, "resolved by webhook", audit.ResolutionNote)
}
