package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

func TestPaidCheckoutReferralRewardUsesConfirmedPayment(t *testing.T) {
	previousPayment := *operation_setting.GetPaymentSetting()
	previousUnit := common.QuotaPerUnit
	previousStripeAPI, previousStripeSecret, previousStripePrice := setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId
	t.Cleanup(func() {
		*operation_setting.GetPaymentSetting() = previousPayment
		common.QuotaPerUnit = previousUnit
		setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = previousStripeAPI, previousStripeSecret, previousStripePrice
	})
	common.QuotaPerUnit = 500000
	setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = "fixture_api", "fixture_signature_secret", "fixture_price"
	for _, provider := range []string{model.PaymentProviderStripe, model.PaymentProviderCreem} {
		t.Run(provider, func(t *testing.T) {
			db := modelManagementDB(t, "sqlite", "")
			require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.SubscriptionOrder{}, &model.FirstTopUpReferralReward{}, &model.Log{}))
			payment := operation_setting.GetPaymentSetting()
			payment.ComplianceConfirmed = true
			payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
			payment.FirstTopUpReferralRewardUSD = 5
			inviter := model.User{Id: 1, Username: "referral_checkout_inviter", AffCode: "checkout-inviter", Status: common.UserStatusEnabled}
			invitee := model.User{Id: 2, Username: "referral_checkout_friend", AffCode: "checkout-friend", InviterId: 1, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&inviter).Error)
			require.NoError(t, db.Create(&invitee).Error)
			for _, paidCents := range []int{0, 1000000} {
				order := model.TopUp{UserId: invitee.Id, Amount: 10000, Money: 10000, TradeNo: fmt.Sprintf("referral-checkout-%s-%d", provider, paidCents), PaymentMethod: provider, PaymentProvider: provider, Status: common.TopUpStatusPending}
				if provider == model.PaymentProviderCreem {
					order.Amount = 5000000000
				}
				require.NoError(t, order.Insert())
				for attempt := range 3 {
					if provider == model.PaymentProviderStripe {
						expectFailure := paidCents > 0 && attempt == 0
						if expectFailure {
							require.NoError(t, db.Model(&model.User{}).Where("id = ?", inviter.Id).Update("aff_quota", common.MaxWalletQuota).Error)
						}
						event := stripe.Event{ID: "evt_referral_fixture", APIVersion: stripe.APIVersion, Type: stripe.EventTypeCheckoutSessionCompleted, Data: &stripe.EventData{Object: map[string]any{
							"client_reference_id": order.TradeNo, "customer": "fixture_customer", "status": "complete", "payment_status": "paid", "amount_total": float64(paidCents), "currency": "usd",
						}}}
						body, err := common.Marshal(map[string]any{"id": event.ID, "api_version": event.APIVersion, "type": event.Type, "data": map[string]any{"object": event.Data.Object}})
						require.NoError(t, err)
						signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: body, Secret: setting.StripeWebhookSecret})
						recorder := httptest.NewRecorder()
						ctx, _ := gin.CreateTestContext(recorder)
						ctx.Request = httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(body)))
						ctx.Request.Header.Set("Stripe-Signature", signed.Header)
						StripeWebhook(ctx)
						if expectFailure {
							assert.Equal(t, http.StatusInternalServerError, recorder.Code, "an uncommitted payment must request a webhook retry")
							pending := model.GetTopUpByTradeNo(order.TradeNo)
							require.NotNil(t, pending)
							assert.Equal(t, common.TopUpStatusPending, pending.Status)
							require.NoError(t, db.Model(&model.User{}).Where("id = ?", inviter.Id).Update("aff_quota", 0).Error)
						} else {
							assert.Equal(t, http.StatusOK, recorder.Code)
						}
					} else {
						event := &CreemWebhookEvent{}
						event.Object.RequestId = order.TradeNo
						event.Object.Order.Status = "paid"
						event.Object.Order.Type = "onetime"
						event.Object.Order.AmountPaid = paidCents
						event.Object.Order.Currency = "USD"
						recorder := httptest.NewRecorder()
						ctx, _ := gin.CreateTestContext(recorder)
						ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/creem/webhook", nil)
						handleCheckoutCompleted(ctx, event)
						assert.Equal(t, http.StatusOK, recorder.Code)
					}
				}
				saved := model.GetTopUpByTradeNo(order.TradeNo)
				require.NotNil(t, saved)
				assert.Equal(t, common.TopUpStatusSuccess, saved.Status)
				require.NotNil(t, saved.PaymentReceived)
				assert.Equal(t, paidCents > 0, *saved.PaymentReceived)
				require.NoError(t, db.First(&inviter, inviter.Id).Error)
				var rewards int64
				require.NoError(t, db.Model(&model.FirstTopUpReferralReward{}).Count(&rewards).Error)
				if paidCents == 0 {
					assert.Zero(t, inviter.AffQuota)
					assert.Zero(t, rewards)
				} else {
					assert.Equal(t, 2500000, inviter.AffQuota)
					assert.EqualValues(t, 1, rewards)
				}
			}
			require.NoError(t, db.First(&invitee, invitee.Id).Error)
			assert.Equal(t, 10000000000, invitee.Quota, "existing checkout credit is preserved, without duplicate credit or an invitee bonus")
		})
	}
}

