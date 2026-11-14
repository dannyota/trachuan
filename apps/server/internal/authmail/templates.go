package authmail

import (
	"errors"
	"strings"

	"danny.vn/trachuan/apps/server/internal/accountemail"
)

type Payload struct {
	To    string `json:"to"`
	Token string `json:"token,omitempty"`
}
type Message struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

func Render(publicURL, kind string, payload Payload) (Message, error) {
	address, err := accountemail.Parse(payload.To)
	if err != nil || address != payload.To {
		return Message{}, errors.New("invalid mail recipient")
	}
	type template struct {
		subject, vi, en string
		link            bool
	}
	templates := map[string]template{
		"registration":            {"Xác minh email / Verify email", "Mở liên kết để xác minh email và tạo mật khẩu.", "Open the link to verify your email and create a password.", true},
		"account_exists":          {"Tài khoản đã có / Account exists", "Email này đã có tài khoản. Hãy đăng nhập hoặc đặt lại mật khẩu.", "This email already has an account. Sign in or reset your password.", false},
		"add_password":            {"Tạo mật khẩu / Set password", "Mở liên kết để tạo mật khẩu cho tài khoản.", "Open the link to set a password for your account.", true},
		"reset":                   {"Đặt lại mật khẩu / Reset password", "Mở liên kết để đặt lại mật khẩu. Cài đặt xác thực hai bước không thay đổi.", "Open the link to reset your password. Two-step verification settings are unchanged.", true},
		"password_changed":        {"Đã đổi mật khẩu / Password changed", "Mật khẩu của bạn đã được tạo hoặc thay đổi.", "Your password was set or changed.", false},
		"totp_enrolled":           {"Đã bật xác thực / Verification enabled", "Xác thực hai bước đã được bật hoặc thay thế.", "Two-step verification was enabled or replaced.", false},
		"totp_removed":            {"Đã tắt xác thực / Verification removed", "Xác thực hai bước đã được tắt.", "Two-step verification was removed.", false},
		"recovery_codes_replaced": {"Đã đổi mã dự phòng / Recovery codes replaced", "Các mã khôi phục đã được thay thế. Các mã cũ không còn dùng được.", "Your recovery codes were replaced. Previous codes no longer work.", false},
	}
	t, ok := templates[kind]
	if !ok {
		return Message{}, errors.New("unknown mail kind")
	}
	text := t.vi + "\n\nEnglish\n" + t.en
	if t.link {
		if payload.Token == "" || strings.ContainsAny(payload.Token, "\r\n#? /\\") {
			return Message{}, errors.New("invalid mail link token")
		}
		text += "\n\n" + strings.TrimSuffix(publicURL, "/") + "/link#" + payload.Token
	}
	text += "\n\ntrachuan.vn\n"
	return Message{To: address, Subject: t.subject, Text: text}, nil
}
