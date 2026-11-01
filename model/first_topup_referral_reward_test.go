package model

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

const firstTopUpTestUnit = 500000

func TestFirstTopUpReferralRewardDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			setupFirstTopUpRewardDatabase(t, dialect)

			for _, provider := range []string{PaymentProviderEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake, "manual"} {
				t.Run(provider+" awards inviter once", func(t *testing.T) {
					resetFirstTopUpRewardFixture(t)
					order := createFirstTopUpRewardOrder(t, "first", provider)
					require.NoError(t, completeFirstTopUpRewardOrder(order, provider, true))
					assertFirstTopUpRewardBalances(t, firstTopUpTestUnit, 5*firstTopUpTestUnit)
					// Existing gateways differ on whether a duplicate returns nil or
					// an invalid-status error; neither may credit either party twice.
					_ = completeFirstTopUpRewardOrder(order, provider, true)
					assertFirstTopUpRewardBalances(t, firstTopUpTestUnit, 5*firstTopUpTestUnit)
					second := createFirstTopUpRewardOrder(t, "second", provider)
					require.NoError(t, completeFirstTopUpRewardOrder(second, provider, true))
					assertFirstTopUpRewardBalances(t, 2*firstTopUpTestUnit, 5*firstTopUpTestUnit)
					var rewards []FirstTopUpReferralReward
					require.NoError(t, DB.Find(&rewards).Error)
					require.Len(t, rewards, 1)
					assert.Equal(t, order.Id, rewards[0].TopUpID)
					assert.Equal(t, order.TradeNo, rewards[0].TradeNo)
					assert.Equal(t, 1, rewards[0].InviterID)
					assert.Equal(t, 2, rewards[0].InviteeID)
					assert.Equal(t, 5*firstTopUpTestUnit, rewards[0].Quota)
					var rewardLogs []Log
					require.NoError(t, DB.Where("user_id = ? AND type = ?", 1, LogTypeSystem).Find(&rewardLogs).Error)
					require.Len(t, rewardLogs, 1)
					assert.Contains(t, rewardLogs[0].Other, "First top-up referral reward")
				})
			}

			for _, tc := range []struct {
				name   string
				change func(t *testing.T, order *TopUp)
			}{
				{"disabled setting", func(t *testing.T, order *TopUp) {
					operation_setting.GetPaymentSetting().FirstTopUpReferralRewardUSD = 0
				}},
				{"compliance unconfirmed", func(t *testing.T, order *TopUp) { operation_setting.GetPaymentSetting().ComplianceConfirmed = false }},
				{"no inviter", func(t *testing.T, order *TopUp) {
					require.NoError(t, DB.Model(&User{}).Where("id = 2").Update("inviter_id", 0).Error)
				}},
				{"missing inviter", func(t *testing.T, order *TopUp) {
					require.NoError(t, DB.Model(&User{}).Where("id = 2").Update("inviter_id", 99).Error)
				}},
				{"self referral", func(t *testing.T, order *TopUp) {
					require.NoError(t, DB.Model(&User{}).Where("id = 2").Update("inviter_id", 2).Error)
				}},
				{"disabled inviter", func(t *testing.T, order *TopUp) {
					require.NoError(t, DB.Model(&User{}).Where("id = 1").Update("status", common.UserStatusDisabled).Error)
				}},
				{"zero money", func(t *testing.T, order *TopUp) { require.NoError(t, DB.Model(order).Update("money", 0).Error) }},
				{"unknown provider", func(t *testing.T, order *TopUp) {
					require.NoError(t, DB.Model(order).Update("payment_provider", "unverified").Error)
				}},
				{"balance payment", func(t *testing.T, order *TopUp) {
					require.NoError(t, DB.Model(order).Update("payment_provider", PaymentProviderBalance).Error)
				}},
			} {
				t.Run(tc.name+" earns no reward", func(t *testing.T) {
					resetFirstTopUpRewardFixture(t)
					order := createFirstTopUpRewardOrder(t, "ineligible", PaymentProviderEpay)
					tc.change(t, order)
					require.NoError(t, ManualCompleteTopUp(order.TradeNo, ""))
					assertFirstTopUpRewardBalances(t, firstTopUpTestUnit, 0)
					var count int64
					require.NoError(t, DB.Model(&FirstTopUpReferralReward{}).Count(&count).Error)
					assert.Zero(t, count)
				})
			}

			for _, status := range []string{common.TopUpStatusPending, common.TopUpStatusFailed, common.TopUpStatusSuccess} {
				t.Run("history "+status, func(t *testing.T) {
					resetFirstTopUpRewardFixture(t)
					prior := createFirstTopUpRewardOrder(t, "legacy", PaymentProviderEpay)
					require.NoError(t, DB.Model(prior).Updates(map[string]any{"status": status, "create_time": 1}).Error)
					order := createFirstTopUpRewardOrder(t, "current", PaymentProviderEpay)
					require.NoError(t, completeFirstTopUpRewardOrder(order, PaymentProviderEpay, true))
					wantBonus := 5 * firstTopUpTestUnit
					if status == common.TopUpStatusSuccess {
						wantBonus = 0
					}
					assertFirstTopUpRewardBalances(t, firstTopUpTestUnit, wantBonus)
				})
			}

			for _, provider := range []string{PaymentProviderStripe, PaymentProviderCreem} {
				t.Run(provider+" free checkout then first paid order", func(t *testing.T) {
					resetFirstTopUpRewardFixture(t)
					freeOrder := createFirstTopUpRewardOrder(t, "coupon-free", provider)
					require.NoError(t, completeFirstTopUpRewardOrder(freeOrder, provider, false))
					assertFirstTopUpRewardBalances(t, firstTopUpTestUnit, 0)
					var reloaded TopUp
					require.NoError(t, DB.First(&reloaded, freeOrder.Id).Error)
					require.NotNil(t, reloaded.PaymentReceived)
					assert.False(t, *reloaded.PaymentReceived)
					paidOrder := createFirstTopUpRewardOrder(t, "paid", provider)
					require.NoError(t, completeFirstTopUpRewardOrder(paidOrder, provider, true))
					assertFirstTopUpRewardBalances(t, 2*firstTopUpTestUnit, 5*firstTopUpTestUnit)
				})
			}

			t.Run("legacy paid order without provider prevents a new reward", func(t *testing.T) {
				resetFirstTopUpRewardFixture(t)
				prior := createFirstTopUpRewardOrder(t, "legacy-null-provider", PaymentProviderEpay)
				require.NoError(t, DB.Model(prior).Updates(map[string]any{"status": common.TopUpStatusSuccess, "payment_provider": nil}).Error)
				order := createFirstTopUpRewardOrder(t, "current-after-legacy", PaymentProviderEpay)
				require.NoError(t, completeFirstTopUpRewardOrder(order, PaymentProviderEpay, true))
				assertFirstTopUpRewardBalances(t, firstTopUpTestUnit, 0)
			})

			t.Run("redemption and subscription do not use first paid eligibility", func(t *testing.T) {
				resetFirstTopUpRewardFixture(t)
				require.NoError(t, DB.Create(&Redemption{Id: 1, Key: "referral-test-redemption", Name: "reward test", Status: common.RedemptionCodeStatusEnabled, Quota: 100}).Error)
				quota, err := Redeem("referral-test-redemption", 2)
				require.NoError(t, err)
				assert.Equal(t, 100, quota)
				plan := &SubscriptionPlan{Id: 1, Title: "Referral test plan", PriceAmount: 10, Currency: "USD", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 1000}
				require.NoError(t, DB.Create(plan).Error)
				subscription := &SubscriptionOrder{UserId: 2, PlanId: 1, Money: 10, TradeNo: "subscription", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending}
				require.NoError(t, DB.Create(subscription).Error)
				require.NoError(t, CompleteSubscriptionOrder(subscription.TradeNo, "", PaymentProviderEpay, ""))
				assertFirstTopUpRewardBalances(t, 100, 0)
				order := createFirstTopUpRewardOrder(t, "paid-after-credits", PaymentProviderEpay)
				require.NoError(t, completeFirstTopUpRewardOrder(order, PaymentProviderEpay, true))
				assertFirstTopUpRewardBalances(t, firstTopUpTestUnit+100, 5*firstTopUpTestUnit)
			})

			t.Run("retained ledger prevents reaward after history deletion", func(t *testing.T) {
				resetFirstTopUpRewardFixture(t)
				order := createFirstTopUpRewardOrder(t, "original", PaymentProviderEpay)
				require.NoError(t, completeFirstTopUpRewardOrder(order, PaymentProviderEpay, true))
				require.NoError(t, DB.Delete(order).Error)
				second := createFirstTopUpRewardOrder(t, "after-cleanup", PaymentProviderEpay)
				require.NoError(t, completeFirstTopUpRewardOrder(second, PaymentProviderEpay, true))
				assertFirstTopUpRewardBalances(t, 2*firstTopUpTestUnit, 5*firstTopUpTestUnit)
				require.Error(t, DB.Create(&FirstTopUpReferralReward{InviteeID: 2, InviterID: 1, TopUpID: second.Id, TradeNo: second.TradeNo, Quota: 1}).Error)
				require.Error(t, DB.Create(&FirstTopUpReferralReward{InviteeID: 3, InviterID: 1, TopUpID: order.Id, TradeNo: order.TradeNo, Quota: 1}).Error)
			})

			t.Run("reused local order ID does not block a different invitee", func(t *testing.T) {
				resetFirstTopUpRewardFixture(t)
				order := createFirstTopUpRewardOrder(t, "original-before-cleanup", PaymentProviderEpay)
				require.NoError(t, completeFirstTopUpRewardOrder(order, PaymentProviderEpay, true))
				require.NoError(t, DB.Delete(order).Error)
				require.NoError(t, DB.Create(&User{Id: 3, Username: "second-referral", AffCode: "second", Status: common.UserStatusEnabled, InviterId: 1}).Error)
				next := &TopUp{Id: order.Id, UserId: 3, Amount: 1, Money: 1, TradeNo: "new-distinct-order", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
				require.NoError(t, DB.Create(next).Error)
				require.NoError(t, completeFirstTopUpRewardOrder(next, PaymentProviderEpay, true))
				var inviter User
				require.NoError(t, DB.First(&inviter, 1).Error)
				assert.Equal(t, 31+10*firstTopUpTestUnit, inviter.AffQuota)
				var rewardCount int64
				require.NoError(t, DB.Model(&FirstTopUpReferralReward{}).Count(&rewardCount).Error)
				assert.EqualValues(t, 2, rewardCount)
			})

			for _, sameOrder := range []bool{false, true} {
				t.Run(fmt.Sprintf("concurrent callbacks same_order=%v award once", sameOrder), func(t *testing.T) {
					resetFirstTopUpRewardFixture(t)
					first := createFirstTopUpRewardOrder(t, "concurrent-a", PaymentProviderEpay)
					second := first
					principal := firstTopUpTestUnit
					if !sameOrder {
						second = createFirstTopUpRewardOrder(t, "concurrent-b", PaymentProviderEpay)
						principal *= 2
					}
					orders := []*TopUp{first, second}
					start := make(chan struct{})
					type completion struct {
						order *TopUp
						err   error
					}
					results := make(chan completion, len(orders))
					for _, order := range orders {
						go func() {
							<-start
							results <- completion{order, completeFirstTopUpRewardOrder(order, PaymentProviderEpay, true)}
						}()
					}
					close(start)
					completed := make([]completion, 0, len(orders))
					for range orders {
						completed = append(completed, <-results)
					}
					for _, result := range completed {
						if result.err != nil {
							// SQLite has no row locks and can reject a competing writer;
							// retry after both callbacks (including post-commit log writes)
							// finish, preserving the same once-only result.
							require.Equal(t, "sqlite", dialect, result.err)
							require.True(t, strings.Contains(result.err.Error(), "locked") || strings.Contains(result.err.Error(), "busy"), result.err)
							if !sameOrder {
								var failedOrder TopUp
								require.NoError(t, DB.First(&failedOrder, result.order.Id).Error)
								assert.Equal(t, common.TopUpStatusPending, failedOrder.Status)
								assert.Nil(t, failedOrder.PaymentReceived)
								assertFirstTopUpRewardBalances(t, firstTopUpTestUnit, 5*firstTopUpTestUnit)
							}
							require.NoError(t, completeFirstTopUpRewardOrder(result.order, PaymentProviderEpay, true))
						}
					}
					assertFirstTopUpRewardBalances(t, principal, 5*firstTopUpTestUnit)
				})
			}

			for _, tc := range []struct {
				name, column string
				initial      int
				wantError    bool
			}{
				{"payer limit", "quota", common.MaxWalletQuota, true},
				{"pending reward limit", "aff_quota", common.MaxWalletQuota - 5*firstTopUpTestUnit + 1, true},
				{"history reward limit", "aff_history", common.MaxWalletQuota - 5*firstTopUpTestUnit + 1, true},
				{"exact reward limit", "aff_quota", common.MaxWalletQuota - 5*firstTopUpTestUnit, false},
			} {
				t.Run(tc.name+" is atomic", func(t *testing.T) {
					resetFirstTopUpRewardFixture(t)
					userID := 1
					if tc.column == "quota" {
						userID = 2
					}
					require.NoError(t, DB.Model(&User{}).Where("id = ?", userID).Update(tc.column, tc.initial).Error)
					order := createFirstTopUpRewardOrder(t, "limit", PaymentProviderEpay)
					err := completeFirstTopUpRewardOrder(order, PaymentProviderEpay, true)
					var updated TopUp
					require.NoError(t, DB.First(&updated, order.Id).Error)
					var ledgerCount int64
					require.NoError(t, DB.Model(&FirstTopUpReferralReward{}).Count(&ledgerCount).Error)
					if tc.wantError {
						require.Error(t, err)
						assert.Equal(t, common.TopUpStatusPending, updated.Status)
						assert.Nil(t, updated.PaymentReceived)
						assert.Zero(t, ledgerCount)
						var inviter, payer User
						require.NoError(t, DB.First(&inviter, 1).Error)
						require.NoError(t, DB.First(&payer, 2).Error)
						if userID == 2 {
							assert.Equal(t, tc.initial, payer.Quota)
							assert.Equal(t, 31, inviter.AffQuota)
							assert.Equal(t, 47, inviter.AffHistoryQuota)
						} else {
							assert.Equal(t, 11, payer.Quota)
							if tc.column == "aff_quota" {
								assert.Equal(t, tc.initial, inviter.AffQuota)
								assert.Equal(t, 47, inviter.AffHistoryQuota)
							} else {
								assert.Equal(t, 31, inviter.AffQuota)
								assert.Equal(t, tc.initial, inviter.AffHistoryQuota)
							}
						}
					} else {
						require.NoError(t, err)
						assert.Equal(t, common.TopUpStatusSuccess, updated.Status)
						assert.EqualValues(t, 1, ledgerCount)
						var inviter User
						require.NoError(t, DB.First(&inviter, 1).Error)
						assert.Equal(t, common.MaxWalletQuota, inviter.AffQuota)
					}
				})
			}

			for _, invalid := range []float64{-1, math.NaN(), math.Inf(1), float64(common.MaxWalletQuota)} {
				t.Run(fmt.Sprintf("invalid bonus %v rolls back", invalid), func(t *testing.T) {
					resetFirstTopUpRewardFixture(t)
					operation_setting.GetPaymentSetting().FirstTopUpReferralRewardUSD = invalid
					order := createFirstTopUpRewardOrder(t, "invalid-config", PaymentProviderEpay)
					require.Error(t, completeFirstTopUpRewardOrder(order, PaymentProviderEpay, true))
					assertFirstTopUpRewardBalances(t, 0, 0)
					var reloaded TopUp
					require.NoError(t, DB.First(&reloaded, order.Id).Error)
					assert.Equal(t, common.TopUpStatusPending, reloaded.Status)
				})
			}

			t.Run("legacy schema upgrades twice without granting old customers", func(t *testing.T) {
				resetFirstTopUpRewardFixture(t)
				require.NoError(t, DB.Migrator().DropTable(&TopUp{}, &FirstTopUpReferralReward{}))
				table := DB.NamingStrategy.TableName("TopUp")
				require.NoError(t, DB.Table(table).AutoMigrate(&firstTopUpLegacyOrder{}))
				legacy := firstTopUpLegacyOrder{Id: 20, UserId: 2, Amount: 1, Money: 1, TradeNo: "pre-upgrade", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CreateTime: 1, CompleteTime: 2}
				require.NoError(t, DB.Table(table).Create(&legacy).Error)
				require.False(t, DB.Migrator().HasColumn(&TopUp{}, "payment_received"))
				for range 2 {
					require.NoError(t, DB.AutoMigrate(&TopUp{}, &FirstTopUpReferralReward{}))
				}
				var existing TopUp
				require.NoError(t, DB.First(&existing, legacy.Id).Error)
				assert.Equal(t, legacy.TradeNo, existing.TradeNo)
				assert.Equal(t, legacy.Status, existing.Status)
				assert.Nil(t, existing.PaymentReceived)
				current := createFirstTopUpRewardOrder(t, "post-upgrade", PaymentProviderEpay)
				require.NoError(t, completeFirstTopUpRewardOrder(current, PaymentProviderEpay, true))
				assertFirstTopUpRewardBalances(t, firstTopUpTestUnit, 0)
			})
		})
	}
}

