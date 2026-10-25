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
import { describe, expect, it } from 'vitest'

import type {
  OfficialPriceCatalog,
  OfficialPriceReference,
} from '@/features/pricing/lib/official-pricing'
import type { PricingModel } from '@/features/pricing/types'

import { findHomePriceComparison } from '../lib/current-price-comparison'

const NOW = Date.parse('2026-10-25T12:00:00Z')

function pricingModel(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    id: 1,
    model_name: 'home-price-fixture',
    vendor_name: 'OpenAI',
    quota_type: 0,
    model_ratio: 0.8,
    completion_ratio: 4,
    enable_groups: ['default'],
    ...overrides,
  }
}

function officialReference(
  modelId = 'home-price-fixture',
  overrides: Partial<OfficialPriceReference> = {}
): OfficialPriceReference {
  return {
    modelId,
    provider: 'openai',
    currency: 'USD',
    unit: 'million_tokens',
    scope: 'standard_text_flat',
    inputUsdPerMillion: 2,
    outputUsdPerMillion: 8,
    sourceUrl: 'https://openai.com/api/pricing/',
    checkedAt: '2026-10-24T12:00:00Z',
    expiresAt: '2026-10-30T12:00:00Z',
    ...overrides,
  }
}

describe('home price comparison selection', () => {
  it('does not advertise half price when a 0.5 group multiplier only yields 80% of official prices', () => {
    const model = pricingModel({ model_ratio: 1.6 })

    expect(
      findHomePriceComparison(
        [model],
        { default: 0.5 },
        { version: 1, references: [officialReference()] },
        NOW
      )
    ).toEqual({ model })
  })

  it('does not advertise half price when only input reaches half price and output remains at 80%', () => {
    const model = pricingModel({ model_ratio: 0.5, completion_ratio: 6.4 })

    expect(
      findHomePriceComparison(
        [model],
        { default: 1 },
        { version: 1, references: [officialReference()] },
        NOW
      )
    ).toEqual({ model })
  })

  it('prefers the first qualifying model over an earlier ordinary comparison and a later cheaper model', () => {
    const ordinary = pricingModel()
    const qualifying = pricingModel({
      id: 2,
      model_name: 'qualifying-fixture',
      model_ratio: 0.5,
    })
    const cheaper = pricingModel({
      id: 3,
      model_name: 'cheaper-fixture',
      model_ratio: 0.25,
    })

    expect(
      findHomePriceComparison(
        [ordinary, qualifying, cheaper],
        { default: 1 },
        {
          version: 1,
          references: [
            officialReference(),
            officialReference(qualifying.model_name),
            officialReference(cheaper.model_name),
          ],
        },
        NOW
      )
    ).toEqual({ model: qualifying, halfPriceGroup: 'default' })
  })

  it('identifies the first available qualifying group instead of selecting the lowest multiplier', () => {
    const model = pricingModel({
      enable_groups: ['unavailable', 'standard', 'first-qualified', 'lower'],
    })

    expect(
      findHomePriceComparison(
        [model],
        { lower: 0.25, 'first-qualified': 0.625, standard: 1 },
        { version: 1, references: [officialReference()] },
        NOW
      )
    ).toEqual({ model, halfPriceGroup: 'first-qualified' })
  })

  it('falls back to the first current ordinary comparison after a qualifying reference expires', () => {
    const expired = pricingModel({ model_ratio: 0.5 })
    const ordinary = pricingModel({
      id: 2,
      model_name: 'current-ordinary-fixture',
    })

    expect(
      findHomePriceComparison(
        [expired, ordinary],
        { default: 1 },
        {
          version: 1,
          references: [
            officialReference(expired.model_name, {
              expiresAt: new Date(NOW).toISOString(),
            }),
            officialReference(ordinary.model_name),
          ],
        },
        NOW
      )
    ).toEqual({ model: ordinary })
  })

  it.each<{
    reason: string
    catalog: OfficialPriceCatalog | null | undefined
  }>([
    { reason: 'missing catalog', catalog: undefined },
    { reason: 'unusable catalog', catalog: null },
    { reason: 'empty catalog', catalog: { version: 1, references: [] } },
    {
      reason: 'different model version',
      catalog: {
        version: 1,
        references: [officialReference('home-price-fixture-v2')],
      },
    },
    {
      reason: 'untrusted source',
      catalog: {
        version: 1,
        references: [
          officialReference('home-price-fixture', {
            sourceUrl: 'https://comparison.example/pricing',
          }),
        ],
      },
    },
  ])('returns no offer for a $reason', ({ catalog }) => {
    expect(
      findHomePriceComparison(
        [pricingModel({ model_ratio: 0.5 })],
        { default: 1 },
        catalog,
        NOW
      )
    ).toBeUndefined()
  })
})
