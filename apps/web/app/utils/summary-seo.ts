import type { components } from '@trachuan/schema/gen/openapi'

export function summarySeo(document: components['schemas']['PublicSummaryDocument']) {
  const title = `${document.type} ${document.number} · Tra Chuẩn`.normalize('NFC').trim()
  const text = document.title.normalize('NFC').trim().replace(/\s+/gu, ' ')
  const part = text.slice(0, 160)
  const boundary = part.search(/\s+\S*$/u)
  return { title: title.length <= 60 ? title : document.number, description: text.length < 160 ? text : boundary > 0 ? part.slice(0, boundary) : '' }
}
