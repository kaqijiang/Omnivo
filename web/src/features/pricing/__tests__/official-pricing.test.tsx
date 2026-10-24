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
import type { AxiosAdapter } from 'axios'
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import {
  ModelOfficialPriceComparison,
  type ModelOfficialPriceComparisonProps,
} from '../components/model-official-price-comparison'
import {
  compareOfficialTokenPrices,
  findOfficialPriceReference,
  parseOfficialPriceCatalog,
  type OfficialPriceReference,
} from '../lib/official-pricing'
import type { PricingModel } from '../types'

const NOW = Date.parse('2026-10-24T12:00:00Z')
// A synthetic model identity keeps test-only amounts out of real model claims.
const reference: OfficialPriceReference = {
  modelId: 'comparison-fixture',
  provider: 'openai',
  currency: 'USD',
  unit: 'million_tokens',
  scope: 'standard_text_flat',
  inputUsdPerMillion: 2,
  outputUsdPerMillion: 8,
  sourceUrl: 'https://openai.com/api/pricing/',
  checkedAt: '2026-10-23T12:00:00Z',
  expiresAt: '2026-10-30T12:00:00Z',
}
const model: PricingModel = {
  id: 1,
  model_name: 'comparison-fixture',
  vendor_name: 'OpenAI',
  quota_type: 0,
  model_ratio: 0.8,
  completion_ratio: 5,
  enable_groups: ['default', 'premium'],
}
const groupRatio = { default: 1, premium: 1.5 }
const catalog = { version: 1, references: [reference] } as const
let client: QueryClient

beforeEach(async () => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW)
  await i18next.changeLanguage('en')
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
})

afterEach(async () => {
  client.clear()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  await i18next.changeLanguage('en')
  vi.restoreAllMocks()
})

function renderComparison(
  overrides: Partial<ModelOfficialPriceComparisonProps> = {}
) {
  return render(
    <QueryClientProvider client={client}>
      <ModelOfficialPriceComparison
        model={model}
        groupRatio={groupRatio}
        {...overrides}
      />
    </QueryClientProvider>
  )
}

describe('official price reference validation', () => {
  it('accepts a current, exact model and provider match from an official source', () => {
    const parsed = parseOfficialPriceCatalog(catalog)
    expect(findOfficialPriceReference(model, parsed, NOW)).toEqual(reference)
  })

  it.each([
    { sourceUrl: 'https://cometapi.com/pricing' },
    { sourceUrl: 'https://openai.com.attacker.example/pricing' },
    { sourceUrl: 'javascript:alert(1)' },
    { sourceUrl: 'not-a-url' },
    { sourceUrl: 'https://user:pass@openai.com/api/pricing' },
    { inputUsdPerMillion: 0 },
    { outputUsdPerMillion: Number.POSITIVE_INFINITY },
    { currency: 'CNY' },
    { scope: 'batch' },
  ])('rejects invalid or incomparable reference data: %j', (change) => {
    expect(
      parseOfficialPriceCatalog({
        version: 1,
        references: [{ ...reference, ...change }],
      })
    ).toBeNull()
  })

  it('rejects duplicate records instead of arbitrarily choosing one price', () => {
    expect(
      parseOfficialPriceCatalog({
        version: 1,
        references: [reference, { ...reference, inputUsdPerMillion: 10 }],
      })
    ).toBeNull()
  })

  it.each([
    { checkedAt: '2026-10-25T00:00:00Z' },
    { expiresAt: '2026-10-24T12:00:00Z' },
    { expiresAt: '2027-10-30T00:00:00Z' },
    { expiresAt: '2026-10-22T00:00:00Z' },
    { modelId: 'comparison-fixture-new-version' },
    {
      provider: 'deepseek',
      sourceUrl: 'https://api-docs.deepseek.com/quick_start/pricing',
    },
  ])(
    'does not compare a stale, future or mismatched reference: %j',
    (change) => {
      const parsed = parseOfficialPriceCatalog({
        version: 1,
        references: [{ ...reference, ...change }],
      })
      expect(findOfficialPriceReference(model, parsed, NOW)).toBeUndefined()
    }
  )

  it('does not infer provider ownership from a model name when metadata is absent', () => {
    expect(
      findOfficialPriceReference(
        { ...model, vendor_name: undefined },
        parseOfficialPriceCatalog(catalog),
        NOW
      )
    ).toBeUndefined()
  })
})

