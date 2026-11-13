import type { Language } from '../../i18n'

type Labels = Record<Language, { in: string, out: string }>
const labels: Record<string, Labels> = {
  repeals: { vi: { in: 'Bị bãi bỏ bởi', out: 'Bãi bỏ' }, en: { in: 'Repealed by', out: 'Repeals' } },
  replaces: { vi: { in: 'Bị thay thế bởi', out: 'Thay thế' }, en: { in: 'Replaced by', out: 'Replaces' } },
  suspends: { vi: { in: 'Bị đình chỉ thi hành bởi', out: 'Đình chỉ thi hành' }, en: { in: 'Suspended by', out: 'Suspends' } },
  amends_supplements: { vi: { in: 'Được sửa đổi, bổ sung bởi', out: 'Sửa đổi, bổ sung' }, en: { in: 'Amended by', out: 'Amends' } },
  consolidates: { vi: { in: 'Được hợp nhất trong', out: 'Hợp nhất' }, en: { in: 'Consolidated in', out: 'Consolidates' } },
  implements: { vi: { in: 'Được quy định chi tiết, hướng dẫn bởi', out: 'Quy định chi tiết, hướng dẫn' }, en: { in: 'Implemented by', out: 'Implements' } },
  guided_by: { vi: { in: 'Hướng dẫn', out: 'Được hướng dẫn bởi' }, en: { in: 'Guides', out: 'Guided by' } },
  legal_basis: { vi: { in: 'Là căn cứ ban hành của', out: 'Căn cứ ban hành' }, en: { in: 'Legal basis for', out: 'Based on' } },
  cites: { vi: { in: 'Được dẫn chiếu bởi', out: 'Dẫn chiếu' }, en: { in: 'Cited by', out: 'Cites' } },
  references: { vi: { in: 'Văn bản liên quan', out: 'Văn bản liên quan' }, en: { in: 'Related', out: 'Related' } },
}

export function relationLabel(kind: string | undefined, dir: string | undefined, lang: Language): string {
  if (!kind || !Object.hasOwn(labels, kind) || (dir !== 'in' && dir !== 'out')) return lang === 'vi' ? 'Quan hệ khác' : 'Other relation'
  return labels[kind]![lang][dir]
}

export function relationCountLabel(key: string, lang: Language): string {
  const parts = key.split(':')
  return parts.length === 2 ? relationLabel(parts[1], parts[0], lang) : relationLabel(undefined, undefined, lang)
}
