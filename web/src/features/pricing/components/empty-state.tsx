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
import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState as SharedEmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'

export interface EmptyStateProps {
  searchQuery?: string
  hasActiveFilters: boolean
  onClearFilters: () => void
}

export function EmptyState(props: EmptyStateProps) {
  const { t } = useTranslation()
  const hasSearch = Boolean(props.searchQuery?.trim())
  const isFiltered = props.hasActiveFilters || hasSearch
  let description = t(
    'Models will appear here once they are available. Check back soon.'
  )
  if (hasSearch) {
    description = t(
      'No results for "{{query}}". Try adjusting your search or filters.',
      { query: props.searchQuery }
    )
  } else if (props.hasActiveFilters) {
    description = t('No models match your current filters.')
  }

  return (
    <SharedEmptyState
      icon={Search}
      bordered
      className='min-h-[340px] rounded-2xl'
      title={
        isFiltered
          ? t('No models found')
          : t('The model catalog is being prepared.')
      }
      description={description}
      action={
        isFiltered ? (
          <Button variant='outline' size='sm' onClick={props.onClearFilters}>
            {t('Clear all filters')}
          </Button>
        ) : undefined
      }
    />
  )
}
