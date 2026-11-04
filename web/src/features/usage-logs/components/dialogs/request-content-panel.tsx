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
import { QueryClient, useInfiniteQuery } from '@tanstack/react-query'
import { useEffect, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  contentSessionKey,
  useContentSession,
} from '../../hooks/use-content-session'
import {
  CONTENT_CHUNK_BYTES,
  CONTENT_DISPLAY_BYTES,
  CONTENT_PAGE_SIZE,
  contentBytes,
  contentPageSchema,
  decodeContent,
  type ContentPage,
} from '../../lib/request-content'
import { DetailSection } from './log-detail-layout'

type ReaderProps = {
  requestId: string
  sessionKey: string
  onError: () => void
}

function RequestContentReader(props: ReaderProps) {
  const { t, i18n } = useTranslation()
  const reportError = props.onError
  // Plaintext lives only in this mounted reader, never the app's shared cache.
  const [client] = useState(() => new QueryClient())
  const query = useInfiniteQuery(
    {
      queryKey: ['request-content', props.sessionKey, props.requestId],
      initialPageParam: { after: -1, limit: CONTENT_PAGE_SIZE },
      queryFn: async ({ signal, pageParam }): Promise<ContentPage | null> => {
        if (contentSessionKey() !== props.sessionKey) {
          throw new Error('Session changed')
        }
        const response = await api.get(
          `/api/log/content/${encodeURIComponent(props.requestId)}`,
          {
            params: pageParam,
            signal,
            disableDuplicate: true,
          }
        )
        if (signal.aborted || contentSessionKey() !== props.sessionKey) {
          throw new Error('Session changed')
        }
        const data = requireServerSuccess(response.data)?.data
        if (data == null) return null
        const page = contentPageSchema.parse(data)
        if (page.expires_at * 1000 <= Date.now()) return null
        if (
          page.request_id !== props.requestId ||
          page.events.length > pageParam.limit
        ) {
          throw new Error('Invalid content page')
        }
        let previous = pageParam.after
        for (const event of page.events) {
          if (
            event.request_id !== props.requestId ||
            event.sequence <= previous ||
            contentBytes([event]) > CONTENT_CHUNK_BYTES
          ) {
            throw new Error('Invalid content sequence')
          }
          previous = event.sequence
        }
        if (
          page.next_sequence !== null &&
          (page.next_sequence !== previous || previous <= pageParam.after)
        ) {
          throw new Error('Invalid content cursor')
        }
        return page
      },
      getNextPageParam: (page, pages) => {
        if (!page || page.next_sequence === null) return undefined
        const size = pages.reduce(
          (sum, item) => sum + contentBytes(item?.events ?? []),
          0
        )
        const limit = Math.min(
          CONTENT_PAGE_SIZE,
          Math.floor((CONTENT_DISPLAY_BYTES - size) / CONTENT_CHUNK_BYTES)
        )
        // Reserve a worst-case page before requesting any more plaintext.
        if (limit < 1) return undefined
        return { after: page.next_sequence, limit }
      },
      retry: false,
      gcTime: 0,
      staleTime: Infinity,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
      meta: { errorToast: false },
    },
    client
  )

  useEffect(
    () => () => {
      void client.cancelQueries()
      client.clear()
    },
    [client]
  )

  useEffect(() => {
    if (query.isError) reportError()
  }, [query.isError, reportError])

  const pages = query.data?.pages
  const events = useMemo(
    () => pages?.flatMap((page) => page?.events ?? []) ?? [],
    [pages]
  )
  const page = pages?.at(-1)
  useEffect(() => {
    if (pages && pages.length > 1 && !page) {
      reportError()
      return
    }
    if (!page) return
    const remaining = page.expires_at * 1000 - Date.now()
    const timer = window.setTimeout(
      reportError,
      Math.max(0, Math.min(remaining, 2_147_483_647))
    )
    return () => window.clearTimeout(timer)
  }, [page, pages, reportError])
  const complete = page?.next_sequence === null && page.state !== 'archiving'
  const bodies = useMemo(
    () => ({
      request: decodeContent(events, 'request', complete),
      response: decodeContent(events, 'response', complete),
    }),
    [events, complete]
  )

  if (query.isError) return null
  if (query.isPending) {
    return <LoadingState size='sm' message={t('Loading retained content...')} />
  }
  if (!page) {
    return (
      <EmptyState
        className='min-h-32'
        title={t('No retained content')}
        description={t(
          'Content may not have been recorded, may still be archiving, or may have expired.'
        )}
      />
    )
  }
  const stateLabel = {
    archiving: t('Awaiting archive completion'),
    complete: t('Archive complete'),
    partial: t('Partial archive'),
  }[page.state]
  const outcomes: Record<string, string> = {
    completed: t('Completed'),
    failed: t('Failed'),
    incomplete: t('Incomplete'),
    cancelled: t('Cancelled'),
  }
  const atLimit = page.next_sequence !== null && !query.hasNextPage

  return (
    <div className='min-w-0 space-y-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <Badge variant='outline'>{stateLabel}</Badge>
        <Badge variant='outline'>
          {t('Model response: {{status}}', {
            status: outcomes[page.outcome] ?? t('Unknown'),
          })}
        </Badge>
        {(page.truncated || page.incomplete) && (
          <Badge variant='outline'>{t('Content is incomplete')}</Badge>
        )}
        <span className='text-muted-foreground text-xs'>
          {t('{{count}} bytes loaded', {
            count: formatNumber(
              contentBytes(events),
              toIntlLocale(i18n.resolvedLanguage || i18n.language)
            ),
          })}
        </span>
      </div>
      {page.state === 'archiving' && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'The archive is not yet complete; recording may still be in progress or may have stopped unexpectedly.'
          )}
        </p>
      )}
      {(['request', 'response'] as const).map((direction) => (
        <section
          key={direction}
          className='min-w-0 space-y-2'
          aria-label={
            direction === 'request' ? t('Request body') : t('Response body')
          }
        >
          <div className='flex items-center justify-between gap-2'>
            <h4 className='text-sm font-medium'>
              {direction === 'request' ? t('Request body') : t('Response body')}
            </h4>
            <div className='flex items-center gap-2'>
              {bodies[direction].binary && (
                <Badge variant='outline'>Base64</Badge>
              )}
              {bodies[direction].text && (
                <CopyButton
                  value={bodies[direction].text}
                  aria-label={
                    direction === 'request'
                      ? t('Copy request body')
                      : t('Copy response body')
                  }
                  size='sm'
                />
              )}
            </div>
          </div>
          <pre className='bg-muted/40 max-h-80 overflow-auto rounded-md border p-3 font-mono text-xs wrap-anywhere whitespace-pre-wrap'>
            {bodies[direction].text || t('No content loaded')}
          </pre>
        </section>
      ))}
      {query.hasNextPage && (
        <Button
          size='sm'
          variant='outline'
          disabled={query.isFetchingNextPage}
          onClick={() => void query.fetchNextPage()}
        >
          {query.isFetchingNextPage ? t('Loading...') : t('Load more content')}
        </Button>
      )}
      {atLimit && (
        <p className='text-muted-foreground text-xs' role='status'>
          {t(
            'The display limit has been reached. Additional archived content is not loaded into this browser.'
          )}
        </p>
      )}
    </div>
  )
}

