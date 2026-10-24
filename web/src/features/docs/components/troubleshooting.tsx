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
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export function Troubleshooting() {
  const { t } = useTranslation()
  return (
    <section id='troubleshooting' className='scroll-mt-24'>
      <h2 className='mb-5 text-2xl font-semibold tracking-tight'>
        {t('Troubleshooting')}
      </h2>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className='w-20'>{t('Status')}</TableHead>
            <TableHead>{t('What to check')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell className='font-mono'>401</TableCell>
            <TableCell className='py-4 leading-7 whitespace-normal'>
              {t(
                'Check that OMNIVO_API_KEY is set and the key is valid. Send it in the Authorization: Bearer header.'
              )}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell className='font-mono'>403</TableCell>
            <TableCell className='py-4 leading-7 whitespace-normal'>
              {t(
                'Check your key permissions, allowed models, and group access. Read the response message for the exact restriction.'
              )}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell className='font-mono'>429</TableCell>
            <TableCell className='py-4 leading-7 whitespace-normal'>
              {t(
                'Read the response message for rate or quota limits. Reduce concurrency, retry with backoff when appropriate, and check your available balance.'
              )}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <div className='mt-8 flex flex-wrap items-center gap-4'>
        <Button render={<Link to='/pricing' />} nativeButton={false}>
          {t('Explore models')}
          <ArrowRight aria-hidden='true' />
        </Button>
        <Button
          variant='outline'
          render={<Link to='/keys' />}
          nativeButton={false}
        >
          {t('Manage API keys')}
        </Button>
      </div>
    </section>
  )
}
