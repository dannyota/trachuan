package retrieval

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"time"
)

type searchCursor struct {
	V               int    `json:"v"`
	Release         string `json:"release"`
	FirstLimit      int    `json:"first_limit"`
	TakenPrimary    int    `json:"taken_primary"`
	TakenNonCurrent int    `json:"taken_non_current"`
	AsOf            Date   `json:"as_of"`
	Digest          string `json:"digest"`
}

func requestDigest(req Request) string {
	inForce := req.InForceOnly == nil || *req.InForceOnly
	// Struct order and normalized, sorted sets give one encoding per request.
	raw, _ := json.Marshal(struct {
		Query                               string
		InForceOnly                         bool
		AsOf                                *Date
		Mode                                Mode
		Arms                                ArmMode
		Nodes, DocTypes, Issuers, Documents []string
	}{req.Query, inForce, req.AsOf, req.Mode, req.Arms, req.Nodes, req.DocTypes, req.Issuers, req.Documents})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func encodeSearchCursor(c searchCursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeSearchCursor(token string) (searchCursor, error) {
	var c searchCursor
	if len(token) > 2048 {
		return c, ErrInvalidRequest
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil {
		return c, ErrInvalidRequest
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil || len(fields) != 7 {
		return c, ErrInvalidRequest
	}
	for _, key := range []string{"v", "release", "first_limit", "taken_primary", "taken_non_current", "as_of", "digest"} {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return c, ErrInvalidRequest
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&c); err != nil {
		return searchCursor{}, ErrInvalidRequest
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return searchCursor{}, ErrInvalidRequest
	}
	digest, err := hex.DecodeString(c.Digest)
	if err != nil || len(digest) != sha256.Size || c.V != 1 || c.Release == "" || c.FirstLimit < 1 || c.FirstLimit > MaxLimit || c.TakenPrimary < 0 || c.TakenPrimary > 150 || c.TakenNonCurrent < 0 || c.TakenNonCurrent > 150 || c.AsOf.Year < 1 || c.AsOf.Year > 9999 {
		return searchCursor{}, ErrInvalidRequest
	}
	return c, nil
}

func (e *Engine) searchRequest(req Request) (Request, searchCursor, error) {
	req, err := normalizeFilters(req)
	if err != nil {
		return Request{}, searchCursor{}, err
	}
	c := searchCursor{V: 1, Release: e.release.Rel, FirstLimit: req.Limit, Digest: requestDigest(req)}
	if c.FirstLimit == 0 {
		c.FirstLimit = FullDefaultLimit
	}
	if req.Cursor != "" {
		decoded, decodeErr := decodeSearchCursor(req.Cursor)
		if decodeErr != nil {
			return Request{}, searchCursor{}, decodeErr
		}
		if decoded.Release != e.release.Rel {
			return Request{}, searchCursor{}, ErrReleaseExpired
		}
		if decoded.Digest != c.Digest {
			return Request{}, searchCursor{}, ErrInvalidRequest
		}
		c = decoded
	} else {
		date, parseErr := time.Parse(time.DateOnly, e.searchDate(req))
		if parseErr != nil {
			return Request{}, searchCursor{}, ErrInvalidRequest
		}
		c.AsOf = Date{date.Year(), date.Month(), date.Day()}
	}
	// Preserve whether the caller requested historical validity; only pin the clock.
	req.Instant = time.Date(c.AsOf.Year, c.AsOf.Month, c.AsOf.Day, 0, 0, 0, 0, time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60))
	return req, c, nil
}
