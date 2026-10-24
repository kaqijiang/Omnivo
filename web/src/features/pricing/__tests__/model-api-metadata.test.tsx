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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { ModelDetailsApi } from '../components/model-details-api'
import type { PricingModel } from '../types'

const model: PricingModel = {
  id: 1,
  model_name: 'catalog-chat-model',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['default', 'premium'],
  supported_endpoint_types: [],
}

let queryClient: QueryClient

beforeEach(() => {
  localStorage.clear()
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], {
    server_address: 'https://api.omnivo.test',
  })
})

afterEach(() => {
  queryClient.clear()
  localStorage.clear()
})

describe('model API metadata', () => {
  it('does not advertise generated request limits when the catalog has no configured limits', () => {
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsApi model={model} endpointMap={{}} />
      </QueryClientProvider>
    )

    expect(
      screen.queryByRole('heading', { name: 'Rate limits' })
    ).not.toBeInTheDocument()
    for (const name of ['RPM', 'TPM', 'RPD']) {
      expect(
        screen.queryByRole('columnheader', { name })
      ).not.toBeInTheDocument()
    }
  })

  it('does not infer supported parameters from a model name without authoritative metadata', () => {
    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsApi
          model={{ ...model, model_name: 'sora-video-pro' }}
          endpointMap={{}}
        />
      </QueryClientProvider>
    )

    expect(
      screen.queryByRole('heading', { name: 'Supported parameters' })
    ).not.toBeInTheDocument()

    rerender(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsApi
          model={{ ...model, model_name: 'o1-reasoning-model' }}
          endpointMap={{}}
        />
      </QueryClientProvider>
    )

    expect(
      screen.queryByRole('heading', { name: 'Supported parameters' })
    ).not.toBeInTheDocument()
  })

  it('keeps chat examples minimal instead of assuming optional parameter support from the model name', () => {
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsApi
          model={{ ...model, supported_endpoint_types: ['openai'] }}
          endpointMap={{ openai: { path: '/v1/chat/completions' } }}
        />
      </QueryClientProvider>
    )

    const sample = screen.getByRole('textbox', { name: 'bash' })
    expect(sample).toHaveTextContent('catalog-chat-model')
    expect(sample).toHaveTextContent('messages')
    expect(sample).not.toHaveTextContent('temperature')
  })

  it('uses the selected supported protocol and model ID when switching code sample languages', async () => {
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsApi
          model={{
            ...model,
            model_name: 'vendor/model-beta',
            supported_endpoint_types: ['openai', 'anthropic'],
          }}
          endpointMap={{
            openai: { path: '/v1/chat/completions' },
            anthropic: { path: '/v1/messages' },
          }}
        />
      </QueryClientProvider>
    )

    expect(screen.getByRole('textbox', { name: 'bash' })).toHaveTextContent(
      'https://api.omnivo.test/v1/chat/completions'
    )
    await user.click(screen.getByRole('tab', { name: 'anthropic' }))
    expect(screen.getByRole('tab', { name: 'anthropic' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    const anthropicSample = screen.getByRole('textbox', { name: 'bash' })
    expect(anthropicSample).toHaveTextContent(
      'https://api.omnivo.test/v1/messages'
    )
    expect(anthropicSample).toHaveTextContent('vendor/model-beta')
    expect(anthropicSample).toHaveTextContent('max_tokens')

    await user.click(screen.getByRole('tab', { name: 'Python' }))
    const pythonSample = screen.getByRole('textbox', { name: 'python' })
    expect(pythonSample).toHaveTextContent('client.messages.create')
    expect(pythonSample).toHaveTextContent('vendor/model-beta')
  })

  it('does not offer code examples when the model has no configured endpoint path', () => {
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsApi
          model={{ ...model, supported_endpoint_types: ['openai'] }}
          endpointMap={{}}
        />
      </QueryClientProvider>
    )

    expect(
      screen.queryByRole('heading', { name: 'Code samples' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('tab', { name: 'Python' })
    ).not.toBeInTheDocument()
  })
})
