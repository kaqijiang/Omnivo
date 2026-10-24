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
import { ArrowUpRight, Layers3, Terminal, Waypoints } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout(props: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div className='bg-background relative min-h-svh lg:grid lg:grid-cols-[1fr_1fr]'>
      <Link
        to='/'
        aria-label={t('Go to home')}
        className='focus-visible:ring-ring absolute top-6 left-6 z-10 flex items-center gap-3 rounded-lg transition-opacity outline-none hover:opacity-80 focus-visible:ring-2 sm:top-8 sm:left-10 lg:text-[var(--omnivo-paper)]'
      >
        {loading ? (
          <>
            <Skeleton className='size-9 rounded-xl' />
            <Skeleton className='h-6 w-24' />
          </>
        ) : (
          <>
            <img
              src={logo}
              alt=''
              className='size-9 rounded-xl object-contain'
            />
            <span className='text-2xl font-semibold tracking-tight'>
              {systemName}
            </span>
          </>
        )}
      </Link>
      <aside className='relative hidden min-h-svh flex-col justify-between overflow-hidden bg-[var(--omnivo-ink)] px-12 pt-40 pb-10 text-[var(--omnivo-paper)] lg:flex xl:px-16'>
        <div
          aria-hidden='true'
          className='pointer-events-none absolute -right-40 bottom-10 size-[480px] rounded-full border border-[var(--omnivo-mint)]/10'
        />
        <div
          aria-hidden='true'
          className='pointer-events-none absolute -right-24 bottom-26 size-[350px] rounded-full border border-[var(--omnivo-mint)]/15'
        />
        <div className='relative my-auto flex max-w-lg flex-col gap-7 pb-16'>
          <img
            src='/omnivo-mark.svg'
            alt=''
            aria-hidden='true'
            className='mb-1 size-16'
          />
          <h1 className='max-w-sm text-5xl leading-[1.1] font-semibold tracking-tight xl:text-6xl'>
            {t('Many models. One API.')}
          </h1>
          <p className='max-w-sm text-lg leading-8 text-[var(--omnivo-paper)]/75'>
            {t(
              'One account for your models, keys, and usage. Build what comes next.'
            )}
          </p>
          <ul className='mt-3 flex flex-col gap-5 text-sm text-[var(--omnivo-paper)]/85'>
            <li className='flex items-center gap-3'>
              <Layers3
                aria-hidden='true'
                className='size-5 text-[var(--omnivo-mint)]'
              />
              {t('Explore models in one place')}
            </li>
            <li className='flex items-center gap-3'>
              <Terminal
                aria-hidden='true'
                className='size-5 text-[var(--omnivo-mint)]'
              />
              {t('Connect with familiar developer tools')}
            </li>
            <li className='flex items-center gap-3'>
              <Waypoints
                aria-hidden='true'
                className='size-5 text-[var(--omnivo-mint)]'
              />
              {t('Keep your keys and usage together')}
            </li>
          </ul>
        </div>
        <p className='relative text-xs tracking-[0.18em] text-[var(--omnivo-paper)]/60 uppercase'>
          Omnivo
        </p>
      </aside>
      <main className='flex min-h-svh min-w-0 flex-col px-6 pt-24 pb-8 sm:px-10 lg:pt-8'>
        <nav
          aria-label={t('Explore Omnivo')}
          className='flex flex-wrap justify-end gap-6 text-sm'
        >
          <Link
            to='/pricing'
            className='text-muted-foreground hover:text-foreground focus-visible:ring-ring rounded outline-none focus-visible:ring-2'
          >
            {t('Models')}
          </Link>
          <Link
            to='/docs'
            className='text-muted-foreground hover:text-foreground focus-visible:ring-ring inline-flex items-center gap-1 rounded outline-none focus-visible:ring-2'
          >
            {t('Documentation')}
            <ArrowUpRight aria-hidden='true' className='size-3.5' />
          </Link>
        </nav>
        <div className='mx-auto my-auto flex w-full max-w-[400px] flex-col gap-2 py-14'>
          {props.children}
        </div>
        <p className='text-muted-foreground text-center text-xs'>
          {t('Many models. One API.')}
        </p>
      </main>
    </div>
  )
}