func confirmPaymentComplianceForTest(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
}

func TestStripeWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
	})

	setting.StripeWebhookSecret = ""
	setting.StripeApiSecret = "sk_test_123"
	setting.StripePriceId = "price_123"
	require.False(t, isStripeWebhookEnabled())

	setting.StripeWebhookSecret = "whsec_test"
	require.True(t, isStripeWebhookEnabled())

	setting.StripePriceId = ""
	require.False(t, isStripeWebhookEnabled())
}

func TestCreemWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPIKey := setting.CreemApiKey
	originalProducts := setting.CreemProducts
	originalWebhookSecret := setting.CreemWebhookSecret
	t.Cleanup(func() {
		setting.CreemApiKey = originalAPIKey
		setting.CreemProducts = originalProducts
		setting.CreemWebhookSecret = originalWebhookSecret
	})

	setting.CreemWebhookSecret = ""
	setting.CreemApiKey = "creem_api_key"
	setting.CreemProducts = `[{"productId":"prod_123"}]`
	require.False(t, isCreemWebhookEnabled())

	setting.CreemWebhookSecret = "creem_secret"
	require.True(t, isCreemWebhookEnabled())

	setting.CreemProducts = "[]"
	require.False(t, isCreemWebhookEnabled())
}

func TestWaffoWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.WaffoEnabled
	originalSandbox := setting.WaffoSandbox
	originalAPIKey := setting.WaffoApiKey
	originalPrivateKey := setting.WaffoPrivateKey
	originalPublicCert := setting.WaffoPublicCert
	originalSandboxAPIKey := setting.WaffoSandboxApiKey
	originalSandboxPrivateKey := setting.WaffoSandboxPrivateKey
	originalSandboxPublicCert := setting.WaffoSandboxPublicCert
	t.Cleanup(func() {
		setting.WaffoEnabled = originalEnabled
		setting.WaffoSandbox = originalSandbox
		setting.WaffoApiKey = originalAPIKey
		setting.WaffoPrivateKey = originalPrivateKey
		setting.WaffoPublicCert = originalPublicCert
		setting.WaffoSandboxApiKey = originalSandboxAPIKey
		setting.WaffoSandboxPrivateKey = originalSandboxPrivateKey
		setting.WaffoSandboxPublicCert = originalSandboxPublicCert
	})

	setting.WaffoEnabled = true
	setting.WaffoSandbox = false
	setting.WaffoApiKey = ""
	setting.WaffoPrivateKey = "private"
	setting.WaffoPublicCert = "public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoApiKey = "api"
	require.True(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = false
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoSandboxApiKey = ""
	setting.WaffoSandboxPrivateKey = "sandbox_private"
	setting.WaffoSandboxPublicCert = "sandbox_public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoSandboxApiKey = "sandbox_api"
	require.True(t, isWaffoWebhookEnabled())
}

func TestWaffoPancakeWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalProductID := setting.WaffoPancakeProductID
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeProductID = originalProductID
	})

	// Presence of all three credentials enables the gateway. Webhook public
	// keys are bundled in the SDK and there is no separate Enabled toggle —
	// clear any of the three fields to disable.
	setting.WaffoPancakeMerchantID = ""
	setting.WaffoPancakePrivateKey = "private"
	setting.WaffoPancakeProductID = "product"
	require.False(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeMerchantID = "merchant"
	require.True(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeProductID = ""
	require.False(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeProductID = "product"
	setting.WaffoPancakePrivateKey = ""
	require.False(t, isWaffoPancakeWebhookEnabled())
}

func TestEpayWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})

	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	require.False(t, isEpayWebhookEnabled())

	operation_setting.EpayKey = "epay_key"
	require.True(t, isEpayWebhookEnabled())

	operation_setting.PayMethods = nil
	require.False(t, isEpayWebhookEnabled())
}

