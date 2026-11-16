package serve

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"sort"
)

type projection struct {
	readable, reduced []int64
	allowed           map[int64]bool
	version           string
}

func (e *Engine) project(caller Caller) projection {
	p := projection{allowed: make(map[int64]bool)}
	handles := []string{}
	for _, v := range e.versions {
		readable := caller.Readable != nil && caller.Readable(v.info.Code, v.info.Access)
		p.allowed[v.id] = readable
		if readable {
			p.readable = append(p.readable, v.id)
			handles = append(handles, v.handle)
		} else {
			p.reduced = append(p.reduced, v.id)
		}
	}
	sort.Strings(handles)
	raw, _ := json.Marshal(struct {
		Release string
		Handles []string
	}{e.release, handles})
	p.version = e.sign("frameworks-access", raw)
	return p
}

func (e *Engine) sign(domain string, raw []byte) string {
	mac := hmac.New(sha256.New, e.key)
	_, _ = mac.Write([]byte(domain + "\x00"))
	_, _ = mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
