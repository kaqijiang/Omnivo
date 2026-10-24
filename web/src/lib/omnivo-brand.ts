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
import { DEFAULT_LOGO, DEFAULT_SYSTEM_NAME } from './constants'

export const OMNIVO_NAME = 'Omnivo'
export const OMNIVO_LOGO = '/omnivo-icon.svg'

/** Display branding only; retain the server's configuration and custom identity. */
export function resolveSiteBrand(systemName?: string, logo?: string) {
  return {
    systemName:
      !systemName || systemName === DEFAULT_SYSTEM_NAME
        ? OMNIVO_NAME
        : systemName,
    logo: !logo || logo === DEFAULT_LOGO ? OMNIVO_LOGO : logo,
  }
}

export function resolveDocumentationLink(configuredUrl?: string) {
  const href =
    !configuredUrl || configuredUrl === 'https://docs.newapi.pro'
      ? '/docs'
      : configuredUrl
  return { href, external: /^https?:\/\//i.test(href) }
}
