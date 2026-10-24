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
import { z } from 'zod'

import { EXCLUDED_GROUPS } from '../constants'
import type { PricingModel } from '../types'
import { calculateTokenPrice } from './price'

// Explicit source ownership prevents a third-party comparison site from being
// presented as a vendor's official price. Add providers here after review.
const PROVIDERS = {
  openai: { names: ['openai'], hosts: ['openai.com', 'platform.openai.com'] },
  anthropic: {
    names: ['anthropic', 'claude'],
    hosts: [
      'anthropic.com',
      'www.anthropic.com',
      'docs.anthropic.com',
      'platform.claude.com',
    ],
  },
  deepseek: { names: ['deepseek'], hosts: ['api-docs.deepseek.com'] },
  google: {
    names: ['google', 'google ai', 'google gemini'],
    hosts: ['ai.google.dev', 'cloud.google.com'],
  },
} as const

const officialPriceSchema = z
  .strictObject({
    modelId: z.string().trim().min(1),
    provider: z.enum(['openai', 'anthropic', 'deepseek', 'google']),
    currency: z.literal('USD'),
    unit: z.literal('million_tokens'),
    scope: z.literal('standard_text_flat'),
    inputUsdPerMillion: z.number().finite().positive(),
    outputUsdPerMillion: z.number().finite().positive(),
    sourceUrl: z.url(),
    checkedAt: z.iso.datetime(),
    expiresAt: z.iso.datetime(),
  })
  .refine((reference) => {
    try {
      const url = new URL(reference.sourceUrl)
      return (
        url.protocol === 'https:' &&
        !url.username &&
        !url.password &&
        !url.port &&
        PROVIDERS[reference.provider].hosts.some(
          (host) => host === url.hostname
        )
      )
    } catch {
      return false
    }
  })

const officialCatalogSchema = z
  .strictObject({
    version: z.literal(1),
    references: z.array(officialPriceSchema),
  })
  .refine((catalog) => {
    const identities = catalog.references.map(
      (reference) => `${reference.provider}:${reference.modelId}`
    )
    return new Set(identities).size === identities.length
  })

export type OfficialPriceReference = z.infer<typeof officialPriceSchema>
export type OfficialPriceCatalog = z.infer<typeof officialCatalogSchema>

export type OfficialPriceRow = {
  type: 'input' | 'output'
  officialUsdPerMillion: number
  omnivoUsdPerMillion: number
  savingsPercent: number
}

export function parseOfficialPriceCatalog(
  value: unknown
): OfficialPriceCatalog | null {
  const parsed = officialCatalogSchema.safeParse(value)
  return parsed.success ? parsed.data : null
}

/** Exact IDs only: aliases and neighbouring model versions need their own record. */
export function findOfficialPriceReference(
  model: PricingModel,
  catalog: OfficialPriceCatalog | null | undefined,
  now = Date.now()
): OfficialPriceReference | undefined {
  if (!catalog || !Number.isFinite(now) || !model.vendor_name) return undefined
  const providerName = model.vendor_name.trim().toLowerCase()
  return catalog.references.find((reference) => {
    const checkedAt = Date.parse(reference.checkedAt)
    const expiresAt = Date.parse(reference.expiresAt)
    return (
      reference.modelId === model.model_name &&
      PROVIDERS[reference.provider].names.some(
        (name) => name === providerName
      ) &&
      checkedAt <= now &&
      now < expiresAt &&
      expiresAt > checkedAt &&
      expiresAt - checkedAt <= 30 * 24 * 60 * 60 * 1000
    )
  })
}

export function getOfficialComparisonGroups(
  model: PricingModel,
  groupRatio: Record<string, number>
): string[] {
  return [...new Set(model.enable_groups)].filter(
    (group) =>
      !EXCLUDED_GROUPS.includes(group) &&
      Object.hasOwn(groupRatio, group) &&
      Number.isFinite(groupRatio[group]) &&
      groupRatio[group] >= 0
  )
}

/** Only a standard flat text price is comparable without a request scenario. */
export function compareOfficialTokenPrices(
  model: PricingModel,
  reference: OfficialPriceReference,
  options: {
    group: string
    groupRatio: Record<string, number>
    showWithRecharge?: boolean
    priceRate?: number
    usdExchangeRate?: number
    now?: number
  }
): OfficialPriceRow[] | null {
  if (
    model.quota_type !== 0 ||
    (model.billing_mode && model.billing_mode !== 'ratio') ||
    model.billing_expr?.trim() ||
    model.billing_plugin_variants?.length ||
    Object.keys(model.billing_usage_schema ?? {}).length > 0 ||
    !getOfficialComparisonGroups(model, options.groupRatio).includes(
      options.group
    ) ||
    !findOfficialPriceReference(
      model,
      parseOfficialPriceCatalog({ version: 1, references: [reference] }),
      options.now
    ) ||
    !Number.isFinite(model.model_ratio) ||
    model.model_ratio < 0 ||
    !Number.isFinite(model.completion_ratio) ||
    model.completion_ratio < 0
  ) {
    return null
  }

  let rechargeMultiplier = 1
  if (options.showWithRecharge) {
    const priceRate = options.priceRate ?? 0
    const exchangeRate = options.usdExchangeRate ?? 0
    if (
      !Number.isFinite(priceRate) ||
      !Number.isFinite(exchangeRate) ||
      priceRate <= 0 ||
      exchangeRate <= 0
    ) {
      return null
    }
    rechargeMultiplier = priceRate / exchangeRate
  }
  const rows = (['input', 'output'] as const).map((type) => {
    const officialUsdPerMillion =
      type === 'input'
        ? reference.inputUsdPerMillion
        : reference.outputUsdPerMillion
    const omnivoUsdPerMillion =
      calculateTokenPrice(model, type, options.groupRatio[options.group]) *
      rechargeMultiplier
    return {
      type,
      officialUsdPerMillion,
      omnivoUsdPerMillion,
      savingsPercent: (1 - omnivoUsdPerMillion / officialUsdPerMillion) * 100,
    }
  })
  return rows.every(
    (row) =>
      Number.isFinite(row.officialUsdPerMillion) &&
      row.officialUsdPerMillion > 0 &&
      Number.isFinite(row.omnivoUsdPerMillion) &&
      row.omnivoUsdPerMillion >= 0 &&
      Number.isFinite(row.savingsPercent)
  )
    ? rows
    : null
}
