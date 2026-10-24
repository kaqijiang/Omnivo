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
import { act, cleanup, renderHook } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeEach, expect, it } from 'vitest'

import { setDefaultDocumentTitle } from '@/lib/document-title'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { useModelPageMetadata } from '../hooks/use-model-page-metadata'
import type { PricingModel } from '../types'

const model: PricingModel = {
  id: 1,
  model_name: 'provider/model',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['default'],
}
const originalName = useSystemConfigStore.getState().config.systemName
let siteDescription: HTMLMetaElement

beforeEach(async () => {
  await i18next.changeLanguage('en')
  useSystemConfigStore.getState().setConfig({ systemName: 'Omnivo' })
  setDefaultDocumentTitle('Omnivo home')
  siteDescription = document.createElement('meta')
  siteDescription.name = 'description'
  siteDescription.content = 'Site description'
  document.head.append(siteDescription)
})

afterEach(async () => {
  cleanup()
  siteDescription.remove()
  useSystemConfigStore.getState().setConfig({ systemName: originalName })
  await i18next.changeLanguage('en')
})

it('keeps a model title during background site-title refresh and restores the latest fallback on exit', () => {
  const page = renderHook(() => useModelPageMetadata(model))
  expect(document.title).toBe(
    'provider/model API — pricing and integration | Omnivo'
  )
  setDefaultDocumentTitle('Updated home title')
  expect(document.title).toContain('provider/model API')
  expect(document.querySelector('link[rel="canonical"]')).toHaveAttribute(
    'href',
    `${window.location.origin}/pricing/provider%2Fmodel`
  )
  page.unmount()
  expect(document.title).toBe('Updated home title')
  expect(siteDescription.content).toBe('Site description')
  expect(document.querySelector('link[rel="canonical"]')).toBeNull()
  expect(document.querySelector('meta[property="og:title"]')).toBeNull()
})

it('updates model identity, site branding and translated metadata without leaving the previous model behind', async () => {
  i18next.addResourceBundle(
    'zhCN',
    'translation',
    {
      '{{model}} API — pricing and integration | {{site}}':
        '{{model}} API — 价格与接入 | {{site}}',
      'Explore {{model}} pricing, capabilities and API examples on {{site}}.':
        '在 {{site}} 查看 {{model}} 的价格、能力与调用示例。',
    },
    true,
    true
  )
  const page = renderHook((props) => useModelPageMetadata(props), {
    initialProps: model,
  })
  await act(async () => {
    useSystemConfigStore.getState().setConfig({ systemName: 'Custom gateway' })
    await i18next.changeLanguage('zhCN')
  })
  expect(document.title).toBe(
    'provider/model API — 价格与接入 | Custom gateway'
  )
  page.rerender({ ...model, model_name: 'second-model' })
  expect(document.title).toContain('second-model API')
  expect(siteDescription.content).toBe(
    '在 Custom gateway 查看 second-model 的价格、能力与调用示例。'
  )
  expect(document.querySelector('meta[property="og:url"]')).toHaveAttribute(
    'content',
    `${window.location.origin}/pricing/second-model`
  )
})

it('leaves generic site metadata untouched when no visible model is available', () => {
  renderHook(() => useModelPageMetadata(null))
  expect(document.title).toBe('Omnivo home')
  expect(siteDescription.content).toBe('Site description')
  expect(document.querySelector('link[rel="canonical"]')).toBeNull()
})

it('preserves the newest route metadata when an older model page unmounts during a transition', () => {
  const first = renderHook(() => useModelPageMetadata(model))
  const second = renderHook(() =>
    useModelPageMetadata({ ...model, model_name: 'next-model' })
  )
  first.unmount()
  expect(document.title).toContain('next-model API')
  expect(siteDescription.content).toContain('next-model')
  second.unmount()
  expect(document.title).toBe('Omnivo home')
  expect(siteDescription.content).toBe('Site description')
  expect(document.querySelector('meta[property="og:title"]')).toBeNull()
})