func TestEpayCallbacksVerifyBeforeSettlement(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	originalServerAddress := system_setting.ServerAddress
	originalDB := model.DB
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
		system_setting.ServerAddress = originalServerAddress
		model.DB = originalDB
	})
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "test-key"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	system_setting.ServerAddress = "https://dashboard.example.com"
	// Rejected callbacks and valid non-success events must never reach settlement.
	// A nil database makes an accidental attempt to settle fail the test.
	model.DB = nil
	require.True(t, isEpayWebhookEnabled())

	// Both ambiguous maps below had this same valid signing string in v0.0.4.
	// This must fail because of parameter ambiguity, not a mismatched signature.
	ambiguousSign := epay.MD5String("money=1.00&name=item&out_trade_no=other&out_trade_no=order&pid=epay_id&trade_status=TRADE_SUCCESS&type=alipay", "test-key")
	pendingSign := epay.MD5String("money=1.00&name=item%26other&out_trade_no=order&pid=epay_id&trade_status=WAIT_BUYER_PAY&type=alipay", "test-key")
	cases := []struct {
		name         string
		productName  string
		tradeNo      string
		status       string
		sign         string
		validPending bool
	}{
		{"separator in product name", "item&out_trade_no=other", "order", epay.StatusTradeSuccess, ambiguousSign, false},
		{"separator in order number", "item", "other&out_trade_no=order", epay.StatusTradeSuccess, ambiguousSign, false},
		{"invalid signature", "item", "order", epay.StatusTradeSuccess, "invalid", false},
		{"missing signature", "item", "order", epay.StatusTradeSuccess, "", false},
		{"valid pending with literal percent escape", "item%26other", "order", "WAIT_BUYER_PAY", pendingSign, true},
	}
	endpoints := []struct {
		name             string
		path             string
		handler          gin.HandlerFunc
		status           int
		rejectedBody     string
		pendingBody      string
		rejectedLocation string
		pendingLocation  string
	}{
		{
			name: "topup notify", path: "/api/user/epay/notify", handler: EpayNotify,
			status: http.StatusOK, rejectedBody: "fail", pendingBody: "success",
		},
		{
			name: "subscription notify", path: "/api/subscription/epay/notify", handler: SubscriptionEpayNotify,
			status: http.StatusOK, rejectedBody: "fail", pendingBody: "fail",
		},
		{
			name: "subscription return", path: "/api/subscription/epay/return", handler: SubscriptionEpayReturn,
			status:           http.StatusFound,
			rejectedLocation: "https://dashboard.example.com/wallet?pay=fail",
			pendingLocation:  "https://dashboard.example.com/wallet?pay=pending",
		},
	}
	router := gin.New()
	for _, endpoint := range endpoints {
		router.Any(endpoint.path, endpoint.handler)
	}
	for _, endpoint := range endpoints {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			for _, tc := range cases {
				t.Run(endpoint.name+"/"+method+"/"+tc.name, func(t *testing.T) {
					params := url.Values{
						"pid": {"epay_id"}, "type": {"alipay"}, "money": {"1.00"},
						"name": {tc.productName}, "out_trade_no": {tc.tradeNo},
						"trade_status": {tc.status}, "sign_type": {"MD5"},
					}
					if tc.sign != "" {
						params.Set("sign", tc.sign)
					}
					request := httptest.NewRequest(method, endpoint.path+"?"+params.Encode(), nil)
					if method == http.MethodPost {
						request = httptest.NewRequest(method, endpoint.path, strings.NewReader(params.Encode()))
						request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					}
					recorder := httptest.NewRecorder()
					require.NotPanics(t, func() { router.ServeHTTP(recorder, request) })
					assert.Equal(t, endpoint.status, recorder.Code)
					wantBody, wantLocation := endpoint.rejectedBody, endpoint.rejectedLocation
					if tc.validPending {
						wantBody, wantLocation = endpoint.pendingBody, endpoint.pendingLocation
					}
					if wantBody != "" {
						assert.Equal(t, wantBody, recorder.Body.String())
					}
					assert.Equal(t, wantLocation, recorder.Header().Get("Location"))
				})
			}
		}
	}
}
