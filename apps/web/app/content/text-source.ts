import type { Language } from '../../i18n'
import { formatDate } from './format'

function host(sourceUrl: string | undefined, lang: Language): string {
  try {
    const url = new URL(sourceUrl || '')
    return url.hostname || (lang === 'vi' ? 'nguồn' : 'the source')
  } catch {
    return lang === 'vi' ? 'nguồn' : 'the source'
  }
}

export function textSourceLabel(textSource: string | undefined, textComplete: boolean, sourceUrl: string | undefined, lang: Language): string {
  if (textSource === 'none' || !textSource) return lang === 'vi' ? 'Chưa có nội dung' : 'No content available'
  const complete = lang === 'vi' ? (textComplete ? 'đầy đủ' : 'có thể chưa đầy đủ') : (textComplete ? 'complete' : 'may be incomplete')
  let source: string
  if (textSource === 'source_tree') source = lang === 'vi' ? `từ cấu trúc văn bản của ${host(sourceUrl, lang)}` : `from the text structure of ${host(sourceUrl, lang)}`
  else if (textSource === 'html_transcription') source = lang === 'vi' ? `từ bản HTML của ${host(sourceUrl, lang)}` : `from the HTML transcription at ${host(sourceUrl, lang)}`
  else if (textSource === 'ocr') source = lang === 'vi' ? 'từ bản quét (nhận dạng ký tự) đã kiểm tra' : 'from a checked scan (optical character recognition)'
  else return lang === 'vi' ? 'Chưa rõ nguồn nội dung' : 'Content source unknown'
  return `${lang === 'vi' ? 'Nội dung' : 'Content'}: ${complete}, ${source}`
}

export function statusAsOf(date: string, sourceUrl: string | undefined, lang: Language): string {
  const formatted = formatDate(date, lang)
  return lang === 'vi' ? `Trạng thái tính đến ${formatted}, theo ${host(sourceUrl, lang)}` : `Status as of ${formatted}, according to ${host(sourceUrl, lang)}`
}

export function pinnedNotice(since: string | undefined, lang: Language): string | null {
  if (!since) return null
  const date = formatDate(since, lang)
  return lang === 'vi' ? `Nguồn đã gỡ văn bản này từ ${date}; nội dung lấy từ bản đã kiểm tra trước đó.` : `The source removed this document on ${date}; the content comes from an earlier checked copy.`
}