function ScopedContentPanel(props: Omit<ReaderProps, 'onError'>) {
  const { t } = useTranslation()
  const id = useId()
  const [expanded, setExpanded] = useState(false)
  const [failed, setFailed] = useState(false)
  const [generation, setGeneration] = useState(0)
  const reload = () => {
    setFailed(false)
    setGeneration((value) => value + 1)
  }

  return (
    <DetailSection label={t('Request content retention')}>
      <div className='flex flex-wrap gap-2'>
        <Button
          size='sm'
          variant='outline'
          aria-expanded={expanded}
          aria-controls={id}
          onClick={() => {
            setExpanded((value) => !value)
            setFailed(false)
          }}
        >
          {expanded
            ? t('Hide retained content')
            : t('View request and response')}
        </Button>
        {expanded && (
          <Button size='sm' variant='ghost' onClick={reload}>
            {t('Refresh')}
          </Button>
        )}
      </div>
      {expanded && (
        <div id={id} className='min-w-0'>
          {failed ? (
            <ErrorState
              className='min-h-32'
              title={t('Unable to read retained content')}
              onRetry={reload}
            />
          ) : (
            <RequestContentReader
              key={generation}
              {...props}
              onError={() => setFailed(true)}
            />
          )}
        </div>
      )}
    </DetailSection>
  )
}

export function RequestContentPanel(props: {
  requestId: string
  isRoot: boolean
  open: boolean
}) {
  const sessionKey = useContentSession()
  if (!props.open || !props.isRoot || !sessionKey || !props.requestId) {
    return null
  }
  return (
    <ScopedContentPanel
      key={`${sessionKey}:${props.requestId}`}
      requestId={props.requestId}
      sessionKey={sessionKey}
    />
  )
}
