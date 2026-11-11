package oauth

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"html/template"
	"net/http"
	"net/url"

	"danny.vn/trachuan/apps/server/internal/store"
)

// The fixed stylesheet is the only trusted CSS value used by the templates.
const consentStyles = "body{font:1rem/1.6 system-ui,sans-serif;margin:2rem auto;padding:0 1rem;max-width:38rem;color:#18212b;background:#fff}main{overflow-wrap:anywhere}button{font:inherit;padding:.5rem 1rem;margin:.5rem .5rem .5rem 0;cursor:pointer}a{color:#174ca4}bdi,code{font-size:1em}"

type consentPages struct {
	base      string
	templates *template.Template
}

func newConsentPages(base string) *consentPages {
	// Each handler owns its parsed templates. The embedded files are read only.
	return &consentPages{base: base, templates: template.Must(template.ParseFS(consentTemplates, "templates/*.html"))}
}

//go:embed templates/consent.html templates/error.html
var consentTemplates embed.FS

type consentReturnField struct{ Name, Value string }

type consentPageData struct {
	Lang, Title, Name, ClientHost, RedirectHost, Token, Action, Toggle, ToggleLabel string
	ScopeLabel, Prompt, Approve, Deny, DCRWarning, LoopbackWarning                  string
	Error, Message, Return                                                          string
	ReturnFields                                                                    []consentReturnField
	Style                                                                           template.CSS
	RedirectLabel                                                                   string
}

func (p *consentPages) consent(w http.ResponseWriter, r *http.Request, client Client, row store.OauthRequest, handle, token string) {
	en := r.URL.Query().Get("lang") == "en"
	data := consentPageData{Lang: "vi", Title: "Cho phép truy cập", Name: client.Name, Token: token, Action: "/oauth/consent/" + handle, Toggle: "?lang=en", ToggleLabel: "English", RedirectLabel: "Máy chủ nhận quyền", ScopeLabel: "Quyền truy cập", Prompt: "Cho phép ứng dụng truy cập tài khoản của bạn?", Approve: "Cho phép", Deny: "Từ chối"}
	if en {
		data.Lang = "en"
		data.Title = "Authorize access"
		data.Toggle = "?lang=vi"
		data.ToggleLabel = "Tiếng Việt"
		data.RedirectLabel = "Redirect host"
		data.ScopeLabel = "Scope"
		data.Prompt = "Allow this application to access your account?"
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
	p.render(w, 200, "consent.html", data, row.RedirectUri)
}
func (p *consentPages) failure(w http.ResponseWriter, r *http.Request, status int, code, redirect, state string) {
	en := r.URL.Query().Get("lang") == "en"
	data := consentPageData{Lang: "vi", Title: "Không thể cấp quyền truy cập", Error: code, Message: "Yêu cầu không hợp lệ hoặc đã hết hạn. Hãy thử kết nối lại từ ứng dụng.", Return: "Quay lại ứng dụng", Toggle: "?lang=en", ToggleLabel: "English"}
	if en {
		data.Lang = "en"
		data.Title = "Cannot authorize access"
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
		target, _ := url.Parse(consentResultURL(redirect, p.base, "", callbackCode, state))
		for key, values := range target.Query() {
			for _, value := range values {
				data.ReturnFields = append(data.ReturnFields, consentReturnField{key, value})
			}
		}
		target.RawQuery = ""
		data.Action = target.String()
	}
	p.render(w, status, "error.html", data, redirect)
}
func (p *consentPages) render(w http.ResponseWriter, status int, name string, data consentPageData, redirect string) {
	consentHeaders(w, redirect)
	data.Style = template.CSS(consentStyles)
	var body bytes.Buffer
	if err := p.templates.ExecuteTemplate(&body, name, data); err != nil {
		w.WriteHeader(503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}
func consentHeaders(w http.ResponseWriter, redirect string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	formAction := "'self'"
	if u, ok := redirectURI(redirect); ok {
		formAction += " " + u.Scheme + "://" + u.Host
	}
	digest := sha256.Sum256([]byte(consentStyles))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self' 'sha256-"+base64.StdEncoding.EncodeToString(digest[:])+"'; frame-ancestors 'none'; form-action "+formAction)
}
