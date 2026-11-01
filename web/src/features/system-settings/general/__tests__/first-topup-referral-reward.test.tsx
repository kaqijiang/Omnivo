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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { QuotaSettingsSection } from '../quota-settings-section'

function Fixture() {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <QuotaSettingsSection
          defaultValues={{
            QuotaForNewUser: 0,
            QuotaForInviter: 500000,
            QuotaForInvitee: 0,
            TopUpLink: '',
            payment_setting: { first_topup_referral_reward_usd: 5 },
            quota_setting: {
              enable_free_model_pre_consume: true,
              trust_quota_usd: 10,
              pre_consume_multiplier: 1,
            },
          }}
        />
      </SettingsPageProvider>
    </>
  )
}

async function renderSettings() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: Fixture }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return screen.findByRole('spinbutton', {
    name: 'First top-up referral reward (USD)',
  })
}

beforeEach(() => {
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

test.each(['0', '2.5'])(
  'saves reward %s in USD without changing existing registration rewards',
  async (value) => {
    const input = await renderSettings()
    expect(input).toHaveValue(5)
    expect(
      screen.getByText(
        'Reward credited to the inviter when an invited friend completes their first paid top-up. Set to 0 to disable. Existing payment minimums apply.'
      )
    ).toBeVisible()
    fireEvent.change(input, { target: { value } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(api.put).toHaveBeenCalledWith('/api/option/', {
        key: 'payment_setting.first_topup_referral_reward_usd',
        value: Number(value),
      })
    )
    expect(api.put).toHaveBeenCalledOnce()
    expect(
      screen.getByRole('spinbutton', { name: 'Inviter Reward' })
    ).toHaveValue(500000)
  }
)

test.each(['', '-5'])(
  'invalid reward "%s" marks the field invalid and prevents saving',
  async (value) => {
    const input = await renderSettings()
    fireEvent.change(input, { target: { value } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
    expect(api.put).not.toHaveBeenCalled()
  }
)
