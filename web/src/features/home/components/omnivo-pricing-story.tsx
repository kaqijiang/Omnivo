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

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import { useHomeCatalogAccess } from '../hooks/use-home-catalog-access'

export function OmnivoPricingStory() {
  const { t } = useTranslation()
  const access = useHomeCatalogAccess()

  return (
    <section
      aria-labelledby='omnivo-pricing-story-title'
      className='mx-auto max-w-6xl px-5 sm:px-8'
    >
      <div className='omnivo-inverse bg-background text-foreground border-border grid gap-10 rounded-2xl border px-6 py-9 md:p-10 lg:grid-cols-[1fr_1.05fr] lg:items-center lg:gap-14 lg:p-12'>
        <div className='min-w-0'>
          <p className='text-primary text-xs font-medium tracking-[0.16em] uppercase'>
            {t('More room to build')}
          </p>
          <h2
            id='omnivo-pricing-story-title'
            className='mt-4 max-w-md scroll-mt-24 text-3xl leading-[1.15] font-semibold tracking-tight text-balance md:text-4xl'
          >
            {t('Make your AI budget go further.')}
          </h2>
          <p className='text-muted-foreground mt-5 max-w-sm text-sm leading-7'>
            {t(
              'Compare model prices with official reference rates, so you can choose where your budget goes.'
            )}
          </p>
          {access.ready && access.enabled && (
            <Button
              role='link'
              size='lg'
              className='mt-7 h-10 px-4'
              render={<Link to='/pricing' search={{ tokenUnit: 'M' }} />}
            >
              {t('Compare model prices')}
              <ArrowUpRight aria-hidden='true' />
            </Button>
          )}
        </div>
        <figure
          aria-labelledby='omnivo-savings-example-title'
          aria-describedby='omnivo-savings-example-note'
          className='bg-card/60 border-border min-w-0 rounded-xl border p-5 md:p-6'
        >
          <Badge id='omnivo-savings-example-title' variant='secondary'>
            {t('20% savings example')}
          </Badge>
          <div className='mt-7 flex flex-col gap-6'>
            <div>
              <div className='flex items-baseline justify-between gap-4'>
                <p className='text-muted-foreground text-sm'>
                  {t('Provider standard price')}
                </p>
                <span className='font-mono text-sm tabular-nums'>100%</span>
              </div>
              <div
                aria-hidden='true'
                className='bg-foreground/25 mt-3 h-3 w-full rounded-sm'
              />
            </div>
            <div>
              <div className='flex items-baseline justify-between gap-4'>
                <p className='text-sm font-medium'>
                  {t('Omnivo in this example')}
                </p>
                <span className='text-primary font-mono text-3xl font-semibold tracking-tight tabular-nums'>
                  80%
                </span>
              </div>
              <div aria-hidden='true' className='mt-3 flex h-3'>
                <div className='bg-primary w-4/5 rounded-l-sm' />
                <div className='border-primary/50 w-1/5 rounded-r-sm border border-l-0 border-dashed' />
              </div>
            </div>
          </div>
          <p className='text-primary mt-4 text-right text-xs font-medium'>
            {t('20% left for your next idea')}
          </p>
          <figcaption
            id='omnivo-savings-example-note'
            className='text-muted-foreground mt-6 text-xs leading-6'
          >
            {t(
              'Example only. Compare each model’s input and output rates with the official reference to confirm its actual discount.'
            )}
          </figcaption>
        </figure>
      </div>
    </section>
  )
}

export function OmnivoFaq() {
  const { t } = useTranslation()
  const questions = [
    {
      question: t('Can I keep using my existing SDK?'),
      answer: t(
        'Use the supported protocol shown on the model page. For OpenAI-compatible models, update the API base URL, API key, and model ID in your existing client.'
      ),
    },
    {
      question: t('How should I read a discount?'),
      answer: t(
        'A 20% saving means paying 80% of the provider’s standard API price for the same usage. Input and output discounts can differ; use the comparison on each model page.'
      ),
    },
    {
      question: t('Do all models have the same price or discount?'),
      answer: t(
        'Rates vary by model, usage type, and account group. The current model page shows the applicable prices; a reference price is not a promise of the same discount across every model.'
      ),
    },
    {
      question: t('Where can I find newly available models?'),
      answer: t(
        'Browse the model catalog for the models available to your account. Each model page brings its description, pricing, and integration examples together.'
      ),
    },
  ]

  return (
    <section
      aria-labelledby='omnivo-faq-title'
      className='mx-auto grid max-w-6xl gap-7 px-5 pt-16 sm:px-8 lg:grid-cols-[0.8fr_1.2fr] lg:gap-14 lg:pt-20'
    >
      <div>
        <p className='text-muted-foreground text-xs font-medium tracking-[0.18em] uppercase'>
          {t('Before you start')}
        </p>
        <h2
          id='omnivo-faq-title'
          className='mt-4 text-3xl leading-tight font-semibold tracking-tight md:text-4xl'
        >
          {t('A few things worth knowing.')}
        </h2>
      </div>
      <Accordion>
        {questions.map((question) => (
          <AccordionItem key={question.question} value={question.question}>
            <AccordionTrigger className='gap-5 py-4'>
              {question.question}
            </AccordionTrigger>
            <AccordionContent className='text-muted-foreground pb-4 leading-7'>
              {question.answer}
            </AccordionContent>
          </AccordionItem>
        ))}
      </Accordion>
    </section>
  )
}
