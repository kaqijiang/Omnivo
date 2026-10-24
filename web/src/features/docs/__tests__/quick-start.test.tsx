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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { DocsContent } from '..'

function renderGuide() {
  const root = createRootRoute({ component: Outlet })
  const docs = createRoute({
    getParentRoute: () => root,
    path: '/docs',
    component: DocsContent,
  })
  const register = createRoute({
    getParentRoute: () => root,
    path: '/sign-up',
    component: () => <h1>Account destination</h1>,
  })
  const pricing = createRoute({
    getParentRoute: () => root,
    path: '/pricing',
    component: () => <h1>Model destination</h1>,
  })
  const keys = createRoute({
    getParentRoute: () => root,
    path: '/keys',
    component: () => <h1>Key destination</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([docs, register, pricing, keys]),
    history: createMemoryHistory({ initialEntries: ['/docs'] }),
  })
  render(<RouterProvider router={router} />)
  return router
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})

it('guides readers to registration, key creation, and models without requiring an account', async () => {
  const user = userEvent.setup()
  const router = renderGuide()
  await screen.findByRole('heading', {
    name: 'Your first request, made simple.',
  })
  expect(
    screen.getByRole('link', { name: 'Create an account' })
  ).toHaveAttribute('href', '/sign-up')
  expect(
    screen.getByRole('link', { name: 'Create an API key' })
  ).toHaveAttribute('href', '/keys')
  expect(
    screen.getByRole('link', { name: 'Choose an available model' })
  ).toHaveAttribute('href', '/pricing')
  expect(
    screen.getByText(/Replace MODEL_ID with an enabled model/)
  ).toBeVisible()
  await user.click(
    screen.getByRole('link', { name: 'Choose an available model' })
  )
  await screen.findByRole('heading', { name: 'Model destination' })
  expect(router.state.location.pathname).toBe('/pricing')
})

it('copies the deployment URL and selected SDK example, including keyboard tab selection', async () => {
  const user = userEvent.setup()
  renderGuide()
  await screen.findByRole('heading', {
    name: 'Your first request, made simple.',
  })
  await user.click(screen.getByRole('button', { name: 'Copy API base URL' }))
  expect(await navigator.clipboard.readText()).toBe(
    `${window.location.origin}/v1`
  )

  await user.click(screen.getByRole('tab', { name: 'Python' }))
  const python = screen.getByRole('tabpanel')
  expect(within(python).getByText('python -m pip install openai')).toBeVisible()
  await user.click(
    within(python).getByRole('button', { name: 'Copy request example' })
  )
  expect(await navigator.clipboard.readText()).toContain(
    'api_key=os.environ["OMNIVO_API_KEY"]'
  )
  expect(await navigator.clipboard.readText()).toContain(
    `base_url="${window.location.origin}/v1"`
  )
  expect(await navigator.clipboard.readText()).toContain('model="MODEL_ID"')

  await user.click(screen.getByRole('tab', { name: 'Python' }))
  await user.keyboard('{ArrowRight}')
  expect(screen.getByRole('tab', { name: 'Node.js' })).toHaveFocus()
  await user.keyboard('{Enter}')
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'Node.js' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  )
  await user.click(
    within(screen.getByRole('tabpanel')).getByRole('button', {
      name: 'Copy request example',
    })
  )
  expect(await navigator.clipboard.readText()).toContain(
    'apiKey: process.env.OMNIVO_API_KEY'
  )
})

it('copies a streaming request and explains authentication and capacity errors', async () => {
  const user = userEvent.setup()
  renderGuide()
  await screen.findByRole('heading', { name: 'Stream a response' })
  await user.click(
    screen.getByRole('button', { name: 'Copy streaming request' })
  )
  const code = await navigator.clipboard.readText()
  expect(code).toContain(
    `curl --no-buffer "${window.location.origin}/v1/chat/completions"`
  )
  expect(code).toContain('"stream": true')
  expect(code).toContain('Authorization: Bearer $OMNIVO_API_KEY')
  const table = screen.getByRole('table')
  expect(
    within(table).getByRole('row', { name: /401.*Check that OMNIVO_API_KEY/ })
  ).toBeVisible()
  expect(
    within(table).getByRole('row', { name: /429.*rate or quota limits/ })
  ).toBeVisible()
})
