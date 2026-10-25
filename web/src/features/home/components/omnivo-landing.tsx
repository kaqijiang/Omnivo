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
import { ArrowRight, ArrowUpRight, BadgePercent } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import { OmnivoModelDiscovery } from './omnivo-model-discovery'
import { OmnivoModelSearch } from './omnivo-model-search'
import { OmnivoFaq, OmnivoPricingStory } from './omnivo-pricing-story'
import { OmnivoQuickstart } from './omnivo-quickstart'

interface OmnivoLandingProps {
  isAuthenticated: boolean
}

export function OmnivoLanding(props: OmnivoLandingProps) {
  const { t } = useTranslation()
  const startRoute = props.isAuthenticated ? '/keys' : '/sign-up'
  const startLabel = props.isAuthenticated
    ? t('Manage API keys')
    : t('Start building')

  return (
    <main>
      <section
        aria-labelledby='omnivo-hero-title'
        className='omnivo-home-hero relative isolate px-5 pt-28 pb-12 sm:px-8 sm:pt-36 sm:pb-16'
      >
        <div className='mx-auto flex max-w-5xl flex-col items-center text-center'>
          <Link
            to='/'
            hash='omnivo-pricing-story-title'
            className='rounded-full outline-offset-4'
          >
            <Badge variant='outline' className='h-auto gap-2 px-3 py-1.5'>
              <BadgePercent aria-hidden='true' className='size-3.5' />
              {t('One connection. Better value.')}
              <ArrowUpRight aria-hidden='true' className='size-3' />
            </Badge>
          </Link>
          <h1
            id='omnivo-hero-title'
            className='mt-7 text-[clamp(3rem,7vw,5.75rem)] leading-[1.06] font-semibold tracking-[-0.065em] text-balance'
          >
            {t('Many models.')}{' '}
            <span className='text-primary block sm:inline'>
              {t('One API.')}
            </span>
          </h1>
          <p className='text-muted-foreground mt-6 max-w-2xl text-base leading-7 text-pretty sm:text-lg sm:leading-8'>
            {t(
              'Explore AI models in one place. Keep your favorite tools, compare prices, and make more of your AI budget.'
            )}
          </p>
          <OmnivoModelSearch />
          <div className='mt-5 flex flex-wrap justify-center gap-3'>
            <Button
              role='link'
              size='lg'
              className='h-11 rounded-lg px-5'
              render={<Link to={startRoute} />}
            >
              {startLabel}
              <ArrowRight aria-hidden='true' />
            </Button>
            <Button
              role='link'
              variant='outline'
              size='lg'
              className='h-11 rounded-lg px-5'
              render={<Link to='/pricing' />}
            >
              {t('Explore models')}
            </Button>
          </div>
          <div
            aria-label={t('Supported API protocols')}
            className='text-muted-foreground mt-9 flex max-w-full flex-col items-center gap-3 text-xs sm:mt-10 sm:flex-row sm:gap-5'
          >
            <span>{t('Fits your existing stack')}</span>
            <ul className='flex flex-wrap justify-center gap-x-5 gap-y-2 font-mono text-[11px]'>
              <li>Chat Completions</li>
              <li>Responses</li>
              <li>Messages</li>
              <li>Gemini</li>
            </ul>
          </div>
        </div>
      </section>

      <OmnivoModelDiscovery isAuthenticated={props.isAuthenticated} />
      <OmnivoPricingStory />
      <OmnivoQuickstart />
      <OmnivoFaq />

      <section
        aria-labelledby='omnivo-cta-title'
        className='mx-auto max-w-6xl px-5 py-16 sm:px-8 sm:py-20'
      >
        <div className='border-border flex flex-col items-start justify-between gap-7 border-t pt-10 sm:flex-row sm:items-center'>
          <div>
            <h2
              id='omnivo-cta-title'
              className='text-2xl font-semibold tracking-tight sm:text-3xl'
            >
              {t('Your next idea starts with one API.')}
            </h2>
            <p className='text-muted-foreground mt-3 text-sm leading-6'>
              {t('Choose your model. Find your price. Start building.')}
            </p>
          </div>
          <Button
            role='link'
            size='lg'
            className='h-11 shrink-0 rounded-lg px-5'
            render={<Link to={startRoute} />}
          >
            {startLabel}
            <ArrowRight aria-hidden='true' />
          </Button>
        </div>
      </section>
    </main>
  )
}
