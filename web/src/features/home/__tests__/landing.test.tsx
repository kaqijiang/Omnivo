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
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { OmnivoLanding } from '../components/omnivo-landing'

function renderLanding(isAuthenticated = false) {
  const root = createRootRoute()
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <OmnivoLanding isAuthenticated={isAuthenticated} />,
  })
  const pricing = createRoute({
    getParentRoute: () => root,
    path: '/pricing',
    component: () => <h1>Model catalog destination</h1>,
  })
  const keys = createRoute({
    getParentRoute: () => root,
    path: '/keys',
    component: () => <h1>API key destination</h1>,
  })
  const docs = createRoute({
    getParentRoute: () => root,
    path: '/docs',
    component: () => <h1>Documentation destination</h1>,
  })
  const signup = createRoute({
    getParentRoute: () => root,
    path: '/sign-up',
    component: () => <h1>Registration destination</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([home, pricing, keys, docs, signup]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
}

test('visitors can reach registration and explore the model catalog', async () => {
  const user = userEvent.setup()
  renderLanding()

  const startLinks = await screen.findAllByRole('link', {
    name: 'Start building',
  })
  expect(startLinks).toHaveLength(2)
  for (const link of startLinks) {
    expect(link).toHaveAttribute('href', '/sign-up')
  }
  await user.click(screen.getByRole('link', { name: 'Explore models' }))
  expect(
    await screen.findByRole('heading', { name: 'Model catalog destination' })
  ).toBeVisible()
})

test('signed-in users go directly to their API keys instead of registration', async () => {
  const user = userEvent.setup()
  renderLanding(true)

  const keyLinks = await screen.findAllByRole('link', {
    name: 'Manage API keys',
  })
  expect(
    screen.queryByRole('link', { name: 'Start building' })
  ).not.toBeInTheDocument()
  await user.click(keyLinks[0])
  expect(
    await screen.findByRole('heading', { name: 'API key destination' })
  ).toBeVisible()
})

test('the quickstart opens the local documentation', async () => {
  const user = userEvent.setup()
  renderLanding()

  await user.click(
    await screen.findByRole('link', { name: 'Read the quickstart' })
  )
  expect(
    await screen.findByRole('heading', { name: 'Documentation destination' })
  ).toBeVisible()
})

test('copying the request preserves shell continuation and clear credential placeholders', async () => {
  const user = userEvent.setup()
  const writeText = vi
    .spyOn(navigator.clipboard, 'writeText')
    .mockResolvedValue(undefined)
  renderLanding()

  await user.click(
    await screen.findByRole('button', { name: 'Copy example request' })
  )
  expect(await screen.findByRole('button', { name: 'Copied' })).toBeVisible()
  expect(writeText).toHaveBeenCalledTimes(1)
  const copied = writeText.mock.calls[0][0]
  expect(copied).toContain('curl "$OMNIVO_BASE_URL/chat/completions"')
  expect(copied).toContain('Authorization: Bearer $OMNIVO_API_KEY')
  expect(copied).toContain('"model": "MODEL_ID"')
  expect(copied.split(`${String.fromCharCode(92)}\n`)).toHaveLength(4)
  expect(
    screen.getByText(
      'Set your API base URL and key, then replace MODEL_ID with a model available to your account.'
    )
  ).toBeVisible()
})
