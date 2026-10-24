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
import { Link, useNavigate } from '@tanstack/react-router'
import { ArrowUpRight, LockKeyhole } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { ModelCard } from '@/features/pricing/components/model-card'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'

import { useHomeCatalogAccess } from '../hooks/use-home-catalog-access'

function AvailableModels() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { models, isLoading, error, refetch, priceRate, usdExchangeRate } =
    usePricingData()

  if (isLoading) {
    return <LoadingState message={t('Loading model catalog...')} />
  }

  if (error) {
    return (
      <ErrorState
        title={t('Unable to load the model catalog')}
        description={t('Please try again to see current models and prices.')}
        onRetry={() => void refetch()}
      />
    )
  }

  if (models.length === 0) {
    return (
      <EmptyState
        title={t('No models available yet')}
        description={t(
          'Explore the quickstart while the model catalog is being updated.'
        )}
        bordered
        action={
          <Button variant='outline' role='link' render={<Link to='/docs' />}>
            {t('View integration guide')}
          </Button>
        }
      />
    )
  }

  return (
    <div className='grid min-w-0 grid-cols-1 gap-5 md:grid-cols-2 xl:grid-cols-3'>
      {models.slice(0, 3).map((model) => (
        <ModelCard
          key={model.model_name}
          model={model}
          priceRate={priceRate}
          usdExchangeRate={usdExchangeRate}
          tokenUnit='M'
          onClick={(modelId) =>
            void navigate({
              to: '/pricing/$modelId',
              params: { modelId },
              search: { tokenUnit: 'M' },
            })
          }
        />
      ))}
    </div>
  )
}

export function OmnivoModelDiscovery(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()
  const access = useHomeCatalogAccess()

  if (access.ready && !access.enabled) return null

  let content
  if (access.loading) {
    content = <LoadingState message={t('Loading model catalog...')} />
  } else if (access.error || !access.ready) {
    content = (
      <ErrorState
        title={t('Unable to load the model catalog')}
        description={t('Please try again to see current models and prices.')}
        onRetry={() => void access.refetch()}
      />
    )
  } else if (access.requireAuth && !props.isAuthenticated) {
    content = (
      <EmptyState
        icon={LockKeyhole}
        title={t('Your model catalog awaits')}
        description={t(
          'Sign in to explore models and prices for your account.'
        )}
        bordered
        action={
          <Button
            role='link'
            render={<Link to='/sign-in' search={{ redirect: '/pricing' }} />}
          >
            {t('Sign in to explore')}
          </Button>
        }
      />
    )
  } else {
    // Mount the data consumer only after access is known. A disabled query can
    // still return cached data, so `enabled: false` alone is insufficient here.
    content = <AvailableModels />
  }

  return (
    <section
      aria-labelledby='omnivo-discovery-title'
      className='mx-auto max-w-7xl px-6 py-20 md:px-10 lg:py-24'
    >
      <div className='mb-9 flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between'>
        <div className='max-w-2xl'>
          <p className='text-muted-foreground text-xs font-medium tracking-[0.18em] uppercase'>
            {t('Explore the possibilities')}
          </p>
          <h2
            id='omnivo-discovery-title'
            className='mt-4 text-3xl leading-tight font-semibold tracking-tight md:text-4xl'
          >
            {t('Find the right model for your next idea.')}
          </h2>
          <p className='text-muted-foreground mt-4 max-w-xl text-sm leading-7'>
            {t(
              'Explore available models, compare their prices, and open a model guide before your first request.'
            )}
          </p>
        </div>
        {access.ready && access.enabled && (
          <Button
            role='link'
            variant='outline'
            className='self-start rounded-full px-5 sm:self-auto'
            render={<Link to='/pricing' search={{ tokenUnit: 'M' }} />}
          >
            {t('Browse all models')}
            <ArrowUpRight aria-hidden='true' />
          </Button>
        )}
      </div>
      {content}
    </section>
  )
}
