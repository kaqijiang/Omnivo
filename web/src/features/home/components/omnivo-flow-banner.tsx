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
import { ArrowUpRight, Code2, Cpu, Pause, Play, Radio } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useMediaQuery } from '@/hooks/use-media-query'

import { OmnivoFlowScene } from './omnivo-flow-scene'

import '@/styles/omnivo-flow.css'

export function OmnivoFlowBanner() {
  const { t } = useTranslation()
  const titleId = useId()
  const sceneRef = useRef<HTMLElement>(null)
  const [paused, setPaused] = useState(false)
  const [visible, setVisible] = useState(false)
  const [foreground, setForeground] = useState(() => !document.hidden)
  const reducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
  const compact = useMediaQuery('(max-width: 639px)')

  useEffect(() => {
    const element = sceneRef.current
    if (!element || reducedMotion) return
    const observer =
      typeof IntersectionObserver === 'undefined'
        ? null
        : new IntersectionObserver(
            ([entry]) => setVisible(entry.isIntersecting),
            { threshold: 0.1 }
          )
    if (observer) observer.observe(element)
    else setVisible(true)
    const onVisibility = () => setForeground(!document.hidden)
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      observer?.disconnect()
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [reducedMotion])

  const running = visible && foreground && !paused && !reducedMotion
  const buttonLabel = paused
    ? t('Play flow animation')
    : t('Pause flow animation')

  return (
    <section
      className='mx-auto max-w-6xl px-5 pb-10 sm:px-8 sm:pb-16'
      aria-labelledby={titleId}
    >
      <figure
        ref={sceneRef}
        className='omnivo-flow'
        data-motion={running ? 'running' : 'paused'}
      >
        <div className='relative z-10 flex items-start justify-between gap-4 px-5 pt-6 sm:px-8 sm:pt-8'>
          <div className='flex min-w-0 flex-col gap-2'>
            <p className='text-muted-foreground text-[10px] font-medium tracking-[0.16em] uppercase'>
              {t('The Omnivo flow')}
            </p>
            <h2
              id={titleId}
              className='text-xl font-medium tracking-tight sm:text-2xl'
            >
              {t('Follow the flow.')}
            </h2>
          </div>
          <Button
            variant='ghost'
            size='sm'
            aria-label={
              reducedMotion ? t('Reduced motion is enabled') : buttonLabel
            }
            title={reducedMotion ? t('Reduced motion is enabled') : buttonLabel}
            disabled={reducedMotion}
            onClick={() => setPaused((value) => !value)}
          >
            {paused ? (
              <Play aria-hidden='true' />
            ) : (
              <Pause aria-hidden='true' />
            )}
            <span className='hidden sm:inline'>
              {paused ? t('Play') : t('Pause')}
            </span>
          </Button>
        </div>
        <p className='text-muted-foreground relative z-10 mt-2 max-w-xl px-5 text-sm leading-6 sm:px-8'>
          {t('From your app to your chosen model, and back.')}
        </p>

        <div className='omnivo-flow-stage'>
          <OmnivoFlowScene compact={compact} />
          <span className='omnivo-flow-wordmark' aria-hidden='true'>
            Omnivo
          </span>
          <div className='omnivo-flow-protocol' aria-hidden='true'>
            POST /v1/chat/completions <ArrowUpRight className='size-3' />
          </div>
        </div>

        <figcaption className='relative z-10 px-5 pb-5 sm:px-8 sm:pb-6'>
          <ol className='omnivo-flow-steps'>
            <li>
              <Code2 aria-hidden='true' />
              <span>{t('Your app')}</span>
            </li>
            <li>
              <img src='/omnivo-mark.svg' alt='' />
              <span>{t('One endpoint')}</span>
            </li>
            <li>
              <Cpu aria-hidden='true' />
              <span>{t('Selected model')}</span>
            </li>
            <li>
              <Radio aria-hidden='true' />
              <span>{t('Streaming response')}</span>
            </li>
          </ol>
          <div className='text-muted-foreground mt-5 flex flex-wrap items-center justify-between gap-x-5 gap-y-2 text-[10px] sm:text-[11px]'>
            <span>{t('API flow illustration')}</span>
            <div className='flex items-center gap-4'>
              <span className='omnivo-flow-legend'>
                <i className='omnivo-flow-request-key' />
                {t('Request')}
              </span>
              <span className='omnivo-flow-legend'>
                <i className='omnivo-flow-response-key' />
                {t('Response')}
              </span>
            </div>
          </div>
        </figcaption>
      </figure>
    </section>
  )
}