func setupFirstTopUpRewardDatabase(t *testing.T, dialect string) {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "sqlite":
		driver = sqlite.Open(filepath.Join(t.TempDir(), "referral.db") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		driver = mysqlMigrationDialector{mysql.Dialector{Config: &mysql.Config{DSN: dsn}}}
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
		driver = postgresMigrationDialector{postgres.Dialector{Config: &postgres.Config{DSN: dsn, PreferSimpleProtocol: true}}}
	}
	db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("ref%d_", time.Now().UnixNano())}, Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	if dialect == "mysql" {
		// Production startup requires a Chinese-capable database charset.
		db = db.Set("gorm:table_options", "CHARSET=utf8mb4").Session(&gorm.Session{})
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousPayment := *operation_setting.GetPaymentSetting()
	previousUnit := common.QuotaPerUnit
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseType(dialect), common.DatabaseType(dialect))
	models := []any{&User{}, &TopUp{}, &FirstTopUpReferralReward{}, &Log{}, &Redemption{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}}
	t.Cleanup(func() {
		assert.NoError(t, db.Migrator().DropTable(models...))
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		*operation_setting.GetPaymentSetting() = previousPayment
		common.QuotaPerUnit = previousUnit
		assert.NoError(t, sqlDB.Close())
	})
	for range 2 {
		require.NoError(t, db.AutoMigrate(models...))
	}
	versionQuery := "SELECT version()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database version: %s", version)
}

