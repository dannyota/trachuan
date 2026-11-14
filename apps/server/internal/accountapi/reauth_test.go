package accountapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type reauthStore struct {
	*fakeStore
	err error
}

func (s reauthStore) CreateWebPersonalToken(context.Context, store.WebPersonalTokenParams) (store.PersonalToken, error) {
	return store.PersonalToken{}, s.err
}

func TestTokens_ReauthContract(t *testing.T) {
	contract, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverage(contract, []string{"/api/v1/me/tokens"})
	for _, factor := range []bool{false, true} {
		t.Run(map[bool]string{false: "proof", true: "factor"}[factor], func(t *testing.T) {
			_, s, v, raw, secret := setup(t)
			now := func() time.Time { return s.now }
			m := &websession.Manager{Store: s, Clock: now, PublicURL: "https://example.invalid"}
			refusal := store.ErrReauthRequired
			if factor {
				refusal = store.ErrFactorRequired
			}
			h := m.Middleware(m.CSRF(New(Config{Store: reauthStore{s, refusal}, Sessions: m, Verifier: v, Limiter: limits.New(now)})))
			w := call(h, raw, secret, "POST", "/api/v1/me/tokens", `{"name":"synthetic"}`)
			expect(t, w, 403)
			if !strings.Contains(w.Body.String(), `"code":"reauth_required"`) || strings.Contains(w.Body.String(), `"factor":true`) != factor {
				t.Fatal(w.Body.String())
			}
			coverage.Check(t, "POST", "/api/v1/me/tokens", w)
		})
	}
}
