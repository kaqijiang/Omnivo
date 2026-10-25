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
import { ArrowRight, ExternalLink } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { GroupBadge } from '@/components/group-badge'
import { Badge } from '@/components/ui/badge'
import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { TOKEN_UNIT_DIVISORS } from '../constants'
import { useOfficialPrices } from '../hooks/use-official-prices'
import {
  compareOfficialTokenPrices,
  findOfficialPriceReference,
  getOfficialComparisonGroups,
  type OfficialPriceRow,
} from '../lib/official-pricing'
import type { PricingModel, TokenUnit } from '../types'

export interface ModelOfficialPriceComparisonProps {
  model: PricingModel
  groupRatio: Record<string, number>
  selectedGroup?: string
  tokenUnit?: TokenUnit
  showWithRecharge?: boolean
  priceRate?: number
  usdExchangeRate?: number
  compact?: boolean
}

type GroupedOfficialPriceRow = OfficialPriceRow & { group: string }

export function ModelOfficialPriceComparison(
  props: ModelOfficialPriceComparisonProps
) {
  const { t, i18n } = useTranslation()
  const titleId = useId()
  const { data, error, comparisonTime } = useOfficialPrices()
  // The shared money formatter reads this store; subscribe for currency changes.
  useSystemConfigStore((state) => state.config.currency)
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const reference = findOfficialPriceReference(
    props.model,
    error ? null : data,
    comparisonTime
  )
  if (!reference) return null

  const groups = getOfficialComparisonGroups(
    props.model,
    props.groupRatio
  ).filter(
    (group) =>
      props.selectedGroup === undefined || group === props.selectedGroup
  )
  const rows = groups.flatMap((group) => {
    const comparison = compareOfficialTokenPrices(props.model, reference, {
      ...props,
      group,
      now: comparisonTime,
    })
    return comparison?.map((row) => ({ ...row, group })) ?? []
  })
  if (rows.length === 0) return null
  const divisor = TOKEN_UNIT_DIVISORS[props.tokenUnit ?? 'M']
  const unitLabel =
    props.tokenUnit === 'K' ? t('per 1K tokens') : t('per 1M tokens')
  const checkedDate = new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeZone: 'UTC',
  }).format(new Date(reference.checkedAt))
  const Heading = props.compact ? 'h3' : 'h2'
  const formatPrice = (usdPerMillion: number): string =>
    formatBillingCurrencyFromUSD(usdPerMillion / divisor, {
      digitsLarge: 4,
      digitsSmall: 6,
      abbreviate: false,
      locale,
    })

  return (
    <section aria-labelledby={titleId} className='flex min-w-0 flex-col gap-4'>
      <div className='flex flex-col gap-2'>
        <Heading
          id={titleId}
          className={cn('font-semibold', props.compact ? 'text-sm' : 'text-lg')}
        >
          {t('Compare with official pricing')}
        </Heading>
        <p
          className={cn(
            'text-muted-foreground',
            props.compact ? 'text-xs leading-5' : 'text-sm leading-6'
          )}
        >
          {t(
            'Standard text input and output, compared in the same currency and token unit.'
          )}
        </p>
        <p className='text-muted-foreground text-xs'>
          {unitLabel} ·{' '}
          {props.showWithRecharge
            ? t('Recharge-adjusted prices')
            : t('Standard catalog prices')}
        </p>
      </div>
      {props.compact ? (
        <CompactPriceRows
          rows={rows}
          groups={groups}
          formatPrice={formatPrice}
        />
      ) : (
        <StaticDataTable
          tableProps={{
            'aria-label': t('Official and Omnivo price comparison'),
          }}
          columns={[
            {
              id: 'group',
              header: t('Group'),
              cell: (row) => <GroupBadge group={row.group} />,
            },
            {
              id: 'type',
              header: t('Usage'),
              cell: (row) => (row.type === 'input' ? t('Input') : t('Output')),
            },
            {
              id: 'official',
              header: t('Official price'),
              cell: (row) => formatPrice(row.officialUsdPerMillion),
            },
            {
              id: 'omnivo',
              header: t('Omnivo price'),
              cell: (row) => formatPrice(row.omnivoUsdPerMillion),
            },
            {
              id: 'difference',
              header: t('Difference'),
              cell: (row) => <PriceDifference row={row} />,
            },
          ]}
          data={rows}
          getRowKey={(row) => `${row.group}:${row.type}`}
        />
      )}
      <div className='text-muted-foreground flex flex-col gap-2 text-xs leading-5'>
        <p>
          {t(
            'Cached input, batch requests and other special charges are not included in this comparison.'
          )}
        </p>
        <a
          href={reference.sourceUrl}
          target='_blank'
          rel='noopener noreferrer'
          className='text-primary inline-flex w-fit items-center gap-1 underline underline-offset-4'
        >
          {t('View official pricing source')}
          <ExternalLink aria-hidden='true' className='size-3' />
        </a>
        <p>{t('Reference checked on {{date}}', { date: checkedDate })}</p>
      </div>
    </section>
  )
}