func resetFirstTopUpRewardFixture(t *testing.T) {
	t.Helper()
	for _, model := range []any{&FirstTopUpReferralReward{}, &TopUp{}, &User{}, &Log{}, &Redemption{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}} {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(model).Error)
	}
	setting := operation_setting.GetPaymentSetting()
	setting.FirstTopUpReferralRewardUSD = 5
	setting.ComplianceConfirmed = true
	setting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	common.QuotaPerUnit = firstTopUpTestUnit
	require.NoError(t, DB.Create(&User{Id: 1, Username: "referral-inviter", AffCode: "inviter", Status: common.UserStatusEnabled, Quota: 23, AffQuota: 31, AffHistoryQuota: 47, AffCount: 7}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "referral-payer", AffCode: "payer", Status: common.UserStatusEnabled, Quota: 11, InviterId: 1}).Error)
}

func createFirstTopUpRewardOrder(t *testing.T, tradeNo, provider string) *TopUp {
	t.Helper()
	if provider == "manual" {
		provider = PaymentProviderEpay
	}
	order := &TopUp{UserId: 2, Amount: 1, Money: 1, TradeNo: tradeNo, PaymentMethod: provider, PaymentProvider: provider, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	if provider == PaymentProviderCreem {
		order.Amount = firstTopUpTestUnit
	}
	require.NoError(t, DB.Create(order).Error)
	return order
}

func completeFirstTopUpRewardOrder(order *TopUp, provider string, paymentReceived bool) error {
	switch provider {
	case PaymentProviderEpay:
		_, err := RechargeEpay(order.TradeNo, "alipay", "")
		return err
	case PaymentProviderStripe:
		return Recharge(order.TradeNo, "test-customer", "", paymentReceived)
	case PaymentProviderCreem:
		return RechargeCreem(order.TradeNo, "payer@example.test", "Payer", "", paymentReceived)
	case PaymentProviderWaffo:
		return RechargeWaffo(order.TradeNo, "")
	case PaymentProviderWaffoPancake:
		return RechargeWaffoPancake(order.TradeNo)
	case "manual":
		return ManualCompleteTopUp(order.TradeNo, "")
	default:
		return errors.New("unsupported test payment provider")
	}
}

func assertFirstTopUpRewardBalances(t *testing.T, principal, reward int) {
	t.Helper()
	var inviter, payer User
	require.NoError(t, DB.First(&inviter, 1).Error)
	require.NoError(t, DB.First(&payer, 2).Error)
	assert.Equal(t, 11+principal, payer.Quota, "the payer receives only the existing recharge principal")
	assert.Equal(t, 23, inviter.Quota, "the reward stays in referral balance until transferred")
	assert.Equal(t, 31+reward, inviter.AffQuota)
	assert.Equal(t, 47+reward, inviter.AffHistoryQuota)
	assert.Equal(t, 7, inviter.AffCount, "first recharge must not increment signup invitation count")
}

// This is the deployed TopUp schema before payment-received evidence was added.
type firstTopUpLegacyOrder struct {
	Id              int
	UserId          int `gorm:"index"`
	Amount          int64
	Money           float64
	TradeNo         string `gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string `gorm:"type:varchar(50)"`
	PaymentProvider string `gorm:"type:varchar(50);default:''"`
	CreateTime      int64
	CompleteTime    int64
	Status          string
}
