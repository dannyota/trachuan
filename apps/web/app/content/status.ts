import type { components } from '@trachuan/schema/gen/openapi'
import type { Language } from '../../i18n'

type StatusStamp = components['schemas']['StatusStamp']
const statuses: Record<string, Record<Language, string>> = {
  in_force: { vi: 'Còn hiệu lực', en: 'In force' },
  partial: { vi: 'Còn hiệu lực một phần', en: 'Partly in force' },
  not_yet: { vi: 'Chưa có hiệu lực', en: 'Not yet in force' },
  expired: { vi: 'Hết hiệu lực', en: 'Expired' },
}
const bases: Record<string, Record<Language, string>> = {
  derived_from_consolidated: { vi: 'suy ra từ văn bản hợp nhất', en: 'derived from a consolidated document' },
  derived_from_text: { vi: 'suy ra từ nội dung văn bản', en: 'derived from the document text' },
  manual_correction: { vi: 'đã hiệu chỉnh theo bằng chứng', en: 'corrected using evidence' },
}

export function statusLabel(stamp: StatusStamp | undefined, lang: Language): string {
  const key = stamp?.class
  return key && Object.hasOwn(statuses, key) ? statuses[key]![lang] : (lang === 'vi' ? 'Chưa rõ hiệu lực' : 'Validity unknown')
}

export function statusBasisLabel(stamp: StatusStamp | undefined, sourceUrl: string | undefined, lang: Language): string | null {
  const key = stamp?.status_basis
  if (key === 'source_status') {
    let host = lang === 'vi' ? 'nguồn' : 'the source'
    try { host = new URL(sourceUrl || '').hostname || host } catch { host = lang === 'vi' ? 'nguồn' : 'the source' }
    return lang === 'vi' ? `theo ${host}` : `according to ${host}`
  }
  return key && Object.hasOwn(bases, key) ? bases[key]![lang] : null
}
