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
import { Terminal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Separator } from '@/components/ui/separator'

const exampleRequest = String.raw`curl "$OMNIVO_BASE_URL/chat/completions" \
  -H "Authorization: Bearer $OMNIVO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MODEL_ID",
    "messages": [
      {"role": "user", "content": "Hello, Omnivo!"}
    ]
  }'`

export function OmnivoCodeSample() {
  const { t } = useTranslation()

  return (
    <div className='omnivo-inverse bg-background text-foreground border-border overflow-hidden rounded-3xl border shadow-xl'>
      <div
        aria-hidden='true'
        className='relative flex h-36 items-center justify-center overflow-hidden sm:h-44'
      >
        <div className='border-primary/15 absolute size-72 rounded-full border' />
        <div className='border-primary/15 absolute size-48 rounded-full border' />
        <div className='border-primary/15 absolute size-24 rounded-full border' />
        <span className='bg-background text-muted-foreground border-border absolute left-6 rounded-full border px-3 py-1 font-mono text-[11px]'>
          messages
        </span>
        <span className='bg-background text-muted-foreground border-border absolute right-6 rounded-full border px-3 py-1 font-mono text-[11px]'>
          responses
        </span>
        <img src='/omnivo-mark.svg' alt='' className='relative size-20' />
      </div>
      <Separator />
      <div className='flex items-center justify-between gap-3 px-5 py-3'>
        <div className='text-muted-foreground flex items-center gap-2 font-mono text-xs'>
          <Terminal aria-hidden='true' className='size-4' />
          <span>{t('Your first request')}</span>
        </div>
        <CopyButton
          value={exampleRequest}
          aria-label={t('Copy example request')}
          size='sm'
        >
          {t('Copy')}
        </CopyButton>
      </div>
      <pre
        tabIndex={0}
        aria-label={t('Example API request')}
        className='text-primary overflow-x-auto px-5 pb-6 font-mono text-[11px] leading-7 outline-offset-[-3px] sm:text-xs'
      >
        <code>{exampleRequest}</code>
      </pre>
      <div className='border-border text-muted-foreground border-t px-5 py-4 text-xs leading-5'>
        {t(
          'Set your API base URL and key, then replace MODEL_ID with a model available to your account.'
        )}
      </div>
    </div>
  )
}
