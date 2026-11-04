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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { RequestContentSection } from '../request-content-section'

function Fixture() {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <RequestContentSection />
      </SettingsPageProvider>
    </>
  )
}

function status(ready = true, enabled = false) {
  return {
    data: {
      success: true,
      data: {
        configured: ready,
        ready,
        settings: { enabled, retention_days: 7, max_body_bytes: 1048576 },
        capture: { active: 3, skipped: 1 },
        publisher: { failed: 2 },
      },
    },
  }
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <Fixture />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore.getState().auth.setBundle({
    access_token: 'test-token',
    token_type: 'Bearer',
    access_expires_at: Date.now() / 1000 + 300,
    user: { id: 1, username: 'root', role: 100 },
    session: {
      sid: 'session',
      current: true,
      login_method: 'password',
      ip: '',
      user_agent: '',
      created_at: 1,
      last_active_at: 1,
      expires_at: Date.now() / 1000 + 3600,
    },
  })
  vi.spyOn(api, 'get').mockResolvedValue(status())
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})
afterEach(() => useAuthStore.getState().auth.reset())

test('an unconfigured service cannot be enabled', async () => {
  vi.spyOn(api, 'get').mockResolvedValue(status(false))
  mount()
  expect(
    await screen.findByRole('switch', {
      name: 'Enable request content retention',
    })
  ).toHaveAttribute('aria-disabled', 'true')
  expect(
    screen.getByText(
      'Configure the content retention service before enabling new recordings.'
    )
  ).toBeVisible()
})

test('saving limits writes them before enabling retention', async () => {
  const user = userEvent.setup()
  mount()
  fireEvent.change(
    await screen.findByRole('spinbutton', { name: 'Content retention (days)' }),
    { target: { value: '14' } }
  )
  fireEvent.change(
    screen.getByRole('spinbutton', { name: 'Maximum retained bytes per body' }),
    { target: { value: '2097152' } }
  )
  await user.click(
    screen.getByRole('switch', { name: 'Enable request content retention' })
  )
  await user.click(
    screen.getByRole('button', { name: 'Save retention settings' })
  )
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(3))
  expect(vi.mocked(api.put).mock.calls.map((call) => call[1])).toEqual([
    { key: 'request_content.retention_days', value: 14 },
    { key: 'request_content.max_body_bytes', value: 2097152 },
    { key: 'request_content.enabled', value: true },
  ])
})

test('disabling remains available when the configured service becomes unavailable', async () => {
  vi.spyOn(api, 'get').mockResolvedValue(status(false, true))
  const user = userEvent.setup()
  mount()
  const toggle = await screen.findByRole('switch', {
    name: 'Enable request content retention',
  })
  expect(toggle).toBeEnabled()
  await user.click(toggle)
  await user.click(
    screen.getByRole('button', { name: 'Save retention settings' })
  )
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith('/api/option/', {
      key: 'request_content.enabled',
      value: false,
    })
  )
})

test('invalid body limits produce a field error and do not save', async () => {
  const user = userEvent.setup()
  mount()
  const bytes = await screen.findByRole('spinbutton', {
    name: 'Maximum retained bytes per body',
  })
  fireEvent.change(bytes, { target: { value: '1023' } })
  await user.click(
    screen.getByRole('button', { name: 'Save retention settings' })
  )
  expect(
    await screen.findByRole('spinbutton', {
      name: 'Maximum retained bytes per body',
    })
  ).toHaveAttribute('aria-invalid', 'true')
  expect(api.put).not.toHaveBeenCalled()
})

test('a non-root identity cannot fetch or render the settings section', () => {
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  )
  mount()
  expect(
    screen.queryByRole('switch', { name: 'Enable request content retention' })
  ).toBeNull()
  expect(api.get).not.toHaveBeenCalled()
})
