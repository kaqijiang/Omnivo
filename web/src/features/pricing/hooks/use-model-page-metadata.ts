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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { claimDocumentTitle } from '@/lib/document-title'
import { resolveSiteBrand } from '@/lib/omnivo-brand'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { PricingModel } from '../types'

type HeadAttributeOwner = {
  element: HTMLElement
  attribute: string
  original: string | null
  created: boolean
  values: Map<symbol, string>
}

const headOwners = new Map<string, HeadAttributeOwner>()

/** Preserve the site metadata even if two route transitions overlap. */
function claimHeadAttribute(
  selector: string,
  create: () => HTMLElement,
  attribute: string,
  value: string
) {
  let entry = headOwners.get(selector)
  if (!entry) {
    const existing = document.head.querySelector<HTMLElement>(selector)
    const element = existing ?? create()
    entry = {
      element,
      attribute,
      original: element.getAttribute(attribute),
      created: !existing,
      values: new Map(),
    }
    headOwners.set(selector, entry)
    if (!existing) document.head.append(element)
  }
  const owner = Symbol('model-page-metadata')
  entry.values.set(owner, value)
  entry.element.setAttribute(attribute, value)
  return () => {
    const previousValue = [...entry.values.values()].at(-1)
    entry.values.delete(owner)
    const nextValue = [...entry.values.values()].at(-1)
    if (nextValue !== undefined) {
      entry.element.setAttribute(entry.attribute, nextValue)
      return
    }
    headOwners.delete(selector)
    if (entry.element.getAttribute(attribute) !== previousValue) return
    if (entry.created) entry.element.remove()
    else if (entry.original === null) entry.element.removeAttribute(attribute)
    else entry.element.setAttribute(attribute, entry.original)
  }
}

/** Client-side metadata for a visible model; route access rules stay upstream. */
export function useModelPageMetadata(model: PricingModel | null) {
  const { t } = useTranslation()
  const systemName = useSystemConfigStore((state) => state.config.systemName)
  const brand = resolveSiteBrand(systemName).systemName
  const modelName = model?.model_name
  const title = modelName
    ? t('{{model}} API — pricing and integration | {{site}}', {
        model: modelName,
        site: brand,
        interpolation: { escapeValue: false },
      })
    : null
  const description = modelName
    ? t(
        'Explore {{model}} pricing, capabilities and API examples on {{site}}.',
        {
          model: modelName,
          site: brand,
          interpolation: { escapeValue: false },
        }
      )
    : null

  useEffect(() => {
    if (!modelName || !title || !description) return

    const releaseTitle = claimDocumentTitle(title)
    const canonical = new URL(
      `/pricing/${encodeURIComponent(modelName)}`,
      window.location.origin
    ).href
    const metadata = [
      { attribute: 'name', name: 'description', content: description },
      { attribute: 'property', name: 'og:title', content: title },
      { attribute: 'property', name: 'og:description', content: description },
      { attribute: 'property', name: 'og:url', content: canonical },
      { attribute: 'property', name: 'og:type', content: 'website' },
    ]
    const cleanups = metadata.map((item) =>
      claimHeadAttribute(
        `meta[${item.attribute}="${item.name}"]`,
        () => {
          const element = document.createElement('meta')
          element.setAttribute(item.attribute, item.name)
          return element
        },
        'content',
        item.content
      )
    )
    cleanups.push(
      claimHeadAttribute(
        'link[rel="canonical"]',
        () => {
          const element = document.createElement('link')
          element.rel = 'canonical'
          return element
        },
        'href',
        canonical
      )
    )

    return () => {
      releaseTitle()
      for (const cleanup of cleanups) cleanup()
    }
  }, [modelName, title, description])
}
