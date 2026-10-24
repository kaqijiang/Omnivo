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
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { ModelDetailActions } from '../components/model-detail-actions'
import { ModelDetailsContent } from '../components/model-details'
import {
  ModelIntroduction,
  ModelQuestions,
} from '../components/model-introduction'
import { ModelRelatedModels } from '../components/model-related-models'
import type { PricingModel } from '../types'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

const model: PricingModel = {
  id: 1,
  model_name: 'provider/model-with-reasoning-in-the-name',
  vendor_id: 1,
  vendor_name: 'Example provider',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 2,
  enable_groups: ['default'],
}
const clients: QueryClient[] = []

function renderGuideLinks(content: ReactNode) {
  const root = createRootRoute()
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => content,
  })
  const detail = createRoute({
    getParentRoute: () => root,
    path: '/pricing/$modelId',
    component: () => <h1>Model guide destination</h1>,
  })
  const register = createRoute({
    getParentRoute: () => root,
    path: '/sign-up',
    component: () => <h1>Registration destination</h1>,
  })
  const keys = createRoute({
    getParentRoute: () => root,
    path: '/keys',
    component: () => <h1>API key destination</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([home, detail, register, keys]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
  return router
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser(null)
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})

afterEach(() => {
  for (const client of clients) client.clear()
  clients.length = 0
  useAuthStore.getState().auth.setUser(null)
  vi.restoreAllMocks()
})

it('does not infer capabilities or model descriptions from the model name or vendor description', () => {
  render(
    <ModelIntroduction
      model={{ ...model, vendor_description: 'Provider-wide description' }}
    />
  )
  expect(
    screen.queryByRole('region', { name: 'About this model' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByText('Provider-wide description')
  ).not.toBeInTheDocument()
})

it('renders safe model Markdown and workflows only for declared capabilities', () => {
  const { container } = render(
    <ModelIntroduction
      model={{
        ...model,
        description:
          '**A model description.** <script>alert(1)</script><img src="x" onerror="alert(1)">',
        capabilities: ['tools'],
        input_modalities: ['text', 'image'],
      }}
    />
  )
  expect(screen.getByText('A model description.').tagName).toBe('STRONG')
  expect(
    screen.getByRole('heading', { name: 'Tool-assisted workflows' })
  ).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Image understanding' })
  ).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Reasoning workflows' })
  ).not.toBeInTheDocument()
  expect(container.querySelector('script, [onerror]')).toBeNull()
})

it('reveals the exact model ID through a keyboard-accessible FAQ', async () => {
  const user = userEvent.setup()
  render(<ModelQuestions modelName={model.model_name} />)
  const question = screen.getByRole('button', {
    name: 'Which model ID should I send?',
  })
  question.focus()
  await user.keyboard('{Enter}')
  expect(question).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByText(model.model_name)).toBeVisible()
})

it('shares an encoded model page URL and directs visitors to registration', async () => {
  const user = userEvent.setup()
  const writeText = vi
    .spyOn(navigator.clipboard, 'writeText')
    .mockResolvedValue()
  renderGuideLinks(
    <ModelDetailActions modelName={model.model_name} showFullPageLink />
  )
  expect(
    await screen.findByRole('link', { name: 'Get an API key' })
  ).toHaveAttribute('href', '/sign-up')
  expect(
    screen.getByRole('link', { name: 'Full model guide' })
  ).toHaveAttribute('href', `/pricing/${encodeURIComponent(model.model_name)}`)
  await user.click(screen.getByRole('button', { name: 'Copy model page link' }))
  expect(writeText).toHaveBeenCalledWith(
    `${window.location.origin}/pricing/${encodeURIComponent(model.model_name)}`
  )
})

it('directs a signed-in reader to their API keys', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'reader', role: 1 })
  renderGuideLinks(<ModelDetailActions modelName={model.model_name} />)
  expect(
    await screen.findByRole('link', { name: 'Get an API key' })
  ).toHaveAttribute('href', '/keys')
})

it('preserves catalog filters and price display preferences when opening the full guide', async () => {
  const user = userEvent.setup()
  const fullPageSearch = {
    search: 'model',
    vendor: '1',
    group: 'premium',
    tokenUnit: 'K' as const,
    view: 'table' as const,
    rechargePrice: true,
  }
  const router = renderGuideLinks(
    <ModelDetailActions
      modelName={model.model_name}
      showFullPageLink
      fullPageSearch={fullPageSearch}
    />
  )
  await user.click(
    await screen.findByRole('link', { name: 'Full model guide' })
  )
  expect(
    await screen.findByRole('heading', { name: 'Model guide destination' })
  ).toBeVisible()
  expect(router.state.location.search).toEqual(fullPageSearch)
})

it('suggests only related models supplied by the visible catalog and links to full guides', async () => {
  const user = userEvent.setup()
  renderGuideLinks(
    <ModelRelatedModels
      model={{ ...model, capabilities: ['tools'] }}
      models={[
        model,
        { ...model, id: 2, model_name: 'same-provider' },
        {
          ...model,
          id: 3,
          model_name: 'shared-tools',
          vendor_id: 2,
          capabilities: ['tools'],
        },
        { ...model, id: 4, model_name: 'unrelated', vendor_id: 3 },
      ]}
    />
  )
  const section = await screen.findByRole('region', {
    name: 'Keep exploring models',
  })
  expect(
    within(section).getByRole('heading', { name: 'same-provider' })
  ).toBeVisible()
  expect(
    within(section).getByRole('heading', { name: 'shared-tools' })
  ).toBeVisible()
  expect(within(section).queryByText('unrelated')).not.toBeInTheDocument()
  expect(within(section).queryByText(model.model_name)).not.toBeInTheDocument()
  await user.click(
    within(section).getAllByRole('link', { name: 'View model guide' })[0]
  )
  expect(
    await screen.findByRole('heading', { name: 'Model guide destination' })
  ).toBeVisible()
})

it('hides related-model recommendations when the catalog contains no matching models', () => {
  render(<ModelRelatedModels model={model} models={[model]} />)
  expect(
    screen.queryByRole('region', { name: 'Keep exploring models' })
  ).not.toBeInTheDocument()
})

it('opens API examples from the primary action and resets the tab when the selected model changes', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { groups: [] } },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const props = {
    groupRatio: { default: 1 },
    usableGroup: { default: { desc: '', ratio: 1 } },
    autoGroups: [],
    endpointMap: {},
    tokenUnit: 'M' as const,
    priceRate: 1,
    usdExchangeRate: 1,
  }
  const view = render(
    <QueryClientProvider client={client}>
      <ModelDetailsContent model={model} {...props} />
    </QueryClientProvider>
  )
  await user.click(screen.getByRole('button', { name: 'View API examples' }))
  expect(screen.getByRole('tab', { name: 'API' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  await act(async () =>
    view.rerender(
      <QueryClientProvider client={client}>
        <ModelDetailsContent
          model={{ ...model, model_name: 'another-model' }}
          {...props}
        />
      </QueryClientProvider>
    )
  )
  expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
})
