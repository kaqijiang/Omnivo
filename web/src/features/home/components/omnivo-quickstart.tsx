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

import { OmnivoCodeSample } from './omnivo-code-sample'

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
      className='mx-auto grid max-w-6xl items-center gap-10 px-5 py-16 sm:px-8 sm:py-20 lg:grid-cols-2 lg:gap-16'
    >
      <div>
        <p className='text-primary text-xs font-medium tracking-wide'>
          {t('From idea to API')}
        </p>
        <h2
          id='omnivo-start-title'
          className='mt-3 text-3xl leading-tight font-semibold tracking-tight sm:text-4xl'
        >
          {t('Your tools. Your workflow. One connection.')}
        </h2>
        <p className='text-muted-foreground mt-4 max-w-md text-sm leading-7'>
          {t(
            'Use OpenAI-compatible clients and connect through a single API base URL.'
          )}
        </p>
        <ol className='mt-7 flex flex-col gap-5'>
          {steps.map((step) => (
            <li key={step.number} className='flex gap-4'>
              <span
                aria-hidden='true'
                className='text-muted-foreground pt-0.5 font-mono text-xs'
              >
                {step.number}
              </span>
              <div>
                <h3 className='text-sm font-semibold'>{step.title}</h3>
                <p className='text-muted-foreground mt-1 max-w-sm text-sm leading-6'>
                  {step.description}
                </p>
              </div>
            </li>
          ))}
        </ol>
        <Button
          role='link'
          variant='outline'
          className='mt-7 rounded-lg px-4'
          render={<Link to='/docs' />}
        >
          {t('Read the quickstart')}
          <ArrowUpRight aria-hidden='true' />
        </Button>
      </div>
      <OmnivoCodeSample />
    </section>
  )
}
