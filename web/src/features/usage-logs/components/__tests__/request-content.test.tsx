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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { decodeContent, type ContentEvent } from '../../lib/request-content'
import { RequestContentPanel } from '../dialogs/request-content-panel'

function login(id = 1, role: number = ROLE.SUPER_ADMIN, sid = 'session-one') {
  useAuthStore.getState().auth.setBundle({
    access_token: 'test-access',
    token_type: 'Bearer',
    access_expires_at: Date.now() / 1000 + 300,
    user: { id, username: 'test', role },
    session: {
      sid,
      current: true,
      login_method: 'password',
      ip: '',
      user_agent: '',
      created_at: 1,
      last_active_at: 1,
      expires_at: Date.now() / 1000 + 3600,
    },
  })
}

function event(
  sequence: number,
  text: string,
  direction: 'request' | 'response' = 'response'
): ContentEvent {
  return {
    request_id: 'req-one',
    kind: 'data',
    sequence,
    direction,
    data: btoa(text),
  }
}

function page(
  events = [event(1, 'private response')],
  next: number | null = null
) {
  return {
    data: {
      success: true,
      data: {
        request_id: 'req-one',
        state: 'complete',
        expires_at: Math.floor(Date.now() / 1000) + 3600,
        stream: true,
        truncated: false,
        incomplete: false,
        events,
        next_sequence: next,
      },
    },
  }
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const result = render(
    <QueryClientProvider client={client}>
      <RequestContentPanel requestId='req-one' isRoot open />
    </QueryClientProvider>
  )
  return { ...result, client }
}

beforeEach(() => login())
afterEach(() => useAuthStore.getState().auth.reset())

test('only a current root session can open retained content, even if the caller supplies isRoot', () => {
  login(2, ROLE.ADMIN)
  const get = vi.spyOn(api, 'get')
  mount()
  expect(
    screen.queryByRole('button', { name: 'View request and response' })
  ).toBeNull()
  expect(get).not.toHaveBeenCalled()
})

test('opening loads content on demand into a private cache and closing removes it', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue(page())
  const user = userEvent.setup()
  const { client } = mount()
  expect(get).not.toHaveBeenCalled()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  expect(await screen.findByText('private response')).toBeVisible()
  expect(get).toHaveBeenCalledWith(
    '/api/log/content/req-one',
    expect.objectContaining({
      params: { after: -1, limit: 8 },
      disableDuplicate: true,
      signal: expect.any(AbortSignal),
    })
  )
  expect(client.getQueryCache().getAll()).toHaveLength(0)
  await user.click(
    screen.getByRole('button', { name: 'Hide retained content' })
  )
  expect(screen.queryByText('private response')).toBeNull()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  await screen.findByText('private response')
  expect(get).toHaveBeenCalledTimes(2)
})

test('closing aborts the in-flight request and its late response cannot reappear', async () => {
  let finish!: (value: ReturnType<typeof page>) => void
  const pending = new Promise<ReturnType<typeof page>>((resolve) => {
    finish = resolve
  })
  const get = vi.spyOn(api, 'get').mockReturnValueOnce(pending)
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  const signal = get.mock.calls[0][1]?.signal
  await user.click(
    screen.getByRole('button', { name: 'Hide retained content' })
  )
  expect(signal?.aborted).toBe(true)
  await act(async () => finish(page()))
  expect(screen.queryByText('private response')).toBeNull()
})

test('switching root sessions immediately clears plaintext and requires another explicit open', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue(page())
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  await screen.findByText('private response')
  act(() => login(1, ROLE.SUPER_ADMIN, 'session-two'))
  expect(screen.queryByText('private response')).toBeNull()
  expect(
    screen.getByRole('button', { name: 'View request and response' })
  ).toHaveAttribute('aria-expanded', 'false')
  expect(get).toHaveBeenCalledTimes(1)
})

