import type { Language } from './index'

const en = {
  signIn: 'Sign in', google: 'Continue with Google', email: 'Email', password: 'Password', signup: 'Create an account', reset: 'Reset password', send: 'Send link', mail: 'Check your mail. If this address is eligible, a link will arrive shortly.',
  factorTitle: 'Two-step verification', code: 'Authenticator code', recovery: 'Recovery code', useRecovery: 'Use a recovery code', useCode: 'Use an authenticator code', verify: 'Verify',
  setPassword: 'Set your password', newPassword: 'New password', currentPassword: 'Current password', passwordHelp: 'Use 15 to 128 characters. Common passwords are not accepted.', savePassword: 'Save password', saved: 'Password saved. Sign in with your new password.', linkSignIn: 'Sign in, then open the link from your mail again.', linkInvalid: 'This link has expired or has already been used. Request a new link.',
  security: 'Security', addPassword: 'Add a password by email', changePassword: 'Change password', setup: 'Set up two-step verification', secret: 'Setup secret', qr: 'Scan this QR code with your authenticator app', confirm: 'Confirm authenticator', remove: 'Remove authenticator', replaceCodes: 'Replace recovery codes', recoveryOnce: 'Save these recovery codes now. They will not be shown again.', copy: 'Copy recovery codes', download: 'Download recovery codes', close: 'Close', reauth: 'Confirm your password to continue', cancel: 'Cancel', success: 'Security settings updated.', enrollmentOff: 'Authenticator setup is unavailable.', loading: 'Loading',
  loginFailed: 'Sign-in failed. Check your email and password.', factorFailed: 'The code is invalid or has already been used.', reauthFailed: 'The password is incorrect.', rateLimited: 'Too many attempts. Please try again later.', passwordRejected: 'Choose a less common password with 15 to 128 characters.', generic: 'Something went wrong. Please try again.', accountChanged: 'Your account changed. Please review and try again.',
}
const vi: Record<keyof typeof en, string> = {
  signIn: 'Đăng nhập', google: 'Tiếp tục với Google', email: 'Email', password: 'Mật khẩu', signup: 'Tạo tài khoản', reset: 'Đặt lại mật khẩu', send: 'Gửi liên kết', mail: 'Kiểm tra email. Nếu địa chỉ đủ điều kiện, bạn sẽ sớm nhận được liên kết.',
  factorTitle: 'Xác minh hai bước', code: 'Mã ứng dụng xác thực', recovery: 'Mã khôi phục', useRecovery: 'Dùng mã khôi phục', useCode: 'Dùng mã ứng dụng xác thực', verify: 'Xác minh',
  setPassword: 'Đặt mật khẩu', newPassword: 'Mật khẩu mới', currentPassword: 'Mật khẩu hiện tại', passwordHelp: 'Dùng từ 15 đến 128 ký tự. Không dùng mật khẩu phổ biến.', savePassword: 'Lưu mật khẩu', saved: 'Đã lưu mật khẩu. Hãy đăng nhập bằng mật khẩu mới.', linkSignIn: 'Đăng nhập, rồi mở lại liên kết trong email.', linkInvalid: 'Liên kết đã hết hạn hoặc đã được sử dụng. Hãy yêu cầu liên kết mới.',
  security: 'Bảo mật', addPassword: 'Thêm mật khẩu qua email', changePassword: 'Đổi mật khẩu', setup: 'Thiết lập xác minh hai bước', secret: 'Khóa thiết lập', qr: 'Quét mã QR này bằng ứng dụng xác thực', confirm: 'Xác nhận ứng dụng xác thực', remove: 'Gỡ ứng dụng xác thực', replaceCodes: 'Thay mã khôi phục', recoveryOnce: 'Lưu các mã khôi phục ngay. Các mã sẽ không hiển thị lại.', copy: 'Sao chép mã khôi phục', download: 'Tải mã khôi phục', close: 'Đóng', reauth: 'Xác nhận mật khẩu để tiếp tục', cancel: 'Hủy', success: 'Đã cập nhật cài đặt bảo mật.', enrollmentOff: 'Chưa thể thiết lập ứng dụng xác thực.', loading: 'Đang tải',
  loginFailed: 'Đăng nhập thất bại. Hãy kiểm tra email và mật khẩu.', factorFailed: 'Mã không hợp lệ hoặc đã được sử dụng.', reauthFailed: 'Mật khẩu không đúng.', rateLimited: 'Quá nhiều lần thử. Vui lòng thử lại sau.', passwordRejected: 'Chọn mật khẩu ít phổ biến hơn, từ 15 đến 128 ký tự.', generic: 'Đã xảy ra lỗi. Vui lòng thử lại.', accountChanged: 'Tài khoản đã thay đổi. Hãy kiểm tra và thử lại.',
}
export function authMessages(language: Language): Record<keyof typeof en, string> { return language === 'en' ? en : vi }
export function authError(language: Language, code?: string, status?: number): string {
  const labels = authMessages(language)
  if (status === 429) return labels.rateLimited
  switch (code) {
    case 'login_failed': return labels.loginFailed
    case 'reauth_failed': return labels.reauthFailed
    case 'factor_failed': return labels.factorFailed
    case 'password_rejected': return labels.passwordRejected
    case 'link_invalid': return labels.linkInvalid
    case 'account_changed': return labels.accountChanged
    default: return labels.generic
  }
}
