import type { Language } from './index'

export const en = {
  search: 'Search', topics: 'Topics', connect: 'Connect AI assistants', connectShort: 'Connect AI', sources: 'Sources',
  account: 'Account', signIn: 'Sign in', terms: 'Terms', privacy: 'Privacy', menu: 'Menu', language: 'Tiếng Việt',
  sourceNote: 'Sources: Vietnam National Database of Legal Documents (vbpl.vn), Government Portal.', copied: 'Copied',
}
export const vi: Record<keyof typeof en, string> = {
  search: 'Tra cứu', topics: 'Chủ đề', connect: 'Kết nối trợ lý AI', connectShort: 'Kết nối AI', sources: 'Nguồn dữ liệu',
  account: 'Tài khoản', signIn: 'Đăng nhập', terms: 'Điều khoản', privacy: 'Quyền riêng tư', menu: 'Menu', language: 'English',
  sourceNote: 'Nguồn: Cơ sở dữ liệu quốc gia về văn bản pháp luật (vbpl.vn), Cổng thông tin điện tử Chính phủ.', copied: 'Đã sao chép',
}
export function shellMessages(language: Language): Record<keyof typeof en, string> {
  return language === 'en' ? en : vi
}
