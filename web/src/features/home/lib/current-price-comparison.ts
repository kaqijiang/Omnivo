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
  compareOfficialTokenPrices,
  findOfficialPriceReference,
  getOfficialComparisonGroups,
  type OfficialPriceCatalog,
} from '@/features/pricing/lib/official-pricing'
import type { PricingModel } from '@/features/pricing/types'

export function findHomePriceComparison(
  models: PricingModel[],
  groupRatio: Record<string, number>,
  catalog: OfficialPriceCatalog | null | undefined,
  now: number
): { model: PricingModel; halfPriceGroup?: string } | undefined {
  let fallback: { model: PricingModel } | undefined

  for (const model of models) {
    const reference = findOfficialPriceReference(model, catalog, now)
    if (!reference) continue

    for (const group of getOfficialComparisonGroups(model, groupRatio)) {
      const comparison = compareOfficialTokenPrices(model, reference, {
        group,
        groupRatio,
        now,
      })
      if (!comparison || comparison.length !== 2) continue
      fallback ??= { model }

      if (comparison.every((row) => row.savingsPercent >= 50)) {
        return { model, halfPriceGroup: group }
      }
    }
  }

  return fallback
}
