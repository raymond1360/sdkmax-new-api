package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

const StripeOrphanSessionFailureStageAttachRecoveryExpire = "attach_recovery_expire"

var stripeAuditEmailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
var stripeOrphanFallbackMu sync.Mutex
var stripeOrphanFallbackDirForTest string

const (
	stripeOrphanFallbackFileName    = "stripe_orphan_session_fallback.jsonl"
	stripeOrphanFallbackMaxBytes    = 10 * 1024 * 1024
	stripeOrphanFallbackMaxArchives = 5
)

type StripeOrphanSessionAudit struct {
	Id                  int    `json:"id"`
	TradeNo             string `json:"trade_no" gorm:"type:varchar(191);not null;default:'';index:idx_stripe_orphan_audits_trade_no"`
	UserId              int    `json:"user_id" gorm:"type:int;not null;default:0;index:idx_stripe_orphan_audits_user_id"`
	StripeSessionId     string `json:"-" gorm:"type:varchar(191);not null;default:'';index:idx_stripe_orphan_audits_session_id"`
	StripeLivemode      bool   `json:"stripe_livemode" gorm:"not null;default:0"`
	FailureStage        string `json:"failure_stage" gorm:"type:varchar(64);not null;default:''"`
	AttachErrorCode     string `json:"attach_error_code" gorm:"type:varchar(128);not null;default:''"`
	RecoveryErrorCode   string `json:"recovery_error_code" gorm:"type:varchar(128);not null;default:''"`
	ExpireErrorCode     string `json:"expire_error_code" gorm:"type:varchar(128);not null;default:''"`
	Resolved            bool   `json:"resolved" gorm:"not null;default:0;index:idx_stripe_orphan_audits_resolved"`
	ResolvedAt          int64  `json:"resolved_at" gorm:"type:bigint;not null;default:0"`
	ResolutionNote      string `json:"resolution_note" gorm:"type:varchar(255);not null;default:''"`
	CreatedAt           int64  `json:"created_at" gorm:"autoCreateTime;type:bigint;not null;default:0;index:idx_stripe_orphan_audits_created_at"`
	UpdatedAt           int64  `json:"updated_at" gorm:"autoUpdateTime;type:bigint;not null;default:0"`
	MaskedStripeSession string `json:"stripe_session_id" gorm:"-:all"`
}

type StripeOrphanSessionAuditInput struct {
	UserId            int
	TradeNo           string
	StripeSessionId   string
	StripeLivemode    bool
	FailureStage      string
	AttachErrorCode   string
	RecoveryErrorCode string
	ExpireErrorCode   string
}

func RecordStripeOrphanSessionAudit(input StripeOrphanSessionAuditInput) error {
	if input.TradeNo == "" || input.StripeSessionId == "" {
		return nil
	}
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	auditDB := DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)})
	return auditDB.Transaction(func(tx *gorm.DB) error {
		userID := stripeOrphanAuditUserIDFromTx(tx, input.TradeNo, input.UserId)
		failureStage := safeStripeAuditText(input.FailureStage, 64)
		if failureStage == "" {
			failureStage = StripeOrphanSessionFailureStageAttachRecoveryExpire
		}
		updates := map[string]interface{}{
			"user_id":             userID,
			"attach_error_code":   safeStripeAuditText(input.AttachErrorCode, 128),
			"recovery_error_code": safeStripeAuditText(input.RecoveryErrorCode, 128),
			"expire_error_code":   safeStripeAuditText(input.ExpireErrorCode, 128),
			"resolution_note":     "",
			"stripe_session_id":   input.StripeSessionId,
			"stripe_livemode":     input.StripeLivemode,
			"failure_stage":       failureStage,
			"resolved":            false,
			"resolved_at":         int64(0),
		}
		var existing StripeOrphanSessionAudit
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("trade_no = ? AND stripe_session_id = ? AND failure_stage = ? AND resolved = ?", input.TradeNo, input.StripeSessionId, failureStage, false).
			First(&existing).Error
		if err == nil {
			return tx.Model(&existing).Updates(updates).Error
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		audit := &StripeOrphanSessionAudit{
			TradeNo:           input.TradeNo,
			UserId:            userID,
			StripeSessionId:   input.StripeSessionId,
			StripeLivemode:    input.StripeLivemode,
			FailureStage:      failureStage,
			AttachErrorCode:   updates["attach_error_code"].(string),
			RecoveryErrorCode: updates["recovery_error_code"].(string),
			ExpireErrorCode:   updates["expire_error_code"].(string),
			Resolved:          false,
		}
		return tx.Create(audit).Error
	})
}

