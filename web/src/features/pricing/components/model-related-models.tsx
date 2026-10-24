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
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import type { PricingModel } from '../types'

/** Only suggest models from the viewer's already-visible catalog. */
export function ModelRelatedModels(props: {
  model: PricingModel
  models: PricingModel[]
}) {
  const { t } = useTranslation()
  const related = props.models
    .filter((candidate) => {
      if (candidate.model_name === props.model.model_name) return false
      const sameVendor = Boolean(
        props.model.vendor_id && candidate.vendor_id === props.model.vendor_id
      )
      const sharedCapability = candidate.capabilities?.some((capability) =>
        props.model.capabilities?.includes(capability)
      )
      return sameVendor || sharedCapability
    })
    .slice(0, 3)

  if (related.length === 0) return null

  return (
    <section
      aria-label={t('Keep exploring models')}
      className='mt-10 flex flex-col gap-4'
    >
      <h2 className='text-xl font-semibold'>{t('Keep exploring models')}</h2>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Compare other available models from this provider or with shared capabilities.'
        )}
      </p>
      <div className='grid gap-4 md:grid-cols-3'>
        {related.map((model) => (
          <Card key={model.model_name}>
            <CardHeader>
              <CardTitle>
                <h3 className='font-mono text-sm [overflow-wrap:anywhere]'>
                  {model.model_name}
                </h3>
              </CardTitle>
            </CardHeader>
            <CardContent className='text-muted-foreground flex-1 text-sm leading-6'>
              {model.vendor_name && <p>{model.vendor_name}</p>}
              {model.description && (
                <p className='mt-2 line-clamp-3 [overflow-wrap:anywhere]'>
                  {model.description}
                </p>
              )}
            </CardContent>
            <CardFooter>
              <Button
                role='link'
                variant='ghost'
                size='sm'
                render={
                  <Link
                    to='/pricing/$modelId'
                    params={{ modelId: model.model_name }}
                  />
                }
              >
                {t('View model guide')}
                <ArrowUpRight aria-hidden='true' />
              </Button>
            </CardFooter>
          </Card>
        ))}
      </div>
    </section>
  )
}
