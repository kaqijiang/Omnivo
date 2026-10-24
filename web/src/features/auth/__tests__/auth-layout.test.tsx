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
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

import { AuthLayout } from '../auth-layout'

function renderLayout(onSubmit = vi.fn()) {
  const root = createRootRoute({ component: Outlet })
  const account = createRoute({
    getParentRoute: () => root,
    path: '/sign-in',
    component: () => (
      <AuthLayout>
        <h2>Existing account form</h2>
        <form
          onSubmit={(event) => {
            event.preventDefault()
            onSubmit()
          }}
        >
          <label htmlFor='identity'>Account name</label>
          <input id='identity' name='identity' />
          <button type='submit'>Continue</button>
        </form>
      </AuthLayout>
    ),
  })
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <h1>Home destination</h1>,
  })
  const docs = createRoute({
    getParentRoute: () => root,
    path: '/docs',
    component: () => <h1>Guide destination</h1>,
  })
  const pricing = createRoute({
    getParentRoute: () => root,
    path: '/pricing',
    component: () => <h1>Model destination</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([account, home, docs, pricing]),
    history: createMemoryHistory({ initialEntries: ['/sign-in'] }),
  })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

beforeEach(() => {
  localStorage.clear()
  const store = useSystemConfigStore.getState()
  store.setConfig({ systemName: 'Custom workspace', logo: '/custom-mark.svg' })
  store.setLoadedLogoUrl('/custom-mark.svg')
  store.setLoading(false)
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})

it('preserves configured branding and lets the existing child form receive input and submit', async () => {
  const user = userEvent.setup()
  const onSubmit = vi.fn()
  renderLayout(onSubmit)
  await screen.findByRole('heading', { name: 'Existing account form' })
  expect(screen.getByRole('link', { name: 'Go to home' })).toHaveTextContent(
    'Custom workspace'
  )
  await user.type(
    screen.getByRole('textbox', { name: 'Account name' }),
    'developer'
  )
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByRole('textbox', { name: 'Account name' })).toHaveValue(
    'developer'
  )
  expect(onSubmit).toHaveBeenCalledOnce()
})

it('offers a working documentation link without submitting the account form', async () => {
  const user = userEvent.setup()
  const onSubmit = vi.fn()
  const router = renderLayout(onSubmit)
  await screen.findByRole('navigation', { name: 'Explore Omnivo' })
  expect(screen.getByRole('link', { name: 'Models' })).toHaveAttribute(
    'href',
    '/pricing'
  )
  await user.click(screen.getByRole('link', { name: 'Documentation' }))
  await screen.findByRole('heading', { name: 'Guide destination' })
  expect(router.state.location.pathname).toBe('/docs')
  expect(onSubmit).not.toHaveBeenCalled()
})