type StripeOrphanSessionFallbackRecord struct {
	Version           int    `json:"version"`
	Type              string `json:"type"`
	CreatedAt         int64  `json:"created_at"`
	TradeNo           string `json:"trade_no"`
	UserId            int    `json:"user_id"`
	StripeSessionId   string `json:"stripe_session_id"`
	StripeLivemode    bool   `json:"stripe_livemode"`
	FailureStage      string `json:"failure_stage"`
	AttachErrorCode   string `json:"attach_error_code"`
	RecoveryErrorCode string `json:"recovery_error_code"`
	ExpireErrorCode   string `json:"expire_error_code"`
	AuditErrorCode    string `json:"audit_error_code"`
	Resolved          bool   `json:"resolved"`
}

func RecordStripeOrphanSessionFallbackFile(input StripeOrphanSessionAuditInput, auditErr error) error {
	if input.TradeNo == "" || input.StripeSessionId == "" {
		return nil
	}
	dir, err := stripeOrphanSessionFallbackDir()
	if err != nil {
		return err
	}
	userID := input.UserId
	if userID <= 0 && DB != nil {
		auditDB := DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)})
		userID = stripeOrphanAuditUserIDFromTx(auditDB, input.TradeNo, 0)
	}
	record := StripeOrphanSessionFallbackRecord{
		Version:           1,
		Type:              "orphan_session",
		CreatedAt:         common.GetTimestamp(),
		TradeNo:           input.TradeNo,
		UserId:            userID,
		StripeSessionId:   input.StripeSessionId,
		StripeLivemode:    input.StripeLivemode,
		FailureStage:      safeStripeAuditText(input.FailureStage, 64),
		AttachErrorCode:   safeStripeAuditText(input.AttachErrorCode, 128),
		RecoveryErrorCode: safeStripeAuditText(input.RecoveryErrorCode, 128),
		ExpireErrorCode:   safeStripeAuditText(input.ExpireErrorCode, 128),
		AuditErrorCode:    safeStripeAuditText(errorText(auditErr), 128),
		Resolved:          false,
	}
	if record.FailureStage == "" {
		record.FailureStage = StripeOrphanSessionFailureStageAttachRecoveryExpire
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	stripeOrphanFallbackMu.Lock()
	defer stripeOrphanFallbackMu.Unlock()

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0700)
	path := filepath.Join(dir, stripeOrphanFallbackFileName)
	if err := ensureStripeFallbackPathSafe(dir, path); err != nil {
		return err
	}
	if err := rotateStripeOrphanFallbackFile(path, int64(len(line))); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_ = file.Chmod(0600)
	if _, err := file.Write(line); err != nil {
		return err
	}
	return file.Sync()
}

func stripeOrphanAuditUserIDFromTx(tx *gorm.DB, tradeNo string, fallback int) int {
	if fallback > 0 || tx == nil || tradeNo == "" {
		return fallback
	}
	var topUp TopUp
	if err := tx.Select("user_id").Where(tradeNoColumn()+" = ?", tradeNo).First(&topUp).Error; err == nil {
		return topUp.UserId
	}
	return fallback
}

