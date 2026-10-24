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
import { useTranslation } from 'react-i18next'

export function GettingStarted() {
  const { t } = useTranslation()
  return (
    <section id='get-started' className='scroll-mt-24'>
      <h2 className='mb-6 text-2xl font-semibold tracking-tight'>
        {t('Before your first request')}
      </h2>
      <ol className='grid gap-5'>
        <li className='flex items-start gap-4'>
          <span
            className='bg-muted text-primary flex size-8 shrink-0 items-center justify-center rounded-full font-mono text-sm'
            aria-hidden='true'
          >
            01
          </span>
          <div>
            <Link
              to='/sign-up'
              className='hover:text-primary font-medium underline-offset-4 hover:underline'
            >
              {t('Create an account')}
            </Link>
            <p className='text-muted-foreground mt-1 text-sm leading-7'>
              {t('Sign in to access your keys, balance, and request history.')}
            </p>
          </div>
        </li>
        <li className='flex items-start gap-4'>
          <span
            className='bg-muted text-primary flex size-8 shrink-0 items-center justify-center rounded-full font-mono text-sm'
            aria-hidden='true'
          >
            02
          </span>
          <div>
            <Link
              to='/keys'
              className='hover:text-primary font-medium underline-offset-4 hover:underline'
            >
              {t('Create an API key')}
            </Link>
            <p className='text-muted-foreground mt-1 text-sm leading-7'>
              {t(
                'Save your Omnivo key as OMNIVO_API_KEY in your server environment. Keep it out of browser code and public repositories.'
              )}
            </p>
          </div>
        </li>
        <li className='flex items-start gap-4'>
          <span
            className='bg-muted text-primary flex size-8 shrink-0 items-center justify-center rounded-full font-mono text-sm'
            aria-hidden='true'
          >
            03
          </span>
          <div>
            <Link
              to='/pricing'
              className='hover:text-primary font-medium underline-offset-4 hover:underline'
            >
              {t('Choose an available model')}
            </Link>
            <p className='text-muted-foreground mt-1 text-sm leading-7'>
              {t(
                'Replace MODEL_ID with an enabled model that supports Chat Completions. Your key must have access and your account needs sufficient quota or an eligible subscription.'
              )}
            </p>
          </div>
        </li>
      </ol>
    </section>
  )
}
