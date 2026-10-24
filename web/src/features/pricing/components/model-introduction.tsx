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
import { useTranslation } from 'react-i18next'

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Markdown } from '@/components/ui/markdown'

import type { PricingModel } from '../types'

export function ModelIntroduction(props: { model: PricingModel }) {
  const { t } = useTranslation()
  const capabilities = new Set(props.model.capabilities ?? [])
  const workflows = []
  if (capabilities.has('reasoning')) {
    workflows.push({
      title: t('Reasoning workflows'),
      description: t('Explore tasks that benefit from multi-step reasoning.'),
    })
  }
  if (capabilities.has('tools') || capabilities.has('function_calling')) {
    workflows.push({
      title: t('Tool-assisted workflows'),
      description: t(
        'Connect model responses to tools and application actions.'
      ),
    })
  }
  if (
    capabilities.has('vision') ||
    props.model.input_modalities?.includes('image')
  ) {
    workflows.push({
      title: t('Image understanding'),
      description: t(
        'Explore requests that include images and visual context.'
      ),
    })
  }
  if (capabilities.has('embeddings')) {
    workflows.push({
      title: t('Semantic search'),
      description: t(
        'Use embeddings to explore retrieval and similarity workflows.'
      ),
    })
  }
  if (capabilities.has('structured_output') || capabilities.has('json_mode')) {
    workflows.push({
      title: t('Structured data workflows'),
      description: t(
        'Explore extraction and responses with a defined structure.'
      ),
    })
  }

  if (!props.model.description?.trim() && workflows.length === 0) return null

  return (
    <section aria-label={t('About this model')} className='flex flex-col gap-5'>
      {props.model.description?.trim() && (
        <Card>
          <CardHeader>
            <CardTitle>
              <h2>{t('About {{model}}', { model: props.model.model_name })}</h2>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Markdown>{props.model.description}</Markdown>
          </CardContent>
        </Card>
      )}
      {workflows.length > 0 && (
        <div className='flex flex-col gap-3'>
          <h2 className='text-lg font-semibold'>{t('Workflows to explore')}</h2>
          <p className='text-muted-foreground text-sm leading-6'>
            {t(
              'Start with these supported capabilities, then evaluate a request from your own workload.'
            )}
          </p>
          <div className='grid gap-3 @lg/details:grid-cols-2'>
            {workflows.map((workflow) => (
              <Card key={workflow.title} size='sm'>
                <CardHeader>
                  <CardTitle>
                    <h3>{workflow.title}</h3>
                  </CardTitle>
                </CardHeader>
                <CardContent className='text-muted-foreground leading-6'>
                  {workflow.description}
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      )}
    </section>
  )
}

export function ModelQuestions(props: { modelName: string }) {
  const { t } = useTranslation()
  return (
    <section aria-label={t('Model questions')} className='flex flex-col gap-3'>
      <h2 className='text-lg font-semibold'>
        {t('Before your first request')}
      </h2>
      <Accordion>
        <AccordionItem value='identifier'>
          <AccordionTrigger>
            {t('Which model ID should I send?')}
          </AccordionTrigger>
          <AccordionContent>
            <p>{t('Use this exact model ID in your request:')}</p>
            <code className='font-mono text-xs [overflow-wrap:anywhere]'>
              {props.modelName}
            </code>
          </AccordionContent>
        </AccordionItem>
        <AccordionItem value='pricing'>
          <AccordionTrigger>{t('How do I compare the cost?')}</AccordionTrigger>
          <AccordionContent>
            <p>
              {t(
                'Review the prices for your group and each billing item. Official-price comparisons use the same model version, currency, unit and billing conditions; discounts can differ between input, output and cache usage.'
              )}
            </p>
          </AccordionContent>
        </AccordionItem>
        <AccordionItem value='integration'>
          <AccordionTrigger>
            {t('How do I start using this model?')}
          </AccordionTrigger>
          <AccordionContent>
            <p>
              {t(
                'Get an API key, open the API examples and use the endpoint shown for this model. Start with a small request that represents your task.'
              )}
            </p>
          </AccordionContent>
        </AccordionItem>
      </Accordion>
    </section>
  )
}