func stripeOrphanSessionFallbackDir() (string, error) {
	if stripeOrphanFallbackDirForTest != "" {
		return filepath.Abs(stripeOrphanFallbackDirForTest)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if filepath.Base(filepath.Clean(cwd)) != "data" {
		return "", errors.New("persistent data directory is not confirmed")
	}
	return filepath.Abs(cwd)
}

func ensureStripeFallbackPathSafe(dir string, path string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	cleanDir := filepath.Clean(absDir)
	cleanPath := filepath.Clean(absPath)
	if filepath.Dir(cleanPath) != cleanDir {
		return errors.New("fallback path escapes data directory")
	}
	lower := strings.ToLower(filepath.ToSlash(cleanPath))
	for _, unsafePart := range []string{"/web/", "/public/", "/static/"} {
		if strings.Contains(lower, unsafePart) {
			return fmt.Errorf("fallback path is in web-accessible directory: %s", unsafePart)
		}
	}
	return nil
}

func rotateStripeOrphanFallbackFile(path string, nextLineBytes int64) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Size()+nextLineBytes <= stripeOrphanFallbackMaxBytes {
		return nil
	}
	lastArchive := fmt.Sprintf("%s.%d", path, stripeOrphanFallbackMaxArchives)
	if _, err := os.Stat(lastArchive); err == nil {
		return errors.New("stripe orphan fallback retention limit reached")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for idx := stripeOrphanFallbackMaxArchives - 1; idx >= 1; idx-- {
		oldPath := fmt.Sprintf("%s.%d", path, idx)
		newPath := fmt.Sprintf("%s.%d", path, idx+1)
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Rename(oldPath, newPath); err != nil {
				return err
			}
			_ = os.Chmod(newPath, 0600)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return err
	}
	_ = os.Chmod(path+".1", 0600)
	return nil
}

func ResolveStripeOrphanSessionAudit(tradeNo string, sessionID string, note string) error {
	if tradeNo == "" || sessionID == "" {
		return nil
	}
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	auditDB := DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)})
	return auditDB.Model(&StripeOrphanSessionAudit{}).
		Where("trade_no = ? AND stripe_session_id = ? AND resolved = ?", tradeNo, sessionID, false).
		Updates(map[string]interface{}{
			"resolved":        true,
			"resolved_at":     common.GetTimestamp(),
			"resolution_note": safeStripeAuditText(note, 255),
		}).Error
}

func GetStripeOrphanSessionAudits(pageInfo *common.PageInfo, tradeNo string, resolved *bool, createdAfter int64, createdBefore int64) (audits []*StripeOrphanSessionAudit, total int64, err error) {
	query := DB.Model(&StripeOrphanSessionAudit{})
	if tradeNo != "" {
		query = query.Where("trade_no = ?", tradeNo)
	}
	if resolved != nil {
		query = query.Where("resolved = ?", *resolved)
	}
	if createdAfter > 0 {
		query = query.Where("created_at >= ?", createdAfter)
	}
	if createdBefore > 0 {
		query = query.Where("created_at <= ?", createdBefore)
	}
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&audits).Error
	for _, audit := range audits {
		audit.MaskSensitiveFields()
	}
	return audits, total, err
}

func (audit *StripeOrphanSessionAudit) MaskSensitiveFields() {
	if audit == nil {
		return
	}
	audit.MaskedStripeSession = MaskStripeIdentifier(audit.StripeSessionId)
	audit.StripeSessionId = ""
}

func safeStripeAuditText(value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" || max <= 0 {
		return ""
	}
	replacements := []string{
		"Stripe-Signature", "redacted_header",
		"stripe-signature", "redacted_header",
		"webhook body", "redacted_body",
		"Webhook Body", "redacted_body",
		"card", "payment_detail",
		"email", "contact_detail",
		"sk_live_", "sk-redacted-",
		"sk_test_", "sk-redacted-",
		"whsec_", "whsec-redacted-",
	}
	value = strings.NewReplacer(replacements...).Replace(value)
	value = stripeAuditEmailPattern.ReplaceAllString(value, "redacted_contact")
	return truncateString(value, max)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func SetStripeOrphanSessionFallbackDirForTest(dir string) func() {
	old := stripeOrphanFallbackDirForTest
	stripeOrphanFallbackDirForTest = dir
	return func() {
		stripeOrphanFallbackDirForTest = old
	}
}
