import type { Language } from './index'

export const en = {
  title: 'Account', created: 'created', signOut: 'Sign out', sessions: 'Sessions', sessionsDescription: 'Browsers signed in to your account.', sessionsEmpty: 'No sessions found.', thisBrowser: 'This browser', otherSession: 'Other session', current: 'This session', signedIn: 'Signed in', lastSeen: 'Last active', revoke: 'Revoke',
  tokens: 'Personal access tokens', tokensDescription: 'For MCP apps that do not support sign-in. A token is shown only once when created.', createToken: 'Create token', tokensEmpty: 'You have not created any tokens.', expires: 'Expires', lastUsed: 'Last used', revoked: 'Revoked',
  grants: 'Authorized AI applications', grantsDescription: 'Assistants with read-only access to search data through MCP.', grantsEmpty: 'You have not authorized any applications.', granted: 'Granted', revokeGrant: 'Revoke access',
  reports: 'Reports', reportsDescription: 'Issues you or your assistant reported about documents or search results.', reportsEmpty: 'No reports yet. When your assistant finds an error, it can report it directly through Tra Chuẩn.',
  deleteAccount: 'Delete account', deleteButton: 'Delete account…', deleteDescription: 'Your account is locked immediately and permanently deleted after 7 days, along with all sessions, tokens and grants.', loading: 'Loading',
}
export const vi: Record<keyof typeof en, string> = {
  title: 'Tài khoản', created: 'tạo ngày', signOut: 'Đăng xuất', sessions: 'Phiên đăng nhập', sessionsDescription: 'Các trình duyệt đã đăng nhập vào tài khoản của bạn.', sessionsEmpty: 'Không có phiên đăng nhập nào.', thisBrowser: 'Trình duyệt này', otherSession: 'Phiên khác', current: 'Phiên này', signedIn: 'Đăng nhập', lastSeen: 'Hoạt động lần cuối', revoke: 'Thu hồi',
  tokens: 'Mã truy cập cá nhân', tokensDescription: 'Dùng cho ứng dụng MCP không hỗ trợ đăng nhập. Mã chỉ hiện một lần khi tạo.', createToken: 'Tạo mã mới', tokensEmpty: 'Bạn chưa tạo mã nào.', expires: 'Hết hạn', lastUsed: 'Dùng lần cuối', revoked: 'Đã thu hồi',
  grants: 'Ứng dụng AI đã cấp quyền', grantsDescription: 'Các trợ lý có quyền chỉ đọc dữ liệu tra cứu qua MCP.', grantsEmpty: 'Bạn chưa cấp quyền cho ứng dụng nào.', granted: 'Cấp quyền', revokeGrant: 'Thu hồi quyền',
  reports: 'Báo lỗi của bạn', reportsDescription: 'Lỗi bạn hoặc trợ lý của bạn đã báo về văn bản hay kết quả tra cứu.', reportsEmpty: 'Chưa có báo lỗi nào. Khi trợ lý phát hiện chỗ sai, nó có thể báo trực tiếp qua Tra Chuẩn.',
  deleteAccount: 'Xóa tài khoản', deleteButton: 'Xóa tài khoản…', deleteDescription: 'Tài khoản bị khóa ngay và bị xóa hẳn sau 7 ngày, cùng mọi phiên, mã truy cập và quyền đã cấp.', loading: 'Đang tải',
}
export function accountMessages(language: Language): Record<keyof typeof en, string> {
  return language === 'en' ? en : vi
}
