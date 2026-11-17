package publicsummary

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func postgresConfig(template, name string) (*pgx.ConnConfig, error) {
	poolConfig, err := pgxpool.ParseConfig(strings.ReplaceAll(template, "{database}", name))
	if err != nil {
		return nil, errors.New("invalid public corpus database configuration")
	}
	cfg := poolConfig.ConnConfig
	if cfg.ConnectTimeout <= 0 || cfg.ConnectTimeout > 2*time.Second {
		cfg.ConnectTimeout = 2 * time.Second
	}
	cfg.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.RuntimeParams["statement_timeout"] = "2000"
	return cfg, nil
}

func OpenPostgres(template string) func(context.Context, string) (Pool, error) {
	return func(ctx context.Context, name string) (Pool, error) {
		cfg, err := postgresConfig(template, name)
		if err != nil {
			return nil, err
		}
		life, cancel := context.WithCancel(ctx)
		p := &postgresPool{config: cfg, ctx: life, cancel: cancel}
		if err = p.connect(life); err != nil {
			cancel()
			return nil, fmt.Errorf("connect public corpus: %w", err)
		}
		return p, nil
	}
}

type postgresPool struct {
	mu         sync.Mutex
	config     *pgx.ConnConfig
	conn       *pgx.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	disconnect context.CancelFunc
}

type ownedConn struct {
	net.Conn
	stop func() bool
}

func (c *ownedConn) Close() error { c.stop(); return c.Conn.Close() }

func (p *postgresPool) connect(ctx context.Context) error {
	life, disconnect := context.WithCancel(p.ctx)
	cfg := p.config.Copy()
	dial := cfg.DialFunc
	cfg.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		work, cancel := context.WithTimeout(ctx, 2*time.Second)
		stop := context.AfterFunc(life, cancel)
		defer func() { stop(); cancel() }()
		if err := life.Err(); err != nil {
			return nil, err
		}
		conn, err := dial(work, network, address)
		if err != nil {
			return nil, err
		}
		closeOnCancel := context.AfterFunc(life, func() { _ = conn.Close() })
		return &ownedConn{Conn: conn, stop: closeOnCancel}, nil
	}
	work, cancel := context.WithTimeout(ctx, 2*time.Second)
	stop := context.AfterFunc(life, cancel)
	defer func() { stop(); cancel() }()
	conn, err := pgx.ConnectConfig(work, cfg)
	if err != nil {
		disconnect()
		return err
	}
	p.conn, p.disconnect = conn, disconnect
	return nil
}

func (p *postgresPool) work(ctx context.Context) (context.Context, func()) {
	work, cancel := context.WithTimeout(ctx, 2*time.Second)
	stop := func() bool { return false }
	if p.ctx != nil {
		stop = context.AfterFunc(p.ctx, cancel)
	}
	return work, func() { stop(); cancel() }
}

func (p *postgresPool) acquire(ctx context.Context) (*pgx.Conn, error) {
	if !p.mu.TryLock() {
		return nil, ErrBusy
	}
	if p.config == nil {
		p.mu.Unlock()
		return nil, ErrBusy
	}
	if err := p.ctx.Err(); err != nil {
		p.mu.Unlock()
		return nil, ErrUnavailable
	}
	if p.conn == nil || p.conn.IsClosed() {
		p.dropConnection()
		if err := p.connect(ctx); err != nil {
			p.mu.Unlock()
			return nil, fmt.Errorf("reconnect public corpus: %w", err)
		}
	}
	return p.conn, nil
}

func (p *postgresPool) dropConnection() {
	if p.disconnect != nil {
		p.disconnect()
	}
	if p.conn != nil {
		_ = p.conn.PgConn().Conn().Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = p.conn.Close(ctx)
		p.conn = nil
	}
}

func (p *postgresPool) Close() {
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dropConnection()
}
