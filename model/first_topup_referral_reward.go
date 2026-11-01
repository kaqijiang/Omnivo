package model

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
)

// FirstTopUpReferralReward permanently identifies the paid order that earned
// an inviter a reward. Keeping it separate from payment history also prevents
// a later history cleanup from making an invitee eligible again.
type FirstTopUpReferralReward struct {
	InviteeID int    `json:"invitee_id" gorm:"primaryKey;autoIncrement:false"`
	TopUpID   int    `json:"top_up_id" gorm:"index;not null"`
	TradeNo   string `json:"trade_no" gorm:"uniqueIndex;type:varchar(255);not null"`
	InviterID int    `json:"inviter_id" gorm:"index;not null"`
	Quota     int    `json:"quota" gorm:"type:bigint;not null"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime;not null"`
}

var ErrReferralRewardBalanceLimit error = common.NewMessage("Referral reward balance limit exceeded")

// awardFirstTopUpReferralRewardTx must run after the pending order is locked
// and before any non-locking query in its settlement transaction. The payer
// lock serializes different orders for one user; taking it before a consistent
// read also ensures MySQL's repeatable-read snapshot includes prior payments.
// The reward, unique ledger entry, payment status and principal credit commit
// together. Callers only log the returned reward after the commit succeeds.
func awardFirstTopUpReferralRewardTx(tx *gorm.DB, topUp *TopUp) (*FirstTopUpReferralReward, error) {
	rewardUSD := operation_setting.GetPaymentSetting().FirstTopUpReferralRewardUSD
	if rewardUSD == 0 || !operation_setting.IsPaymentComplianceConfirmed() {
		return nil, nil
	}
	if topUp.Amount <= 0 || topUp.Money <= 0 || math.IsNaN(topUp.Money) || math.IsInf(topUp.Money, 0) ||
		topUp.PaymentProvider == PaymentProviderBalance || topUp.Status != common.TopUpStatusPending ||
		(topUp.PaymentReceived != nil && !*topUp.PaymentReceived) {
		return nil, nil
	}
	switch topUp.PaymentProvider {
	case PaymentProviderEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake:
	case "":
		// Older paid orders did not persist a provider; recognize the built-in
		// methods and the administrator's configured gateway methods.
		switch topUp.PaymentMethod {
		case PaymentMethodStripe, PaymentMethodCreem, PaymentMethodWaffo, PaymentMethodWaffoPancake, "alipay", "wxpay":
		default:
			if !operation_setting.ContainsPayMethod(topUp.PaymentMethod) {
				return nil, nil
			}
		}
	default:
		return nil, nil
	}

	var payer User
	if err := lockForUpdate(tx).Select("id", "inviter_id").Where("id = ?", topUp.UserId).First(&payer).Error; err != nil {
		return nil, err
	}
	if payer.InviterId <= 0 || payer.InviterId == payer.Id {
		return nil, nil
	}
	var existingReward FirstTopUpReferralReward
	err := tx.Select("invitee_id").Where("invitee_id = ?", payer.Id).Take(&existingReward).Error
	if err == nil {
		return nil, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var priorTopUp TopUp
	err = tx.Select("id").Where(
		"user_id = ? AND id <> ? AND status = ? AND amount > 0 AND money > 0 AND (payment_provider IS NULL OR payment_provider <> ?) AND (payment_received IS NULL OR payment_received = ?)",
		payer.Id, topUp.Id, common.TopUpStatusSuccess, PaymentProviderBalance, true,
	).Take(&priorTopUp).Error
	if err == nil {
		return nil, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var inviter User
	err = lockForUpdate(tx).Select("id", "status", "aff_quota", "aff_history").
		Where("id = ?", payer.InviterId).First(&inviter).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && inviter.Status != common.UserStatusEnabled) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	quota, err := operation_setting.FirstTopUpReferralRewardQuota(rewardUSD)
	if err != nil {
		return nil, err
	}

	reward := &FirstTopUpReferralReward{
		InviteeID: payer.Id,
		TopUpID:   topUp.Id,
		TradeNo:   topUp.TradeNo,
		InviterID: inviter.Id,
		Quota:     quota,
		CreatedAt: common.GetTimestamp(),
	}
	// The payer lock and existing-ledger read establish idempotency. A plain
	// insert retains the unique constraint as a backstop without depending on
	// MySQL's clientFoundRows treatment of an ON DUPLICATE KEY no-op.
	if err := tx.Create(reward).Error; err != nil {
		return nil, err
	}

	maxCurrentQuota := common.MaxWalletQuota - quota
	result := tx.Model(&User{}).
		Where("id = ? AND status = ? AND aff_quota >= 0 AND aff_quota <= ? AND aff_history >= 0 AND aff_history <= ?",
			inviter.Id, common.UserStatusEnabled, maxCurrentQuota, maxCurrentQuota).
		Updates(map[string]any{
			"aff_quota":   gorm.Expr("aff_quota + ?", quota),
			"aff_history": gorm.Expr("aff_history + ?", quota),
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrReferralRewardBalanceLimit
	}
	return reward, nil
}

func recordFirstTopUpReferralReward(reward *FirstTopUpReferralReward) {
	if reward == nil {
		return
	}
	RecordLog(reward.InviterID, LogTypeSystem, common.NewMessage(
		"First top-up referral reward {{quota}} for inviting user {{user_id}}",
		map[string]any{"quota": logger.FormatQuota(reward.Quota), "user_id": reward.InviteeID},
	))
}
