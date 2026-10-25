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
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import * as pricingApi from '@/features/pricing/api'
import type { PricingData, PricingModel } from '@/features/pricing/types'
import * as api from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { OmnivoLanding } from '../components/omnivo-landing'

let queryClient: QueryClient
const catalog: PricingData = {
  success: true,
  data: [],
  vendors: [],
  group_ratio: { default: 1 },
  usable_group: { default: { desc: 'Default', ratio: 1 } },
  supported_endpoint: {},
  auto_groups: [],
}
const model = {
  id: 1,
  model_name: 'provider/fixture-model',
  description: 'A model supplied by the current catalog.',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 3,
  enable_groups: ['default'],
} satisfies PricingModel

beforeEach(() => {
  localStorage.clear()
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  queryClient.setQueryData(['status'], {})
  queryClient.setQueryData(['official-model-prices'], {
    version: 1,
    references: [],
  })
  vi.spyOn(pricingApi, 'getPricing').mockResolvedValue(catalog)
  vi.spyOn(api, 'getStatus').mockResolvedValue({})
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
})

afterEach(() => {
  queryClient.clear()
  localStorage.clear()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: DEFAULT_CURRENCY_CONFIG })
})

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
  const details = createRoute({
    getParentRoute: () => root,
    path: '/pricing/$modelId',
    component: () => <h1>Model guide destination</h1>,
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
  const signin = createRoute({
    getParentRoute: () => root,
    path: '/sign-in',
    component: () => <h1>Sign in destination</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([
      home,
      pricing,
      details,
      keys,
      docs,
      signup,
      signin,
    ]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
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

test('submitting a model search trims the query and opens the catalog in the displayed token unit', async () => {
  const user = userEvent.setup()
  const router = renderLanding()

  await user.type(
    await screen.findByRole('textbox', { name: 'Search models' }),
    '  provider/fixture  {Enter}'
  )

  expect(
    await screen.findByRole('heading', { name: 'Model catalog destination' })
  ).toBeVisible()
  expect(router.state.location.search).toEqual({
    search: 'provider/fixture',
    tokenUnit: 'M',
  })
})

test('clearing a search empties the input without submitting the form', async () => {
  const user = userEvent.setup()
  const router = renderLanding()
  const search = await screen.findByRole('textbox', { name: 'Search models' })

  await user.type(search, 'temporary query')
  await user.click(screen.getByRole('button', { name: 'Clear search' }))

  expect(search).toHaveValue('')
  expect(router.state.location.pathname).toBe('/')
})

test('available models show real catalog data and open an independent model guide', async () => {
  const user = userEvent.setup()
  vi.mocked(pricingApi.getPricing).mockResolvedValue({
    ...catalog,
    data: [model],
  })
  const router = renderLanding()

  expect(
    await screen.findByRole('heading', { name: model.model_name })
  ).toBeVisible()
  expect(screen.getByText(model.description)).toBeVisible()
  expect(screen.getByText('$2')).toBeVisible()
  expect(screen.getByText('$6')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Details' }))

  expect(
    await screen.findByRole('heading', { name: 'Model guide destination' })
  ).toBeVisible()
  expect(router.state.matches.at(-1)?.params).toEqual({
    modelId: model.model_name,
  })
  expect(router.state.location.search).toEqual({ tokenUnit: 'M' })
})

test('an empty catalog offers documentation without inventing model availability', async () => {
  renderLanding()

  expect(await screen.findByText('No models available yet')).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'View integration guide' })
  ).toHaveAttribute('href', '/docs')
  expect(
    screen.queryByRole('button', { name: 'Details' })
  ).not.toBeInTheDocument()
})

test('a pending catalog request shows loading until real model data arrives', async () => {
  let resolveRequest: ((value: PricingData) => void) | undefined
  const response = new Promise<PricingData>((resolve) => {
    resolveRequest = resolve
  })
  vi.mocked(pricingApi.getPricing).mockReturnValue(response)
  renderLanding()

  expect(await screen.findByText('Loading model catalog...')).toBeVisible()
  expect(screen.queryByText('No models available yet')).not.toBeInTheDocument()

  await act(async () => resolveRequest?.({ ...catalog, data: [model] }))

  expect(
    await screen.findByRole('heading', { name: model.model_name })
  ).toBeVisible()
  expect(screen.queryByText('Loading model catalog...')).not.toBeInTheDocument()
})

test('an anonymous visitor cannot fetch or see cached models from a protected catalog', async () => {
  queryClient.setQueryData(['status'], {
    HeaderNavModules: { pricing: { enabled: true, requireAuth: true } },
  })
  queryClient.setQueryData(['pricing'], { ...catalog, data: [model] })
  renderLanding()

  expect(await screen.findByText('Your model catalog awaits')).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Sign in to explore' })
  ).toHaveAttribute('href', '/sign-in?redirect=%2Fpricing')
  expect(
    screen.queryByRole('heading', { name: model.model_name })
  ).not.toBeInTheDocument()
  expect(pricingApi.getPricing).not.toHaveBeenCalled()
})

test('signed-in visitors can explore a catalog that requires authentication', async () => {
  queryClient.setQueryData(['status'], {
    HeaderNavModules: { pricing: { enabled: true, requireAuth: true } },
  })
  vi.mocked(pricingApi.getPricing).mockResolvedValue({
    ...catalog,
    data: [model],
  })
  renderLanding(true)

  expect(
    await screen.findByRole('heading', { name: model.model_name })
  ).toBeVisible()
  expect(
    screen.queryByRole('link', { name: 'Sign in to explore' })
  ).not.toBeInTheDocument()
})

test('a disabled catalog hides discovery and search without requesting prices', async () => {
  queryClient.setQueryData(['status'], {
    HeaderNavModules: { pricing: { enabled: false, requireAuth: false } },
  })
  queryClient.setQueryData(['pricing'], { ...catalog, data: [model] })
  renderLanding()

  expect(
    await screen.findByRole('heading', { name: /Many models/ })
  ).toBeVisible()
  expect(screen.queryByRole('search')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('heading', {
      name: 'Find your next model.',
    })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('heading', { name: model.model_name })
  ).not.toBeInTheDocument()
  expect(pricingApi.getPricing).not.toHaveBeenCalled()
})

test('a failed status request does not trust a persisted public catalog snapshot', async () => {
  queryClient.removeQueries({ queryKey: ['status'] })
  localStorage.setItem(
    'status',
    JSON.stringify({ HeaderNavModules: { pricing: true } })
  )
  vi.mocked(api.getStatus).mockRejectedValue(new Error('Status unavailable'))
  renderLanding()

  expect(
    await screen.findByText('Unable to load the model catalog')
  ).toBeVisible()
  expect(screen.queryByRole('search')).not.toBeInTheDocument()
  expect(pricingApi.getPricing).not.toHaveBeenCalled()
})

test('a catalog load failure can be retried without presenting it as an empty catalog', async () => {
  const user = userEvent.setup()
  vi.mocked(pricingApi.getPricing)
    .mockRejectedValueOnce(new Error('Pricing unavailable'))
    .mockResolvedValueOnce({ ...catalog, data: [model] })
  renderLanding()

  expect(
    await screen.findByText('Unable to load the model catalog')
  ).toBeVisible()
  expect(screen.queryByText('No models available yet')).not.toBeInTheDocument()
  await user.click(
    within(
      screen.getByRole('region', { name: 'Find your next model.' })
    ).getByRole('button', { name: 'Retry' })
  )

  expect(
    await screen.findByRole('heading', { name: model.model_name })
  ).toBeVisible()
})

test('the discount FAQ opens with the keyboard and explains savings without claiming a universal discount', async () => {
  const user = userEvent.setup()
  renderLanding()
  const question = await screen.findByRole('button', {
    name: 'How should I read a discount?',
  })

  expect(question).toHaveAttribute('aria-expanded', 'false')
  question.focus()
  await user.keyboard('{Enter}')

  expect(question).toHaveAttribute('aria-expanded', 'true')
  expect(
    screen.getByText(
      'Discounts compare the current Omnivo price with the provider’s standard API price for the same model and usage. Input and output savings can differ and change over time.'
    )
  ).toBeVisible()
})

test('model names are direct guide links while catalog prices keep their group context', async () => {
  vi.mocked(pricingApi.getPricing).mockResolvedValue({
    ...catalog,
    data: [model],
  })
  renderLanding()

  expect(
    await screen.findByRole('link', { name: model.model_name })
  ).toHaveAttribute('href', '/pricing/provider%2Ffixture-model?tokenUnit=M')
  expect(
    screen.getByText(
      'Base catalog prices shown. Your account group and usage can affect the final price.'
    )
  ).toBeVisible()
})

test('missing official references show the catalog entry without a fixed discount claim', async () => {
  renderLanding()
  const current = await screen.findByRole('region', {
    name: 'Current model pricing',
  })

  expect(
    await within(current).findByText('Explore current model prices')
  ).toBeVisible()
  expect(
    screen.queryByRole('figure', { name: '20% savings example' })
  ).not.toBeInTheDocument()
  expect(within(current).queryByText(/Save .*%/)).not.toBeInTheDocument()
  expect(
    screen.getByRole('link', { name: 'Compare model prices' })
  ).toHaveAttribute('href', '/pricing?tokenUnit=M')
  expect(
    screen.getByRole('link', { name: 'One connection. Better value.' })
  ).toHaveAttribute('href', '/#omnivo-pricing-story-title')
})

function setComparableCatalog() {
  const pricedModel = { ...model, vendor_id: 1 }
  const pricing = {
    ...catalog,
    data: [pricedModel],
    vendors: [{ id: 1, name: 'OpenAI' }],
  }
  vi.mocked(pricingApi.getPricing).mockResolvedValue(pricing)
  queryClient.setQueryData(['official-model-prices'], {
    version: 1,
    references: [
      {
        modelId: model.model_name,
        provider: 'openai',
        currency: 'USD',
        unit: 'million_tokens',
        scope: 'standard_text_flat',
        inputUsdPerMillion: 2.5,
        outputUsdPerMillion: 7.5,
        sourceUrl: 'https://openai.com/api/pricing/',
        checkedAt: new Date(Date.now() - 60_000).toISOString(),
        expiresAt: new Date(Date.now() + 86_400_000).toISOString(),
      },
    ],
  })
  return pricing
}

test('a new model price changes homepage savings from 20% to 50% without changing copy', async () => {
  const pricing = setComparableCatalog()
  renderLanding()
  const current = await screen.findByRole('region', {
    name: 'Current model pricing',
  })
  expect(await within(current).findAllByText('Save 20%')).toHaveLength(2)
  expect(
    within(current).queryByRole('region', { name: 'Half-price model offer' })
  ).not.toBeInTheDocument()

  await act(async () => {
    queryClient.setQueryData(['pricing'], {
      ...pricing,
      data: [{ ...pricing.data[0], model_ratio: 0.625 }],
    })
  })

  expect(await within(current).findAllByText('Save 50%')).toHaveLength(2)
  expect(within(current).queryByText('Save 20%')).not.toBeInTheDocument()
  expect(
    within(current).getByRole('link', { name: 'View official pricing source' })
  ).toHaveAttribute('href', 'https://openai.com/api/pricing/')
  expect(
    within(current).getByRole('region', { name: 'Half-price model offer' })
  ).toHaveTextContent('default')
  await act(async () => queryClient.setQueryData(['pricing'], pricing))
  expect(await within(current).findAllByText('Save 20%')).toHaveLength(2)
  expect(
    within(current).queryByRole('region', { name: 'Half-price model offer' })
  ).not.toBeInTheDocument()
})

test('homepage comparison keeps input, output, and account groups distinct', async () => {
  const pricing = setComparableCatalog()
  vi.mocked(pricingApi.getPricing).mockResolvedValue({
    ...pricing,
    data: [
      {
        ...pricing.data[0],
        completion_ratio: 3.75,
        enable_groups: ['default', 'premium'],
      },
    ],
    group_ratio: { default: 1, premium: 2 },
  })
  renderLanding()
  const current = await screen.findByRole('region', {
    name: 'Current model pricing',
  })

  expect(await within(current).findByText('Save 20%')).toBeVisible()
  expect(within(current).getByText('Same price')).toBeVisible()
  expect(within(current).getByText('60% higher')).toBeVisible()
  expect(within(current).getByText('100% higher')).toBeVisible()
  expect(within(current).getByText('premium')).toBeVisible()
})

test('a failed price refresh removes cached discounts until current prices can be retrieved', async () => {
  setComparableCatalog()
  renderLanding()
  const current = await screen.findByRole('region', {
    name: 'Current model pricing',
  })
  expect(await within(current).findAllByText('Save 20%')).toHaveLength(2)
  vi.mocked(pricingApi.getPricing).mockRejectedValue(
    new Error('Pricing refresh failed')
  )

  await act(async () => {
    await queryClient.refetchQueries({ queryKey: ['pricing'] })
  })

  expect(
    await within(current).findByText('Unable to load current prices')
  ).toBeVisible()
  expect(within(current).queryByText('Save 20%')).not.toBeInTheDocument()
})

test('private cached comparisons are not mounted for an anonymous visitor', async () => {
  const pricing = setComparableCatalog()
  queryClient.setQueryData(['pricing'], pricing)
  queryClient.setQueryData(['status'], {
    HeaderNavModules: { pricing: { enabled: true, requireAuth: true } },
  })
  renderLanding()
  const current = await screen.findByRole('region', {
    name: 'Current model pricing',
  })

  expect(
    await within(current).findByText('Explore current model prices')
  ).toBeVisible()
  expect(within(current).queryByText('Save 20%')).not.toBeInTheDocument()
  expect(
    within(current).queryByRole('link', { name: model.model_name })
  ).not.toBeInTheDocument()
  expect(pricingApi.getPricing).not.toHaveBeenCalled()
})
