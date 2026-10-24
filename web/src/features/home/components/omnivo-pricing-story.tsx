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
import { ArrowUpRight, BadgeCheck, Scale, Wallet } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Button } from '@/components/ui/button'

import { useHomeCatalogAccess } from '../hooks/use-home-catalog-access'

export function OmnivoPricingStory() {
  const { t } = useTranslation()
  const access = useHomeCatalogAccess()
  const comparisons = [
    {
      icon: Scale,
      title: t('Compare the same usage'),
      description: t(
        'Input, output, and cache usage have their own prices. Compare the same model, unit, and billing conditions.'
      ),
    },
    {
      icon: BadgeCheck,
      title: t('See where savings come from'),
      description: t(
        'When an official reference is available, compare the Omnivo rate with the provider’s standard API price.'
      ),
    },
    {
      icon: Wallet,
      title: t('Choose with the current price'),
      description: t(
        'Check the model page for current rates and any billing tiers before you build.'
      ),
    },
  ]
  return (
    <section
      aria-labelledby='omnivo-pricing-story-title'
      className='mx-auto max-w-7xl px-6 md:px-10'
    >
      <div className='bg-muted/40 border-border grid gap-10 rounded-3xl border px-7 py-10 md:px-12 md:py-14 lg:grid-cols-[1fr_1.1fr] lg:gap-16'>
        <div>
          <p className='text-muted-foreground text-xs font-medium tracking-[0.18em] uppercase'>
            {t('A clearer view of your AI costs')}
          </p>
          <h2
            id='omnivo-pricing-story-title'
            className='mt-4 max-w-md text-3xl leading-tight font-semibold tracking-tight md:text-4xl'
          >
            {t('Know the price before your first request.')}
          </h2>
          <p className='text-muted-foreground mt-5 max-w-md text-sm leading-7'>
            {t(
              'Compare models by what you need and what you pay. Clear units and official references help you make an informed choice.'
            )}
          </p>
          {access.ready && access.enabled && (
            <Button
              role='link'
              variant='outline'
              className='mt-7 rounded-full px-5'
              render={<Link to='/pricing' search={{ tokenUnit: 'M' }} />}
            >
              {t('Compare model prices')}
              <ArrowUpRight aria-hidden='true' />
            </Button>
          )}
        </div>
        <dl className='flex flex-col gap-7'>
          {comparisons.map((comparison) => (
            <div key={comparison.title} className='flex gap-4'>
              <comparison.icon
                aria-hidden='true'
                className='text-primary mt-0.5 size-5 shrink-0'
              />
              <div>
                <dt className='font-medium'>{comparison.title}</dt>
                <dd className='text-muted-foreground mt-2 text-sm leading-7'>
                  {comparison.description}
                </dd>
              </div>
            </div>
          ))}
        </dl>
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
      className='mx-auto grid max-w-7xl gap-8 px-6 pt-20 md:px-10 lg:grid-cols-[0.8fr_1.2fr] lg:gap-16 lg:pt-24'
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
            <AccordionTrigger className='py-5'>
              {question.question}
            </AccordionTrigger>
            <AccordionContent className='text-muted-foreground pb-5 leading-7'>
              {question.answer}
            </AccordionContent>
          </AccordionItem>
        ))}
      </Accordion>
    </section>
  )
}
