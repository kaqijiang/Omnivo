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
/** Coordinate the site fallback with route-owned document titles. */
let defaultTitle: string | undefined
const pageTitles = new Map<symbol, string>()

function applyDocumentTitle() {
  const title = [...pageTitles.values()].at(-1) ?? defaultTitle
  if (title !== undefined) document.title = title
}

export function setDefaultDocumentTitle(title: string) {
  defaultTitle = title
  applyDocumentTitle()
}

/** Returns an ownership-aware cleanup, safe when route transitions overlap. */
export function claimDocumentTitle(title: string) {
  if (defaultTitle === undefined) defaultTitle = document.title
  const owner = Symbol('page-title')
  pageTitles.set(owner, title)
  applyDocumentTitle()
  return () => {
    pageTitles.delete(owner)
    applyDocumentTitle()
  }
}
