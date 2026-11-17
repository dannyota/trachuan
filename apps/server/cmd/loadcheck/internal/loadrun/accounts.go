package loadrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type secureJar struct{ http.CookieJar }

func secureURL(u *url.URL) *url.URL                         { v := *u; v.Scheme = "https"; return &v }
func (j secureJar) Cookies(u *url.URL) []*http.Cookie       { return j.CookieJar.Cookies(secureURL(u)) }
func (j secureJar) SetCookies(u *url.URL, c []*http.Cookie) { j.CookieJar.SetCookies(secureURL(u), c) }

type account struct {
	client     *http.Client
	csrf, addr string
}

func accounts(ctx context.Context, c Config) ([]account, error) {
	sessionsOnly := len(c.Sessions) == 3 && len(c.Emails) == 0 && c.Google == ""
	googleOnly := len(c.Sessions) == 0 && len(c.Emails) == 3 && c.Google != ""
	if !sessionsOnly && !googleOnly {
		return nil, errors.New("provide three sessions or three emails and fake Google")
	}
	var out []account
	for i := 0; i < 3; i++ {
		jar, jarErr := cookiejar.New(nil)
		if jarErr != nil {
			return nil, fmt.Errorf("create cookie jar: %w", jarErr)
		}
		cl := &http.Client{Jar: secureJar{jar}, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		if len(c.Sessions) == 3 {
			b, err := os.ReadFile(c.Sessions[i])
			if err != nil {
				return nil, errors.New("read session file failed")
			}
			value := strings.TrimSpace(string(b))
			if value == "" {
				return nil, errors.New("empty session file")
			}
			for _, origin := range []string{c.API, c.Pages} {
				u, err := url.Parse(origin)
				if err != nil {
					return nil, errors.New("invalid origin")
				}
				cl.Jar.SetCookies(u, []*http.Cookie{{Name: "__Host-session", Value: value, Path: "/", Secure: true}})
			}
		} else {
			if err := login(ctx, cl, c, c.Emails[i], address(i+100, 1000)); err != nil {
				return nil, err
			}
			public, err := url.Parse(c.PublicURL)
			if err != nil {
				return nil, errors.New("invalid public origin")
			}
			cookies := cl.Jar.Cookies(public)
			for _, origin := range []string{c.API, c.Pages} {
				u, err := url.Parse(origin)
				if err != nil {
					return nil, errors.New("invalid origin")
				}
				cl.Jar.SetCookies(u, cookies)
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.API+"/api/v1/me", nil)
		if err != nil {
			return nil, errors.New("invalid API origin")
		}
		req.Header.Set("X-Forwarded-For", address(i+100, 1000))
		resp, err := cl.Do(req)
		if err != nil {
			return nil, errors.New("account lookup failed")
		}
		var body struct {
			CSRF string `json:"csrf_token"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != 200 || body.CSRF == "" {
			return nil, errors.New("account lookup rejected")
		}
		out = append(out, account{client: cl, csrf: body.CSRF, addr: address(i+100, 1000)})
	}
	return out, nil
}
func login(ctx context.Context, cl *http.Client, c Config, email, addr string) error {
	u, err := url.Parse(c.Google)
	if err != nil || u.Hostname() == "" {
		return errors.New("invalid fake Google origin")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("fake Google must be loopback")
	}
	request := func(method, target string, body io.Reader) (*http.Response, error) {
		req, e := http.NewRequestWithContext(ctx, method, target, body)
		if e != nil {
			return nil, e
		}
		req.Header.Set("X-Forwarded-For", addr)
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return cl.Do(req)
	}
	r, err := request("GET", c.PublicURL+"/api/v1/auth/google/start?next=%2Faccount", nil)
	if err != nil {
		return errors.New("fake Google start failed")
	}
	loc := r.Header.Get("Location")
	_ = r.Body.Close()
	if r.StatusCode != 302 || !strings.HasPrefix(loc, c.Google+"/") {
		return errors.New("fake Google start rejected")
	}
	r, err = request("GET", loc, nil)
	if err != nil {
		return errors.New("fake Google form failed")
	}
	doc, err := html.Parse(io.LimitReader(r.Body, 1<<20))
	_ = r.Body.Close()
	if err != nil || r.StatusCode != 200 {
		return errors.New("fake Google form rejected")
	}
	values := url.Values{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			if attrs["type"] == "hidden" {
				values.Add(attrs["name"], attrs["value"])
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	values.Set("email", email)
	values.Set("email_verified", "on")
	r, err = request("POST", c.Google+"/authorize", strings.NewReader(values.Encode()))
	if err != nil {
		return errors.New("fake Google authorization failed")
	}
	loc = r.Header.Get("Location")
	_ = r.Body.Close()
	if r.StatusCode != 302 || !strings.HasPrefix(loc, c.PublicURL+"/") {
		return errors.New("fake Google authorization rejected")
	}
	r, err = request("GET", loc, nil)
	if err != nil {
		return errors.New("fake Google callback failed")
	}
	_ = r.Body.Close()
	if r.StatusCode != 302 || r.Header.Get("Location") != "/account" {
		return errors.New("fake Google callback rejected")
	}
	r, err = request("GET", c.PublicURL+"/account", nil)
	if err != nil {
		return errors.New("account landing failed")
	}
	_ = r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return errors.New("account landing rejected")
	}
	return nil
}
