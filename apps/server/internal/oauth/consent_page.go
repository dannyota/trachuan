package oauth

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

const consentStyles = `@font-face{font-family:"Be Vietnam Pro";src:url("/fonts/be-vietnam-pro-var.woff2") format("woff2");font-weight:100 900;font-display:swap}
@font-face{font-family:"Roboto Mono";src:url("/fonts/roboto-mono-var.woff2") format("woff2");font-weight:100 900;font-display:swap}
:root{--page:#FFFFFF;--surface:#F6F7F9;--card:#FFFFFF;--ink:#14171C;--ink-soft:#444B55;--muted:#59606B;--line:#E3E6EA;--line-strong:#C6CBD2;--primary:#1E3A8A;--primary-fill:#1E3A8A;--danger:#B42318;--danger-line:#F3C7C1;--danger-bg:#FEF3F2;--note:#F1F4FA;--note-warn:#FDF6E9;--highlight:#FFF1A8;--highlight-ink:#14171C}
@media (prefers-color-scheme: dark){:root{--page:#111418;--surface:#181C22;--card:#161A20;--ink:#E6E8EB;--ink-soft:#C4C9D1;--muted:#A0A7B2;--line:#2C323B;--line-strong:#3A414C;--primary:#9DB4F5;--primary-fill:#3451B2;--danger:#E5604F;--danger-line:#5A2620;--danger-bg:#2A1412;--note:#19213A;--note-warn:#2B2110;--highlight:#4A3F12;--highlight-ink:#F5ECC0}}
*{box-sizing:border-box}html{background:var(--page)}html{background:var(--surface)}body{margin:0;min-height:100vh;padding:2rem 1rem;background:var(--surface);color:var(--ink);font:16px/1.6 "Be Vietnam Pro",system-ui,sans-serif}main{max-width:34rem;margin:0 auto;padding:2rem;background:var(--card);border:1px solid var(--line);border-radius:12px;overflow-wrap:anywhere}.top{display:flex;justify-content:space-between;align-items:center;gap:1rem;margin-bottom:2rem}.brand{font-weight:700;color:var(--primary)}.toggle{display:inline-flex;align-items:center;justify-content:center;min-width:44px;min-height:44px;font-size:14px}a{color:var(--primary)}h1{font-size:1.625rem;line-height:1.3;font-weight:700;margin:0 0 1rem}dl{margin:1.5rem 0}dt{font-size:14px;color:var(--muted)}dd{margin:0 0 1rem}dd bdi,code{font-family:"Roboto Mono",ui-monospace,monospace;font-variant-ligatures:none;overflow-wrap:anywhere}.note{border:1px solid var(--line);border-radius:8px;background:var(--note-warn);color:var(--ink-soft);padding:.75rem 1rem;margin:1rem 0}.note strong{display:block;font-size:14px}.scope{display:inline-block;background:var(--highlight);color:var(--highlight-ink);border-radius:6px;padding:.15rem .5rem}.scope-label,.error-code{font-size:14px;color:var(--muted)}.actions{display:flex;gap:.75rem;margin-top:1.5rem}button,.button{display:inline-flex;align-items:center;justify-content:center;min-height:44px;padding:.6rem 1.25rem;border:1px solid var(--primary-fill);border-radius:8px;font:inherit;font-weight:600;text-decoration:none;cursor:pointer}.primary{background:var(--primary-fill);color:#FFFFFF}.secondary{background:var(--card);border-color:var(--line-strong);color:var(--ink)}a:focus-visible,button:focus-visible{outline:3px solid var(--primary);outline-offset:3px}@media(max-width:390px){body{padding:1rem}main{padding:1.25rem}.actions{flex-direction:column}.actions button{width:100%}}`

func WriteRateLimitedPage(w http.ResponseWriter, r *http.Request, retryAfter time.Duration) {
	seconds := max(1, int64(math.Ceil(retryAfter.Seconds())))
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	newConsentPages("").failure(w, r, http.StatusTooManyRequests, "temporarily_unavailable", "", "")
}

type consentPages struct {
	base      string
	templates *template.Template
}

func newConsentPages(base string) *consentPages {
	return &consentPages{base: base, templates: template.Must(template.ParseFS(consentTemplates, "templates/*.html"))}
}

//go:embed templates/consent.html templates/error.html
var consentTemplates embed.FS

