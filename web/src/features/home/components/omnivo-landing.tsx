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
import {
  ArrowRight,
  AudioLines,
  Braces,
  Layers3,
  SlidersHorizontal,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'

import { OmnivoCodeSample } from './omnivo-code-sample'
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
  const capabilities = [
    {
      icon: Braces,
      title: t('A familiar interface'),
      description: t(
        'Use OpenAI-compatible clients and connect through a single API base URL.'
      ),
    },
    {
      icon: Layers3,
      title: t('Room to explore'),
      description: t(
        'Discover models for different tasks, with capabilities and prices in one catalog.'
      ),
    },
    {
      icon: SlidersHorizontal,
      title: t('Clarity at every step'),
      description: t(
        'Manage your API keys, review request logs, and keep track of your usage.'
      ),
    },
  ]

  return (
    <main>
      <section
        aria-labelledby='omnivo-hero-title'
        className='mx-auto max-w-7xl px-6 pt-32 pb-20 md:px-10 md:pt-40 lg:pt-44 lg:pb-28'
      >
        <div className='grid items-center gap-14 lg:grid-cols-[1.1fr_1fr] lg:gap-16'>
          <div>
            <Badge
              variant='outline'
              className='mb-7 h-auto px-3 py-1.5 text-[11px] tracking-wide'
            >
              {t('One connection. More possibilities.')}
            </Badge>
            <h1
              id='omnivo-hero-title'
              className='text-[clamp(3.1rem,6.7vw,6rem)] leading-[1.04] font-semibold tracking-[-0.065em]'
            >
              {t('Many models.')}
              <br />
              <span className='text-primary'>{t('One API.')}</span>
            </h1>
            <p className='text-muted-foreground mt-7 max-w-lg text-base leading-8 md:text-lg'>
              {t(
                'A simpler way to build with AI. Explore models, connect your app, and manage your usage with Omnivo.'
              )}
            </p>
            <div className='mt-9 flex flex-wrap gap-3'>
              <Button
                role='link'
                size='lg'
                className='h-12 rounded-full px-6'
                render={<Link to={startRoute} />}
              >
                {startLabel}
                <ArrowRight aria-hidden='true' />
              </Button>
              <Button
                role='link'
                variant='outline'
                size='lg'
                className='h-12 rounded-full px-6'
                render={<Link to='/pricing' />}
              >
                {t('Explore models')}
              </Button>
            </div>
            <p className='text-muted-foreground mt-6 flex items-center gap-2 text-xs'>
              <Braces aria-hidden='true' className='size-4' />
              {t('Built for the tools you already use.')}
            </p>
          </div>
          <OmnivoCodeSample />
        </div>
      </section>

      <section
        aria-label={t('Supported API protocols')}
        className='bg-muted/40 border-border border-y'
      >
        <div className='mx-auto flex max-w-7xl flex-col gap-6 px-6 py-7 md:flex-row md:items-center md:justify-between md:px-10'>
          <p className='text-muted-foreground text-xs font-medium tracking-wide'>
            {t('Different protocols. A shared starting point.')}
          </p>
          <ul className='text-foreground flex flex-wrap items-center gap-x-8 gap-y-4 font-mono text-xs md:gap-x-10'>
            <li>Chat Completions</li>
            <li>Responses</li>
            <li>Messages</li>
            <li>Gemini</li>
          </ul>
        </div>
      </section>

      <section
        aria-labelledby='omnivo-capabilities-title'
        className='mx-auto max-w-7xl px-6 pt-20 md:px-10 lg:pt-28'
      >
        <div className='flex flex-col justify-between gap-5 md:flex-row md:items-end'>
          <div className='max-w-xl'>
            <p className='text-muted-foreground text-xs font-medium tracking-[0.18em] uppercase'>
              {t('Less setup. More creating.')}
            </p>
            <h2
              id='omnivo-capabilities-title'
              className='mt-4 text-3xl leading-tight font-semibold tracking-tight md:text-4xl'
            >
              {t('One place for your AI workflow.')}
            </h2>
          </div>
          <p className='text-muted-foreground max-w-sm text-sm leading-7'>
            {t(
              'From the first experiment to your next application, keep the essentials connected.'
            )}
          </p>
        </div>
        <div className='mt-10 grid gap-5 md:grid-cols-3'>
          {capabilities.map((capability) => (
            <Card key={capability.title} className='rounded-2xl shadow-none'>
              <CardHeader className='gap-4 p-7'>
                <capability.icon
                  aria-hidden='true'
                  className='text-primary mb-3 size-7'
                  strokeWidth={1.4}
                />
                <CardTitle className='text-lg'>
                  <h3>{capability.title}</h3>
                </CardTitle>
                <CardDescription className='text-sm leading-7'>
                  {capability.description}
                </CardDescription>
              </CardHeader>
            </Card>
          ))}
        </div>
        <p className='text-muted-foreground mt-5 flex items-start gap-2 text-xs leading-5'>
          <AudioLines aria-hidden='true' className='mt-0.5 size-4 shrink-0' />
          {t(
            'Model availability and supported features depend on the channels enabled for your account.'
          )}
        </p>
      </section>

      <OmnivoQuickstart />

      <section
        aria-labelledby='omnivo-cta-title'
        className='mx-auto max-w-7xl px-6 pb-20 md:px-10 lg:pb-28'
      >
        <div className='omnivo-inverse bg-background text-foreground rounded-3xl px-7 py-10 md:px-12 md:py-14'>
          <div className='flex flex-col items-start justify-between gap-8 md:flex-row md:items-center'>
            <div>
              <p className='text-primary mb-4 font-mono text-xs tracking-[0.18em] uppercase'>
                Omnivo
              </p>
              <h2
                id='omnivo-cta-title'
                className='max-w-xl text-3xl leading-tight font-semibold tracking-tight md:text-4xl'
              >
                {t('Make room for your next idea.')}
              </h2>
              <p className='text-muted-foreground mt-4 max-w-lg text-sm leading-7'>
                {t('Choose a model. Make a connection. Build something yours.')}
              </p>
            </div>
            <Button
              role='link'
              size='lg'
              className='h-12 rounded-full px-6'
              render={<Link to={startRoute} />}
            >
              {startLabel}
              <ArrowRight aria-hidden='true' />
            </Button>
          </div>
          <Separator className='my-8' />
          <Link
            to='/docs'
            className='text-muted-foreground hover:text-foreground inline-flex items-center gap-2 text-sm underline-offset-4 hover:underline'
          >
            {t('Start with the documentation')}
            <ArrowRight aria-hidden='true' className='size-4' />
          </Link>
        </div>
      </section>
    </main>
  )
}
