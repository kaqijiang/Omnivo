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
import { ArrowUpRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export function OmnivoQuickstart() {
  const { t } = useTranslation()

  const steps = [
    {
      number: '01',
      title: t('Make it yours'),
      description: t(
        'Create your account and generate an API key in your console.'
      ),
    },
    {
      number: '02',
      title: t('Find your model'),
      description: t(
        'Explore the model catalog and compare capabilities and pricing.'
      ),
    },
    {
      number: '03',
      title: t('Bring your idea to life'),
      description: t(
        'Set your base URL, send a request, and follow usage in one place.'
      ),
    },
  ]

  return (
    <section
      aria-labelledby='omnivo-start-title'
      className='mx-auto max-w-7xl px-6 py-20 md:px-10 lg:py-28'
    >
      <div className='flex flex-col items-start justify-between gap-6 sm:flex-row sm:items-end'>
        <div className='max-w-xl'>
          <p className='text-muted-foreground text-xs font-medium tracking-[0.18em] uppercase'>
            {t('From idea to API')}
          </p>
          <h2
            id='omnivo-start-title'
            className='mt-4 text-3xl leading-tight font-semibold tracking-tight md:text-4xl'
          >
            {t('Your next build starts here.')}
          </h2>
        </div>
        <Button
          role='link'
          variant='outline'
          size='lg'
          className='h-11 rounded-full px-5'
          render={<Link to='/docs' />}
        >
          {t('Read the quickstart')}
          <ArrowUpRight aria-hidden='true' />
        </Button>
      </div>
      <ol className='mt-12 grid gap-10 md:grid-cols-3 md:gap-8'>
        {steps.map((step) => (
          <li key={step.number} className='border-border border-t pt-6'>
            <span aria-hidden='true' className='text-primary font-mono text-sm'>
              {step.number}
            </span>
            <h3 className='mt-6 text-xl font-semibold tracking-tight'>
              {step.title}
            </h3>
            <p className='text-muted-foreground mt-3 max-w-sm text-sm leading-7'>
              {step.description}
            </p>
          </li>
        ))}
      </ol>
    </section>
  )
}