function CompactPriceRows(props: {
  rows: GroupedOfficialPriceRow[]
  groups: string[]
  formatPrice: (usdPerMillion: number) => string
}) {
  const { t } = useTranslation()

  return (
    <div className='flex min-w-0 flex-col gap-5'>
      {props.groups.map((group) => {
        const groupRows = props.rows.filter((row) => row.group === group)
        if (groupRows.length === 0) return null

        return (
          <section
            key={group}
            aria-label={group}
            className='flex min-w-0 flex-col gap-3'
          >
            <GroupBadge group={group} />
            <dl className='flex min-w-0 flex-col gap-4'>
              {groupRows.map((row) => (
                <div key={row.type} className='min-w-0'>
                  <dt className='flex flex-wrap items-center justify-between gap-2 text-sm font-medium'>
                    <span>
                      {row.type === 'input' ? t('Input') : t('Output')}
                    </span>
                    <PriceDifference row={row} />
                  </dt>
                  <dd className='mt-2 grid min-w-0 grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-2'>
                    <div className='flex min-w-0 flex-col gap-1'>
                      <span className='text-muted-foreground text-xs'>
                        {t('Official price')}
                      </span>
                      <span className='font-mono text-sm wrap-anywhere tabular-nums'>
                        {props.formatPrice(row.officialUsdPerMillion)}
                      </span>
                    </div>
                    <ArrowRight
                      aria-hidden='true'
                      className='text-muted-foreground size-3 shrink-0'
                    />
                    <div className='flex min-w-0 flex-col gap-1'>
                      <span className='text-muted-foreground text-xs'>
                        {t('Omnivo price')}
                      </span>
                      <span className='font-mono text-sm font-medium wrap-anywhere tabular-nums'>
                        {props.formatPrice(row.omnivoUsdPerMillion)}
                      </span>
                    </div>
                  </dd>
                </div>
              ))}
            </dl>
          </section>
        )
      })}
    </div>
  )
}

function PriceDifference(props: { row: OfficialPriceRow }) {
  const { t, i18n } = useTranslation()
  const difference = props.row.savingsPercent
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  let label = t('Same price')
  if (difference > 0) {
    label =
      difference < 0.01
        ? t('Less than 0.01% lower')
        : t('Save {{percent}}%', { percent: formatNumber(difference, locale) })
  } else if (difference < 0) {
    label =
      difference > -0.01
        ? t('Less than 0.01% higher')
        : t('{{percent}}% higher', {
            percent: formatNumber(-difference, locale),
          })
  }
  return (
    <Badge
      variant={difference > 0 ? 'secondary' : 'outline'}
      className='whitespace-nowrap'
    >
      {label}
    </Badge>
  )
}
