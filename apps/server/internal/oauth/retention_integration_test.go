//go:build integration

package oauth

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestRetention_DeletesByRule(t *testing.T) {
	db := consentPostgres(t)
	ctx := t.Context()
	now := time.Now().UTC()
	_, err := db.Exec(ctx, `
 INSERT INTO oauth_request(handle_hash,browser_hash,client_id,redirect_uri,state,code_challenge,resource,scope,expires_at)
 SELECT repeat(x,64),repeat('f',64),'fixture','http://localhost/cb','','challenge','resource','mcp:read',$1::timestamptz + delta
 FROM (VALUES ('a', interval '-1 second'),('b', interval '1 hour')) v(x,delta);
 INSERT INTO oauth_grant(id,account_id,client_id,scope,resource,retirement_anchor,revoked_at) OVERRIDING SYSTEM VALUE VALUES
 (1,7,'fixture','mcp:read','resource',$1::timestamptz,$1::timestamptz - interval '2 days'),
 (3,7,'fixture','mcp:read','resource',$1::timestamptz - interval '31 days',$1::timestamptz - interval '31 days'),
 (2,7,'fixture','mcp:read','resource',$1::timestamptz,NULL);
 INSERT INTO oauth_code(code_hash,grant_id,account_id,client_id,redirect_uri,resource,code_challenge,scope,auth_epoch,expires_at)
 VALUES (repeat('c',64),2,7,'fixture','http://localhost/cb','resource','challenge','mcp:read',3,$1::timestamptz - interval '2 days'),
 (repeat('d',64),2,7,'fixture','http://localhost/cb','resource','challenge','mcp:read',3,$1::timestamptz - interval '1 hour');
 INSERT INTO oauth_token(token_hash,kind,grant_id,account_id,client_id,family_id,resource,scope,auth_epoch,expires_at,family_expires_at)
 VALUES (repeat('e',64),'access',1,7,'fixture',repeat('e',64),'resource','mcp:read',3,$1::timestamptz + interval '1 day',$1::timestamptz + interval '1 day'),
 (repeat('f',64),'access',2,7,'fixture',repeat('f',64),'resource','mcp:read',3,$1::timestamptz - interval '2 days',$1::timestamptz - interval '2 days'),
 (repeat('0',64),'access',2,7,'fixture',repeat('0',64),'resource','mcp:read',3,$1::timestamptz + interval '1 day',$1::timestamptz + interval '1 day');`, pgx.QueryExecModeSimpleProtocol, now)
	if err != nil {
		t.Fatal(err)
	}
	retentionPass(ctx, store.New(db), now, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tt := range []struct {
		table, column, key string
		exists             bool
	}{
		{"oauth_request", "handle_hash", "a", false}, {"oauth_request", "handle_hash", "b", true},
		{"oauth_code", "code_hash", "c", false}, {"oauth_code", "code_hash", "d", true},
		{"oauth_token", "token_hash", "e", false}, {"oauth_token", "token_hash", "f", false}, {"oauth_token", "token_hash", "0", true},
	} {
		var exists bool
		if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+tt.table+" WHERE "+tt.column+"=repeat($1,64))", tt.key).Scan(&exists); err != nil || exists != tt.exists {
			t.Fatalf("%s %s exists=%t err=%v", tt.table, tt.key, exists, err)
		}
	}
	var retired bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM oauth_grant WHERE id=3)`).Scan(&retired); err != nil || retired {
		t.Fatalf("retired grant exists=%t err=%v", retired, err)
	}
}
