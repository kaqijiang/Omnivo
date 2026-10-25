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
import { Link } from '@tanstack/react-router'
import { BadgePercent } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { GroupBadge } from '@/components/group-badge'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { ModelOfficialPriceComparison } from '@/features/pricing/components/model-official-price-comparison'
import { useOfficialPrices } from '@/features/pricing/hooks/use-official-prices'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'

import { findHomePriceComparison } from '../lib/current-price-comparison'

export function CurrentPriceOverview() {
  const { t } = useTranslation()
  return (
    <EmptyState
      icon={BadgePercent}
      className='min-h-60 px-0'
      title={t('Explore current model prices')}
      description={t(
        'Discounts vary by model and billing item. Explore the catalog for current prices and available official-price comparisons.'
      )}
    />
  )
}

/** Mounted only after the homepage has checked current catalog access. */
export function OmnivoCurrentPrices() {
  const { t } = useTranslation()
  const pricing = usePricingData(true, 60_000)
  const official = useOfficialPrices()

  if (pricing.isLoading || official.isPending) {
    return <LoadingState message={t('Loading current price comparison...')} />
  }
  if (pricing.error || official.error) {
    return (
      <ErrorState
        className='min-h-60 px-0'
        title={t('Unable to load current prices')}
        description={t('Refresh to see the latest available prices.')}
        onRetry={() =>
          void Promise.all([pricing.refetch(), official.refetch()])
        }
      />
    )
  }

  const comparison = findHomePriceComparison(
    pricing.models,
    pricing.groupRatio,
    official.data,
    official.comparisonTime
  )
  if (!comparison) return <CurrentPriceOverview />
  const model = comparison.model

  return (
    <div className='flex min-w-0 flex-col gap-5'>
      {comparison.halfPriceGroup && (
        <section
          aria-label={t('Half-price model offer')}
          className='flex flex-col gap-3'
        >
          <h3 className='text-primary text-2xl leading-tight font-semibold tracking-tight'>
            {t('Half the official price, or less.')}
          </h3>
          <p className='text-muted-foreground flex flex-wrap items-center gap-2 text-xs'>
            {t('For this model in group:')}
            <GroupBadge group={comparison.halfPriceGroup} />
          </p>
        </section>
      )}
      <div className='flex min-w-0 flex-col gap-3'>
        <Badge variant='secondary' className='w-fit'>
          {t('Current model pricing')}
        </Badge>
        <Link
          to='/pricing/$modelId'
          params={{ modelId: model.model_name }}
          search={{ tokenUnit: 'M' }}
          className='text-lg font-semibold [overflow-wrap:anywhere] underline-offset-4 hover:underline'
        >
          {model.model_name}
        </Link>
      </div>
      <ModelOfficialPriceComparison
        model={model}
        groupRatio={pricing.groupRatio}
        tokenUnit='M'
        compact
      />
    </div>
  )
}
