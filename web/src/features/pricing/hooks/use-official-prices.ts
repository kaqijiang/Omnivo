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
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'

import { api } from '@/lib/api'

import { parseOfficialPriceCatalog } from '../lib/official-pricing'

export function useOfficialPrices() {
  const query = useQuery({
    queryKey: ['official-model-prices'],
    queryFn: async ({ signal }) => {
      const response = await api.get<unknown>('/official-model-prices.json', {
        signal,
        // Revalidate the public catalog even when static assets have a long TTL.
        headers: { 'Cache-Control': 'no-cache' },
      })
      return parseOfficialPriceCatalog(response.data)
    },
    staleTime: 5 * 60 * 1000,
    refetchInterval: 5 * 60 * 1000,
    retry: false,
    meta: { errorToast: false },
  })
  const [comparisonTime, setComparisonTime] = useState(() => Date.now())

  useEffect(() => {
    let timer: number | undefined
    const refreshComparisonTime = () => {
      if (timer !== undefined) window.clearTimeout(timer)
      const now = Date.now()
      setComparisonTime(now)
      let nextExpiry = Number.POSITIVE_INFINITY
      for (const reference of query.data?.references ?? []) {
        const expiresAt = Date.parse(reference.expiresAt)
        if (expiresAt > now) nextExpiry = Math.min(nextExpiry, expiresAt)
      }
      if (Number.isFinite(nextExpiry)) {
        // Long-lived references can exceed the browser's signed 32-bit delay.
        timer = window.setTimeout(
          refreshComparisonTime,
          Math.min(nextExpiry - now, 2_147_483_647)
        )
      }
    }

    refreshComparisonTime()
    window.addEventListener('focus', refreshComparisonTime)
    document.addEventListener('visibilitychange', refreshComparisonTime)
    return () => {
      if (timer !== undefined) window.clearTimeout(timer)
      window.removeEventListener('focus', refreshComparisonTime)
      document.removeEventListener('visibilitychange', refreshComparisonTime)
    }
    // A successful refresh must advance the clock even when JSON is unchanged.
  }, [query.data, query.dataUpdatedAt])

  return { ...query, comparisonTime }
}
