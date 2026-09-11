package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TopUp struct {
	Id                      int     `json:"id"`
	UserId                  int     `json:"user_id" gorm:"index"`
	Amount                  int64   `json:"amount"`
	Money                   float64 `json:"money"`
	TradeNo                 string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod           string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider         string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	Currency                string  `json:"currency" gorm:"type:varchar(8);default:'';index"`
	ExpectedAmountMinor     int64   `json:"expected_amount_minor" gorm:"default:0"`
	PaidAmountMinor         int64   `json:"paid_amount_minor" gorm:"default:0"`
	ExpectedQuota           int     `json:"expected_quota" gorm:"default:0"`
	StripeSessionId         *string `json:"stripe_session_id,omitempty" gorm:"type:varchar(191);uniqueIndex"`
	StripeRecoverySessionId *string `json:"-" gorm:"type:varchar(191);index"`
	StripePaymentIntentId   *string `json:"stripe_payment_intent_id,omitempty" gorm:"type:varchar(191);index"`
	StripeEventId           *string `json:"stripe_event_id,omitempty" gorm:"type:varchar(191);uniqueIndex"`
	StripeLivemode          *bool   `json:"stripe_livemode,omitempty"`
	ProviderPayloadDigest   *string `json:"provider_payload_digest,omitempty" gorm:"type:varchar(128)"`
	PaymentError            *string `json:"payment_error,omitempty" gorm:"type:varchar(255)"`
	CreateTime              int64   `json:"create_time"`
	CompleteTime            int64   `json:"complete_time"`
	Status                  string  `json:"status"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
)

var (
	ErrPaymentMethodMismatch  = errors.New("payment method mismatch")
	ErrTopUpNotFound          = errors.New("topup not found")
	ErrTopUpStatusInvalid     = errors.New("topup status invalid")
	ErrStripeTopUpMismatch    = errors.New("stripe topup mismatch")
	ErrStripeEventReplayed    = errors.New("stripe event replayed")
	ErrStripeTopUpUnbound     = errors.New("stripe topup missing session binding")
	ErrStripeTopUpBadAmount   = errors.New("stripe topup amount mismatch")
	ErrStripeTopUpBadCurrency = errors.New("stripe topup currency mismatch")
	ErrStripeTopUpBadMode     = errors.New("stripe topup livemode mismatch")
)

type StripeTopUpCompletion struct {
	TradeNo               string
	SessionID             string
	PaymentIntentID       string
	EventID               string
	Livemode              bool
	Currency              string
	PaidAmountMinor       int64
	CustomerID            string
	CallerIP              string
	ProviderPayloadDigest string
}

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func AttachStripeSessionToTopUp(tradeNo string, sessionID string) error {
	if tradeNo == "" || sessionID == "" {
		return errors.New("missing stripe order binding")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(tradeNoColumn()+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if topUp.StripeSessionId != nil && *topUp.StripeSessionId != sessionID {
			return ErrStripeTopUpMismatch
		}
		topUp.StripeSessionId = stringPtr(sessionID)
		topUp.StripeRecoverySessionId = nil
		topUp.PaymentError = nil
		return tx.Save(topUp).Error
	})
}

func RecordStripeSessionRecovery(tradeNo string, sessionID string, reason string) error {
	if tradeNo == "" || sessionID == "" {
		return errors.New("missing stripe recovery binding")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(tradeNoColumn()+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}
		if topUp.StripeSessionId != nil && *topUp.StripeSessionId != "" && *topUp.StripeSessionId != sessionID {
			return ErrStripeTopUpMismatch
		}
		topUp.StripeRecoverySessionId = stringPtr(sessionID)
		if reason != "" {
			topUp.PaymentError = stringPtr(truncateString(reason, 255))
		}
		return tx.Save(topUp).Error
	})
}

func MarkStripeTopUpFailed(tradeNo string, sessionID string, eventID string, livemode bool, targetStatus string, reason string) error {
	if tradeNo == "" {
		return errors.New("missing stripe order id")
	}
	if targetStatus == "" {
		targetStatus = common.TopUpStatusFailed
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(tradeNoColumn()+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if sessionID != "" {
			if topUp.StripeSessionId != nil && *topUp.StripeSessionId != "" && *topUp.StripeSessionId != sessionID {
				return ErrStripeTopUpMismatch
			}
			if topUp.StripeSessionId == nil || *topUp.StripeSessionId == "" {
				topUp.StripeRecoverySessionId = stringPtr(sessionID)
			}
		}
		if topUp.StripeLivemode == nil || *topUp.StripeLivemode != livemode {
			return ErrStripeTopUpBadMode
		}
		if eventID != "" {
			topUp.StripeEventId = stringPtr(eventID)
		}
		if reason != "" {
			topUp.PaymentError = stringPtr(truncateString(reason, 255))
		}
		topUp.Status = targetStatus
		topUp.CompleteTime = common.GetTimestamp()
		return tx.Save(topUp).Error
	})
}

func CompleteStripeTopUp(params StripeTopUpCompletion) error {
	if params.TradeNo == "" || params.SessionID == "" || params.EventID == "" {
		return errors.New("missing stripe completion identifiers")
	}
	var quotaToAdd int
	var topUpUserID int
	var topUpAmount int64
	var topUpMoney float64
	err := DB.Transaction(func(tx *gorm.DB) error {
		if params.EventID != "" {
			var existing TopUp
			err := tx.Where("stripe_event_id = ? AND trade_no <> ?", params.EventID, params.TradeNo).First(&existing).Error
			if err == nil {
				return ErrStripeEventReplayed
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		topUp := &TopUp{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(tradeNoColumn()+" = ?", params.TradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending &&
			topUp.Status != common.TopUpStatusFailed &&
			topUp.Status != common.TopUpStatusExpired {
			return ErrTopUpStatusInvalid
		}
		if topUp.StripeSessionId != nil && *topUp.StripeSessionId != "" && *topUp.StripeSessionId != params.SessionID {
			return ErrStripeTopUpMismatch
		}
		if topUp.StripeSessionId == nil || *topUp.StripeSessionId == "" {
			if topUp.StripeRecoverySessionId != nil && *topUp.StripeRecoverySessionId != "" && *topUp.StripeRecoverySessionId != params.SessionID {
				return ErrStripeTopUpMismatch
			}
			topUp.StripeSessionId = stringPtr(params.SessionID)
			topUp.StripeRecoverySessionId = nil
		}
		if topUp.StripeLivemode == nil || *topUp.StripeLivemode != params.Livemode {
			return ErrStripeTopUpBadMode
		}
		if topUp.Currency != "USD" || params.Currency != "USD" {
			return ErrStripeTopUpBadCurrency
		}
		if topUp.ExpectedAmountMinor <= 0 || topUp.ExpectedAmountMinor != params.PaidAmountMinor {
			return ErrStripeTopUpBadAmount
		}

		quotaToAdd = topUp.ExpectedQuota
		if quotaToAdd <= 0 {
			quotaToAdd = int(decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		}
		if quotaToAdd <= 0 {
			return errors.New("invalid topup quota")
		}

		topUp.PaidAmountMinor = params.PaidAmountMinor
		topUp.StripePaymentIntentId = nullableString(params.PaymentIntentID)
		topUp.StripeEventId = stringPtr(params.EventID)
		topUp.ProviderPayloadDigest = nullableString(params.ProviderPayloadDigest)
		topUp.PaymentError = nil
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{"quota": gorm.Expr("quota + ?", quotaToAdd)}
		if params.CustomerID != "" {
			updates["stripe_customer"] = params.CustomerID
		}
		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updates).Error; err != nil {
			return err
		}
		topUpUserID = topUp.UserId
		topUpAmount = topUp.Amount
		topUpMoney = topUp.Money
		return nil
	})
	if err != nil {
		common.SysError("stripe topup failed: " + err.Error())
		return err
	}
	if quotaToAdd > 0 {
		RecordTopupLog(topUpUserID, fmt.Sprintf("Stripe topup succeeded, credited %v, requested amount: %d, credit amount USD: %.2f", logger.FormatQuota(quotaToAdd), topUpAmount, topUpMoney), params.CallerIP, PaymentMethodStripe, PaymentProviderStripe)
	}
	return nil
}

func tradeNoColumn() string {
	if common.UsingPostgreSQL {
		return `"trade_no"`
	}
	return "`trade_no`"
}

func stringPtr(value string) *string {
	return &value
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func MaskStripeIdentifier(id string) string {
	if len(id) <= 10 {
		return id
	}
	return id[:6] + "..." + id[len(id)-4:]
}

func IsPermanentStripeTopUpError(err error) bool {
	return errors.Is(err, ErrPaymentMethodMismatch) ||
		errors.Is(err, ErrTopUpNotFound) ||
		errors.Is(err, ErrTopUpStatusInvalid) ||
		errors.Is(err, ErrStripeTopUpMismatch) ||
		errors.Is(err, ErrStripeEventReplayed) ||
		errors.Is(err, ErrStripeTopUpUnbound) ||
		errors.Is(err, ErrStripeTopUpBadAmount) ||
		errors.Is(err, ErrStripeTopUpBadCurrency) ||
		errors.Is(err, ErrStripeTopUpBadMode)
}

func truncateString(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota float64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quota = topUp.Money * common.QuotaPerUnit
		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(map[string]interface{}{"stripe_customer": customerId, "quota": gorm.Expr("quota + ?", quota)}).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(int(quota)), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)

	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.PaymentProvider == PaymentProviderStripe {
			return errors.New("Stripe 订单必须通过 Stripe webhook 或专用 Stripe 凭证核验补单")
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表经分组倍率换算后的美元数量，直接 * QuotaPerUnit
		// - 其他订单（如易支付）：Amount 为美元数量，* QuotaPerUnit
		if topUp.PaymentProvider == PaymentProviderStripe {
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(decimal.NewFromFloat(topUp.Money).Mul(dQuotaPerUnit).IntPart())
		} else {
			dAmount := decimal.NewFromInt(topUp.Amount)
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		}
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		return nil
	})

	if err != nil {
		return err
	}

	// 事务外记录日志，避免阻塞
	RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), callerIp, paymentMethod, "admin")
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota = topUp.Amount

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]interface{}{
			"quota": gorm.Expr("quota + ?", quota),
		}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updateFields).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		quotaToAdd = int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
	}

	return nil
}
