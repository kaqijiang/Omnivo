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
import { describe, expect, it } from 'vitest'

import { resolveDocumentationLink, resolveSiteBrand } from '../omnivo-brand'

describe('site branding', () => {
  it('shows Omnivo while the server uses its upstream defaults', () => {
    expect(resolveSiteBrand('New API', '/logo.png')).toEqual({
      systemName: 'Omnivo',
      logo: '/omnivo-icon.svg',
    })
    expect(resolveSiteBrand()).toEqual(resolveSiteBrand('New API', '/logo.png'))
  })

  it('preserves the administrator’s custom name and logo independently', () => {
    expect(resolveSiteBrand('My workspace', '/custom.svg')).toEqual({
      systemName: 'My workspace',
      logo: '/custom.svg',
    })
    expect(resolveSiteBrand('My workspace', '')).toEqual({
      systemName: 'My workspace',
      logo: '/omnivo-icon.svg',
    })
  })
})

describe('documentation navigation', () => {
  it('opens the built-in guide for the upstream default or an unset link', () => {
    expect(resolveDocumentationLink('https://docs.newapi.pro')).toEqual({
      href: '/docs',
      external: false,
    })
    expect(resolveDocumentationLink()).toEqual({
      href: '/docs',
      external: false,
    })
  })

  it('preserves custom destinations and keeps relative guides in the same tab', () => {
    expect(resolveDocumentationLink('https://example.com/guide')).toEqual({
      href: 'https://example.com/guide',
      external: true,
    })
    expect(resolveDocumentationLink('/guide')).toEqual({
      href: '/guide',
      external: false,
    })
  })
})
