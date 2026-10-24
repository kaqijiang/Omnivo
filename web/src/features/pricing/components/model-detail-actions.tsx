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
import { ArrowUpRight, KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'

import type { TokenUnit } from '../types'

export interface ModelGuideSearch {
  search?: string
  sort?: string
  vendor?: string
  group?: string
  quotaType?: string
  endpointType?: string
  tag?: string
  tokenUnit?: TokenUnit
  view?: 'card' | 'table'
  rechargePrice?: boolean
}

export function ModelDetailActions(props: {
  modelName: string
  showFullPageLink?: boolean
  fullPageSearch?: ModelGuideSearch
}) {
  const { t } = useTranslation()
  const isAuthenticated = useAuthStore((state) => Boolean(state.auth.user))
  const url = new URL(
    `/pricing/${encodeURIComponent(props.modelName)}`,
    window.location.origin
  ).href

  return (
    <>
      <Button
        role='link'
        size='sm'
        render={<Link to={isAuthenticated ? '/keys' : '/sign-up'} />}
      >
        <KeyRound aria-hidden='true' />
        {t('Get an API key')}
      </Button>
      {props.showFullPageLink && (
        <Button
          role='link'
          variant='outline'
          size='sm'
          render={
            <Link
              to='/pricing/$modelId'
              params={{ modelId: props.modelName }}
              search={props.fullPageSearch}
            />
          }
        >
          {t('Full model guide')}
          <ArrowUpRight aria-hidden='true' />
        </Button>
      )}
      <CopyButton
        value={url}
        variant='outline'
        size='sm'
        tooltip={t('Copy model page link')}
      >
        {t('Share model')}
      </CopyButton>
    </>
  )
}