describe('official price comparisons', () => {
  it('uses the selected group and computes input/output differences independently', () => {
    const rows = compareOfficialTokenPrices(model, reference, {
      group: 'default',
      groupRatio,
      now: NOW,
    })
    expect(rows?.[0].omnivoUsdPerMillion).toBe(1.6)
    expect(rows?.[0].savingsPercent).toBeCloseTo(20)
    expect(rows?.[1].omnivoUsdPerMillion).toBe(8)
    expect(rows?.[1].savingsPercent).toBe(0)
    const premium = compareOfficialTokenPrices(model, reference, {
      group: 'premium',
      groupRatio,
      now: NOW,
    })
    expect(premium?.[0].savingsPercent).toBeCloseTo(-20)
    expect(premium?.[1].savingsPercent).toBeCloseTo(-50)
  })

  it('adjusts only the Omnivo price for recharge and preserves a zero-price group', () => {
    const rows = compareOfficialTokenPrices(model, reference, {
      group: 'default',
      groupRatio,
      showWithRecharge: true,
      priceRate: 4,
      usdExchangeRate: 8,
      now: NOW,
    })
    expect(rows?.[0].omnivoUsdPerMillion).toBe(0.8)
    expect(rows?.[0].officialUsdPerMillion).toBe(2)
    expect(rows?.[0].savingsPercent).toBe(60)
    const free = compareOfficialTokenPrices(model, reference, {
      group: 'default',
      groupRatio: { default: 0 },
      now: NOW,
    })
    expect(free?.map((row) => row.omnivoUsdPerMillion)).toEqual([0, 0])
  })

  it.each([
    { quota_type: 1 },
    {
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("standard", p * 2 + c * 8)',
    },
    {
      billing_usage_schema: {
        seconds: { type: 'number' as const, unit: 'second' as const },
      },
    },
    { model_ratio: Number.NaN },
    { completion_ratio: -1 },
  ])(
    'hides discounts for unsupported or invalid actual billing: %j',
    (change) => {
      expect(
        compareOfficialTokenPrices({ ...model, ...change }, reference, {
          group: 'default',
          groupRatio,
          now: NOW,
        })
      ).toBeNull()
    }
  )

  it('does not fall back to a default or lower price for an unavailable group or invalid recharge rate', () => {
    expect(
      compareOfficialTokenPrices(model, reference, {
        group: 'unknown',
        groupRatio,
        now: NOW,
      })
    ).toBeNull()
    expect(
      compareOfficialTokenPrices(model, reference, {
        group: 'default',
        groupRatio: {},
        now: NOW,
      })
    ).toBeNull()
    expect(
      compareOfficialTokenPrices(model, reference, {
        group: 'default',
        groupRatio,
        showWithRecharge: true,
        priceRate: 4,
        usdExchangeRate: 0,
        now: NOW,
      })
    ).toBeNull()
  })
})

describe('official price comparison display', () => {
  it('revalidates the public catalog on each request and shows updated reference prices', async () => {
    const previousAdapter = api.defaults.adapter
    let responseData: unknown = catalog
    const adapter = vi.fn<AxiosAdapter>(async (config) => ({
      data: responseData,
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }))
    api.defaults.adapter = adapter
    try {
      renderComparison({ selectedGroup: 'default' })
      await screen.findByText('Save 20%')
      responseData = {
        version: 1,
        references: [{ ...reference, inputUsdPerMillion: 4 }],
      }
      await act(() =>
        client.invalidateQueries({ queryKey: ['official-model-prices'] })
      )
      expect(await screen.findByText('Save 60%')).toBeVisible()
      expect(adapter).toHaveBeenCalledTimes(2)
      for (const [config] of adapter.mock.calls) {
        expect(config.url).toBe('/official-model-prices.json')
        expect(config.headers.get('Cache-Control')).toBe('no-cache')
        expect(config.signal).toBeInstanceOf(AbortSignal)
      }
    } finally {
      api.defaults.adapter = previousAdapter
    }
  })

  it('shows every available group including equal and higher prices, with the source and date', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: catalog })
    renderComparison()
    const table = await screen.findByRole('table', {
      name: 'Official and Omnivo price comparison',
    })
    expect(within(table).getAllByRole('row')).toHaveLength(5)
    expect(within(table).getByText('Save 20%')).toBeVisible()
    expect(within(table).getByText('Same price')).toBeVisible()
    expect(within(table).getByText('20% higher')).toBeVisible()
    expect(within(table).getByText('50% higher')).toBeVisible()
    expect(
      screen.getByRole('link', { name: 'View official pricing source' })
    ).toHaveAttribute('href', reference.sourceUrl)
    expect(screen.getByText('Reference checked on Oct 23, 2026')).toBeVisible()
  })

  it('honours the selected group, token unit and recharge-adjusted currency on both columns', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: catalog })
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 8,
      },
    })
    renderComparison({
      selectedGroup: 'premium',
      tokenUnit: 'K',
      showWithRecharge: true,
      priceRate: 4,
      usdExchangeRate: 8,
    })
    const table = await screen.findByRole('table')
    expect(within(table).getAllByRole('row')).toHaveLength(3)
    expect(within(table).queryByText('default')).not.toBeInTheDocument()
    expect(within(table).getByText('¥0.016')).toBeVisible()
    expect(within(table).getByText('¥0.0096')).toBeVisible()
    expect(within(table).getByText('Save 40%')).toBeVisible()
    expect(
      screen.getByText(/per 1K tokens · Recharge-adjusted prices/)
    ).toBeVisible()
  })

  it('responds to interface language changes and accepts Chinese interface codes', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: catalog })
    renderComparison({ selectedGroup: 'default' })
    await screen.findByRole('table')
    expect(screen.getByText('$1.6')).toBeVisible()
    await act(() => i18next.changeLanguage('fr'))
    expect(screen.getByText(/^1,6\s*\$$/)).toBeVisible()
    await act(() => i18next.changeLanguage('zhTW'))
    expect(screen.getByText('$1.6')).toBeVisible()
  })

  it.each([
    { version: 1, references: [] },
    {
      version: 1,
      references: [{ ...reference, expiresAt: '2026-10-22T00:00:00Z' }],
    },
    '<html>Missing static file</html>',
  ])(
    'renders no discount when reference data is missing or unusable: %j',
    async (data) => {
      vi.spyOn(api, 'get').mockResolvedValue({ data })
      renderComparison()
      await waitFor(() =>
        expect(client.getQueryState(['official-model-prices'])?.status).toBe(
          'success'
        )
      )
      expect(screen.queryByRole('table')).not.toBeInTheDocument()
      expect(screen.queryByText(/Save .*%/)).not.toBeInTheDocument()
    }
  )

  it('leaves the surrounding model page usable when the public reference request fails', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new Error('Reference unavailable'))
    renderComparison()
    await waitFor(() =>
      expect(client.getQueryState(['official-model-prices'])?.status).toBe(
        'error'
      )
    )
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