type consentPageData struct {
	Lang, Title, Name, ClientHost, RedirectHost, Token, Action, Toggle, ToggleLabel string
	ScopeLabel, Prompt, Approve, Deny, DCRWarning, LoopbackWarning, NoteLabel       string
	Error, Message, Return                                                          string
	Style                                                                           template.CSS
	RedirectLabel, Heading                                                          string
}

func (p *consentPages) consent(w http.ResponseWriter, r *http.Request, client Client, row store.OauthRequest, handle, token string) {
	en := r.URL.Query().Get("lang") == "en"
	data := consentPageData{Lang: "vi", Title: "Cho phép truy cập · Tra Chuẩn", Name: client.Name, Token: token, Action: "/oauth/consent/" + handle, Toggle: "?lang=en", ToggleLabel: "English", RedirectLabel: "Máy chủ nhận quyền", ScopeLabel: "Quyền truy cập", NoteLabel: "Lưu ý", Prompt: "Cho phép ứng dụng truy cập tài khoản Tra Chuẩn của bạn?", Approve: "Cho phép", Deny: "Từ chối"}
	if en {
		data.Lang = "en"
		data.Title = "Authorize access · Tra Chuẩn"
		data.Toggle = "?lang=vi"
		data.ToggleLabel = "Tiếng Việt"
		data.RedirectLabel = "Redirect host"
		data.ScopeLabel = "Scope"
		data.NoteLabel = "Note"
		data.Prompt = "Allow this app to access your Tra Chuẩn account?"
		data.Approve = "Allow"
		data.Deny = "Deny"
		data.Action += "?lang=en"
	}
	redirect, _ := url.Parse(row.RedirectUri)
	data.RedirectHost = redirect.Host
	if client.Kind == "cimd" {
		id, err := url.Parse(client.ID)
		if err == nil {
			data.ClientHost = id.Host
		}
	}
	if client.Kind == "dcr" {
		data.DCRWarning = "Tên ứng dụng chưa được xác minh."
		if en {
			data.DCRWarning = "The client name is unverified."
		}
	}
	if redirect.Scheme == "http" {
		data.LoopbackWarning = "Một ứng dụng trên máy tính này sẽ nhận quyền truy cập. Danh tính của ứng dụng chưa được xác minh."
		if en {
			data.LoopbackWarning = "An application on this computer will receive access. Its identity is not verified."
		}
	}
	p.render(w, 200, "consent.html", data)
}
func (p *consentPages) failure(w http.ResponseWriter, r *http.Request, status int, code, redirect, state string) {
	en := r.URL.Query().Get("lang") == "en"
	data := consentPageData{Lang: "vi", Title: "Không thể cấp quyền truy cập · Tra Chuẩn", Error: code, Message: "Yêu cầu không hợp lệ hoặc đã hết hạn. Hãy thử kết nối lại từ ứng dụng.", Return: "Quay lại ứng dụng", Toggle: "?lang=en", ToggleLabel: "English"}
	if en {
		data.Lang = "en"
		data.Title = "Cannot authorize access · Tra Chuẩn"
		data.Message = "The request is invalid or expired. Connect again from the application."
		data.Return = "Return to application"
		data.Toggle = "?lang=vi"
		data.ToggleLabel = "Tiếng Việt"
	}
	if code == "temporarily_unavailable" {
		data.Message = "Dịch vụ tạm thời không khả dụng. Vui lòng thử lại."
		if en {
			data.Message = "The service is temporarily unavailable. Please try again."
		}
	}
	callbackCode := code
	if code == "grant_limit" {
		callbackCode = "access_denied"
		data.Error = callbackCode
		data.Message = "Bạn đã đạt giới hạn 20 ứng dụng. Hãy thu hồi quyền của một ứng dụng trước."
		if en {
			data.Message = "You have reached the limit of 20 applications. Please revoke a grant first."
		}
	}
	if redirect != "" {
		data.Action = consentResultURL(redirect, p.base, "", callbackCode, state)
	}
	p.render(w, status, "error.html", data)
}
func (p *consentPages) render(w http.ResponseWriter, status int, name string, data consentPageData) {
	consentHeaders(w)
	data.Style = template.CSS(consentStyles)
	data.Heading = strings.TrimSuffix(data.Title, " · Tra Chuẩn")
	var body bytes.Buffer
	if err := p.templates.ExecuteTemplate(&body, name, data); err != nil {
		w.WriteHeader(503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}
func consentHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
	digest := sha256.Sum256([]byte(consentStyles))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self' 'sha256-"+base64.StdEncoding.EncodeToString(digest[:])+"'; font-src 'self'; frame-ancestors 'none'")
}
