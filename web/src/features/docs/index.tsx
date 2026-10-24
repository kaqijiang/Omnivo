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
import { BookOpen, KeyRound, Radio, Terminal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CodeBlockFrame } from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

import { GettingStarted } from './components/getting-started'
import { RequestExample } from './components/request-example'
import { Troubleshooting } from './components/troubleshooting'
import { buildRequestExamples } from './request-examples'

export function DocsContent() {
  const { t } = useTranslation()
  const baseUrl = new URL('/v1', window.location.origin).toString()
  const examples = buildRequestExamples(baseUrl)
  const sections = [
    { id: 'get-started', label: t('Before your first request') },
    { id: 'connect', label: t('Make a request') },
    { id: 'streaming', label: t('Stream a response') },
    { id: 'troubleshooting', label: t('Troubleshooting') },
  ]

  return (
    <main className='mx-auto w-full max-w-6xl px-5 pt-28 pb-20 sm:px-8 lg:pt-36'>
      <div className='mb-12 max-w-3xl'>
        <p className='text-primary mb-5 flex items-center gap-2 text-xs font-semibold tracking-[0.18em] uppercase'>
          <BookOpen aria-hidden='true' className='size-4' />{' '}
          {t('Developer guide')}
        </p>
        <h1 className='text-4xl leading-[1.08] font-semibold tracking-tight sm:text-5xl lg:text-6xl'>
          {t('Your first request, made simple.')}
        </h1>
        <p className='text-muted-foreground mt-6 max-w-xl text-base leading-8 sm:text-lg'>
          {t(
            'Connect your application to Omnivo with a familiar API and the tools you already use.'
          )}
        </p>
      </div>
      <div className='grid min-w-0 gap-10 lg:grid-cols-[210px_minmax(0,1fr)] lg:gap-16'>
        <aside>
          <nav
            aria-label={t('On this page')}
            className='flex flex-wrap gap-3 lg:sticky lg:top-24 lg:flex-col lg:gap-1'
          >
            <p className='text-muted-foreground mb-3 hidden text-xs font-medium tracking-wider uppercase lg:block'>
              {t('On this page')}
            </p>
            {sections.map((section) => (
              <a
                key={section.id}
                href={`#${section.id}`}
                className='text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-ring rounded-lg px-3 py-2 text-sm transition-colors outline-none focus-visible:ring-2'
              >
                {section.label}
              </a>
            ))}
          </nav>
        </aside>
        <div className='flex min-w-0 flex-col gap-16'>
          <GettingStarted />
          <section id='connect' className='min-w-0 scroll-mt-24'>
            <h2 className='mb-3 flex items-center gap-3 text-2xl font-semibold tracking-tight'>
              <Terminal aria-hidden='true' className='text-primary size-5' />
              {t('Make a request')}
            </h2>
            <p className='text-muted-foreground mb-5 text-sm leading-7'>
              {t(
                'Use this deployment’s API address below. If your operator provides a separate gateway address, use that address instead.'
              )}
            </p>
            <div className='bg-muted/50 mb-6 flex min-w-0 items-center justify-between gap-3 rounded-xl border px-4 py-3'>
              <div className='min-w-0'>
                <p className='text-muted-foreground mb-1 text-xs'>
                  {t('Base URL')}
                </p>
                <code className='block text-sm break-all'>{baseUrl}</code>
              </div>
              <CopyButton value={baseUrl} aria-label={t('Copy API base URL')} />
            </div>
            <RequestExample baseUrl={baseUrl} />
            <Alert className='mt-5'>
              <KeyRound aria-hidden='true' />
              <AlertTitle>{t('Use your Omnivo key')}</AlertTitle>
              <AlertDescription>
                {t(
                  'These examples run on your server or in a terminal. Use a key issued by this platform, not a provider key. Model availability depends on your configured channels.'
                )}
              </AlertDescription>
            </Alert>
          </section>
          <section id='streaming' className='min-w-0 scroll-mt-24'>
            <h2 className='mb-3 flex items-center gap-3 text-2xl font-semibold tracking-tight'>
              <Radio aria-hidden='true' className='text-primary size-5' />
              {t('Stream a response')}
            </h2>
            <p className='text-muted-foreground mb-5 text-sm leading-7'>
              {t(
                'For a model that supports streaming, set stream to true. Read the server-sent events as they arrive; the final [DONE] event ends the stream.'
              )}
            </p>
            <CodeBlockFrame
              title={t('Streaming request')}
              showToolbar
              endActions={
                <CopyButton
                  value={examples.stream}
                  aria-label={t('Copy streaming request')}
                />
              }
            >
              <pre className='p-5 text-xs leading-7 sm:text-sm'>
                <code>{examples.stream}</code>
              </pre>
            </CodeBlockFrame>
          </section>
          <Troubleshooting />
        </div>
      </div>
    </main>
  )
}

export function Docs() {
  return (
    <PublicLayout showMainContainer={false}>
      <DocsContent />
      <Footer />
    </PublicLayout>
  )
}
