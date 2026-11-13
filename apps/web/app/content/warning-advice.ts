import type { Language } from '../../i18n'

const advice: Record<string, Record<Language, string>> = {
  amends_supplements: { vi: 'Đối chiếu văn bản sửa đổi trước khi dẫn chiếu.', en: 'Check the amending act before citing.' },
  repeals: { vi: 'Đối chiếu văn bản bãi bỏ trước khi dẫn chiếu.', en: 'Check the repealing act before citing.' },
  replaces: { vi: 'Đối chiếu văn bản thay thế trước khi dẫn chiếu.', en: 'Check the replacement act before citing.' },
  suspends: { vi: 'Đối chiếu văn bản ngưng hiệu lực trước khi dẫn chiếu.', en: 'Check the suspending act before citing.' },
}
export function warningAdvice(kind: string | undefined, lang: Language): string | null {
  return kind && Object.hasOwn(advice, kind) ? advice[kind]![lang] : null
}
