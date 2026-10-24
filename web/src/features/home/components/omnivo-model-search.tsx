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
import { useNavigate } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { SearchBar } from '@/features/pricing/components/search-bar'

import { useHomeCatalogAccess } from '../hooks/use-home-catalog-access'

export function OmnivoModelSearch() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const access = useHomeCatalogAccess()

  if (!access.ready || !access.enabled) return null

  return (
    <form
      role='search'
      aria-label={t('Find your next model')}
      className='mt-8 flex max-w-xl flex-col gap-3 sm:flex-row'
      onSubmit={(event) => {
        event.preventDefault()
        void navigate({
          to: '/pricing',
          search: { search: search.trim() || undefined, tokenUnit: 'M' },
        })
      }}
    >
      <SearchBar
        value={search}
        onChange={setSearch}
        onClear={() => setSearch('')}
        placeholder={t('Search by model name or provider')}
        className='min-w-0 flex-1'
      />
      <Button type='submit' size='lg' className='h-12 px-5'>
        {t('Find models')}
        <ArrowRight aria-hidden='true' />
      </Button>
    </form>
  )
}
