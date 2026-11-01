/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import type { UserWalletData } from '../../types'
import { AffiliateRewardsCard } from '../affiliate-rewards-card'

const inviter: UserWalletData = {
  id: 1,
  username: 'inviter',
  quota: 0,
  used_quota: 0,
  request_count: 0,
  aff_quota: 2500000,
  aff_history_quota: 2500000,
  aff_count: 1,
  group: 'default',
}

const initialConfig = useSystemConfigStore.getState().config

beforeEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG },
  })
})

afterEach(() => {
  useSystemConfigStore.getState().setConfig(initialConfig)
})

test('shows the configured reward to the inviter and preserves the existing transfer action', async () => {
  const onTransfer = vi.fn()
  render(
    <AffiliateRewardsCard
      user={inviter}
      affiliateLink='https://omnivo.example/sign-up?aff=friend'
      firstTopupRewardUSD={5}
      onTransfer={onTransfer}
    />
  )

  expect(
    screen.getByText(
      "Earn $5 in referral credit when an invited friend makes their first successful paid top-up. Awarded once per friend; the payment method's minimum top-up applies."
    )
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Copy referral link' })
  ).toBeEnabled()
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Transfer to Balance' }))
  expect(onTransfer).toHaveBeenCalledOnce()
})

test.each([undefined, 0, -5, Number.NaN, Number.POSITIVE_INFINITY])(
  'hides the first top-up offer when the configured reward is %s',
  (reward) => {
    render(
      <AffiliateRewardsCard
        user={inviter}
        affiliateLink='https://omnivo.example/sign-up?aff=friend'
        firstTopupRewardUSD={reward}
        onTransfer={vi.fn()}
      />
    )
    expect(screen.queryByText(/first successful paid top-up/)).toBeNull()
    expect(screen.getByText('Referral Program')).toBeVisible()
  }
)

test('hides the offer and disables reward transfer when compliance is not confirmed', () => {
  render(
    <AffiliateRewardsCard
      user={inviter}
      affiliateLink='https://omnivo.example/sign-up?aff=friend'
      firstTopupRewardUSD={5}
      complianceConfirmed={false}
      onTransfer={vi.fn()}
    />
  )
  expect(screen.queryByText(/first successful paid top-up/)).toBeNull()
  expect(
    screen.getByRole('button', { name: 'Transfer to Balance' })
  ).toBeDisabled()
})

test('formats referral credit in the display currency and responds to a language change', async () => {
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CNY',
      usdExchangeRate: 7,
    },
  })
  const i18n = createInstance()
  await i18n.use(initReactI18next).init({
    lng: 'zhCN',
    fallbackLng: false,
    resources: {
      zhCN: { translation: { 'Referral Program': '推荐计划' } },
      fr: { translation: { 'Referral Program': 'Parrainage' } },
    },
    interpolation: { escapeValue: false },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <AffiliateRewardsCard
        user={inviter}
        affiliateLink='https://omnivo.example/sign-up?aff=friend'
        firstTopupRewardUSD={5}
        onTransfer={vi.fn()}
      />
    </I18nextProvider>
  )

  expect(screen.getByText(/first successful paid top-up/)).toHaveTextContent(
    'Earn ¥35 in referral credit'
  )
  await act(() => i18n.changeLanguage('fr'))
  expect(screen.getByText('Parrainage')).toBeVisible()
  expect(screen.getByText(/first successful paid top-up/)).toHaveTextContent(
    'Earn 35 ¥ in referral credit'
  )
})
