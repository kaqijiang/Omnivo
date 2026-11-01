package operation_setting

import (
	"math"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/shopspring/decimal"
)

type PaymentSetting struct {
	AmountOptions               []int           `json:"amount_options"`
	AmountDiscount              map[int]float64 `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠
	FirstTopUpReferralRewardUSD float64         `json:"first_topup_referral_reward_usd"`

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`
}

const CurrentComplianceTermsVersion = "v1"
const FirstTopUpReferralRewardOption = "payment_setting.first_topup_referral_reward_usd"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:               []int{10, 20, 50, 100, 200, 500},
	AmountDiscount:              map[int]float64{},
	FirstTopUpReferralRewardUSD: 5,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}

// FirstTopUpReferralRewardQuota converts the configured USD credit without
// applying model or payment-group multipliers. Zero disables the reward.
func FirstTopUpReferralRewardQuota(usd float64) (int, error) {
	if math.IsNaN(usd) || math.IsInf(usd, 0) || usd < 0 {
		return 0, common.NewMessage("Invalid first top-up referral reward")
	}
	if usd == 0 {
		return 0, nil
	}
	if math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || common.QuotaPerUnit <= 0 {
		return 0, common.NewMessage("Invalid first top-up referral reward")
	}
	quota, err := common.WalletQuotaFromDecimalStrict(decimal.NewFromFloat(usd).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	if err != nil || quota <= 0 {
		return 0, common.NewMessage("Invalid first top-up referral reward")
	}
	return quota, nil
}

func ValidateFirstTopUpReferralReward(value string) error {
	usd, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return common.NewMessage("Invalid first top-up referral reward")
	}
	_, err = FirstTopUpReferralRewardQuota(usd)
	return err
}
