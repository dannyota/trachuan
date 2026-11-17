import type { components } from '@trachuan/schema/gen/openapi'
import type { Language } from '../../i18n'
import { formatNumber } from '../content'

function trimWords(value: string, limit: number): string {
  const text = value.normalize('NFC').trim()
  if (text.length <= limit) return text
  const part = text.slice(0, limit + 1)
  const boundary = part.search(/\s+\S*$/u)
  return boundary > 0 ? part.slice(0, boundary) : text.slice(0, limit)
}

export function topicSeo(node: components['schemas']['NodeEntry'], language: Language) {
  const suffix = language === 'vi' ? ' · Chủ đề · Tra Chuẩn' : ' · Topics · Tra Chuẩn'
  const name = node.name ?? ''
  const documents = formatNumber(node.documents ?? 0, language)
  const provisions = formatNumber(node.provisions ?? 0, language)
  const prefix = language === 'vi' ? 'Văn bản và chủ đề con của “' : 'Documents and subtopics under “'
  const ending = language === 'vi' ? `” trong Bộ pháp điển: ${documents} văn bản, ${provisions} quy định.` : `” in the legal code: ${documents} documents, ${provisions} provisions.`
  const description = node.description || `${prefix}${trimWords(name, 160 - prefix.length - ending.length)}${ending}`
  return { title: `${trimWords(name, 60 - suffix.length)}${suffix}`, description: trimWords(description, 160) }
}
