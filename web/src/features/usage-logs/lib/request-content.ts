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
import { z } from 'zod'

export const CONTENT_DISPLAY_BYTES = 4 * 1024 * 1024
export const CONTENT_CHUNK_BYTES = 256 * 1024
export const CONTENT_PAGE_SIZE = 8

const contentEventSchema = z.object({
  request_id: z.string().max(128),
  kind: z.enum(['start', 'data', 'end']),
  sequence: z.number().int().nonnegative().safe(),
  direction: z.enum(['request', 'response']).optional(),
  data: z
    .string()
    .max(349528)
    .regex(/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/)
    .optional(),
})

export const contentPageSchema = z.object({
  request_id: z.string().max(128),
  state: z.enum(['archiving', 'complete', 'partial']),
  outcome: z.string().max(64).default('unknown'),
  expires_at: z.number().int().positive(),
  stream: z.boolean(),
  truncated: z.boolean(),
  incomplete: z.boolean(),
  events: z.array(contentEventSchema).max(CONTENT_PAGE_SIZE),
  next_sequence: z.number().int().nonnegative().safe().nullable(),
})

export type ContentEvent = z.infer<typeof contentEventSchema>
export type ContentPage = z.infer<typeof contentPageSchema>

export function contentBytes(events: ContentEvent[]): number {
  return events.reduce((total, event) => {
    const data = event.data ?? ''
    const padding = data.endsWith('==') ? 2 : Number(data.endsWith('='))
    return total + Math.floor((data.length * 3) / 4) - padding
  }, 0)
}

// A UTF-8 character may straddle fragments or pages. Decode each direction as
// one byte stream, preserving its unfinished tail until the next page arrives.
export function decodeContent(
  events: ContentEvent[],
  direction: 'request' | 'response',
  complete: boolean
): { text: string; binary: boolean } {
  const chunks = events
    .filter((event) => event.kind === 'data' && event.direction === direction)
    .map((event) =>
      Uint8Array.from(atob(event.data ?? ''), (c) => c.charCodeAt(0))
    )
  const decoder = new TextDecoder('utf-8', { fatal: true })
  try {
    let text = ''
    for (const chunk of chunks) {
      if (chunk.includes(0)) throw new Error('Binary content')
      text += decoder.decode(chunk, { stream: true })
    }
    if (complete) text += decoder.decode()
    return { text, binary: false }
  } catch {
    // Concatenating padded base64 fragments would change the original bytes.
    let bytes = ''
    for (const chunk of chunks) {
      for (const byte of chunk) bytes += String.fromCharCode(byte)
    }
    return { text: btoa(bytes), binary: true }
  }
}
