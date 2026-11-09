package build

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func toolEnvironment(connection string) ([]string, string, error) {
	conn, err := pgx.ParseConfig(connection)
	if err != nil {
		return nil, "", fmt.Errorf("parse builder connection: %w", err)
	}
	u, err := url.Parse(connection)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return nil, "", fmt.Errorf("builder URL must be a PostgreSQL URL")
	}
	env := make([]string, 0, len(os.Environ())+8)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PG") {
			env = append(env, entry)
		}
	}
	env = append(env, "PGHOST="+conn.Host, "PGPORT="+strconv.Itoa(int(conn.Port)), "PGUSER="+conn.User, "PGPASSWORD="+conn.Password)
	if ssl := u.Query().Get("sslmode"); ssl != "" {
		env = append(env, "PGSSLMODE="+ssl)
	}
	return env, conn.Password, nil
}