test('downgrading the user or logging out removes the content entry and plaintext', async () => {
  vi.spyOn(api, 'get').mockResolvedValue(page())
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  await screen.findByText('private response')
  act(() => login(1, ROLE.ADMIN))
  expect(screen.queryByText('private response')).toBeNull()
  expect(
    screen.queryByRole('button', { name: 'View request and response' })
  ).toBeNull()
})

test('a failed next page clears all previously displayed plaintext', async () => {
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce(page([event(1, 'private response')], 1))
    .mockRejectedValueOnce({ response: { status: 403 } })
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  await screen.findByText('private response')
  await user.click(screen.getByRole('button', { name: 'Load more content' }))
  expect(
    await screen.findByText('Unable to read retained content')
  ).toBeVisible()
  expect(screen.queryByText('private response')).toBeNull()
})

test('UTF-8 split across pages is assembled without replacement characters', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce(page([event(1, '\xe4\xb8')], 1))
    .mockResolvedValueOnce(page([event(2, '\xad')]))
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  await user.click(
    await screen.findByRole('button', { name: 'Load more content' })
  )
  expect(await screen.findByText('中')).toBeVisible()
  expect(get).toHaveBeenLastCalledWith(
    '/api/log/content/req-one',
    expect.objectContaining({ params: { after: 1, limit: 8 } })
  )
  expect(screen.queryByText('�')).toBeNull()
})

test('an empty archive shows its possible states without claiming the request was dropped', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: null },
  })
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  expect(await screen.findByText('No retained content')).toBeVisible()
  expect(
    screen.getByText(
      'Content may not have been recorded, may still be archiving, or may have expired.'
    )
  ).toBeVisible()
})

test('binary fragments produce the original combined base64 instead of concatenating padding', () => {
  expect(
    decodeContent([event(1, '\x00'), event(2, '\xff')], 'response', true)
  ).toEqual({ text: 'AP8=', binary: true })
})

test('an archived failure is shown separately from archive completion', async () => {
  const response = page()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      ...response.data,
      data: { ...response.data.data, outcome: 'failed' },
    },
  })
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  expect(await screen.findByText('Archive complete')).toBeVisible()
  expect(screen.getByText('Model response: Failed')).toBeVisible()
})

test('an expired returned record is discarded before any plaintext is displayed', async () => {
  const response = page()
  response.data.data.expires_at = 1
  vi.spyOn(api, 'get').mockResolvedValue(response)
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  expect(await screen.findByText('No retained content')).toBeVisible()
  expect(screen.queryByText('private response')).toBeNull()
})

test('a malformed cursor is rejected instead of endlessly loading the same content', async () => {
  vi.spyOn(api, 'get').mockResolvedValue(
    page([event(1, 'private response')], 0)
  )
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  expect(
    await screen.findByText('Unable to read retained content')
  ).toBeVisible()
  expect(screen.queryByText('private response')).toBeNull()
})

test('reaching the byte budget disables further paging without truncating the loaded response', async () => {
  const chunk = 'x'.repeat(256 * 1024)
  const first = Array.from({ length: 8 }, (_, i) => event(i + 1, chunk))
  const second = Array.from({ length: 8 }, (_, i) => event(i + 9, chunk))
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce(page(first, 8))
    .mockResolvedValueOnce(page(second, 16))
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'View request and response' })
  )
  await user.click(
    await screen.findByRole('button', { name: 'Load more content' })
  )
  expect(await screen.findByRole('status')).toHaveTextContent(
    'The display limit has been reached.'
  )
  expect(screen.queryByRole('button', { name: 'Load more content' })).toBeNull()
  expect(get).toHaveBeenCalledTimes(2)
  expect(
    within(screen.getByRole('region', { name: 'Response body' })).getByText(
      /^x+$/
    ).textContent
  ).toHaveLength(4 * 1024 * 1024)
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Copy response body' })
    ).toBeVisible()
  )
})
