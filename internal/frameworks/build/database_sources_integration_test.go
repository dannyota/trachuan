//go:build integration

package build

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/frameworks/registry"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type sourceTestDatabase struct{ conn *pgx.Conn }

func (db sourceTestDatabase) Build(ctx context.Context, req DatabaseRequest, load func(context.Context, *pgx.Conn) error, publish func() error) error {
	if err := load(ctx, db.conn); err != nil {
		return err
	}
	if err := writeFile(filepath.Dir(req.DumpPath), filepath.Base(req.DumpPath), []byte("synthetic dump")); err != nil {
		return err
	}
	return publish()
}

func TestRun_SupplementaryFilesStayInSnapshot(t *testing.T) {
	work := fixture(t)
	source := os.Getenv("FRAMEWORKS_SOURCE_DIR")
	for _, file := range []struct{ name, role string }{{"a-guide.pdf", "guide"}, {"z-change-log.pdf", "change log"}} {
		data := []byte("Invented " + file.role + " document")
		require.NoError(t, os.WriteFile(filepath.Join(source, file.name), data, 0600))
		editRegistry(t, func(reg *registry.Registry) {
			reg.Frameworks[0].Files = append(reg.Frameworks[0].Files, registry.File{Path: file.name, Role: file.role, Format: "pdf", SHA256: digest(data), Size: int64(len(data))})
		})
	}
	ctx := context.Background()
	conn := frameworkDB(t)
	var out bytes.Buffer
	require.NoError(t, Run(ctx, "20990101", &out, fakeEmbedder{}, sourceTestDatabase{conn: conn}))
	require.Contains(t, out.String(), "build complete")
	for _, name := range []string{"a-guide.pdf", "z-change-log.pdf"} {
		original, err := os.ReadFile(filepath.Join(source, name))
		require.NoError(t, err)
		snapshot, err := os.ReadFile(filepath.Join(work, "20990101/snapshot/files", name))
		require.NoError(t, err)
		require.Equal(t, original, snapshot)
	}
	var names []string
	var ids []int64
	var structured, complete bool
	require.NoError(t, conn.QueryRow(ctx, `SELECT array_agg(name ORDER BY id),array_agg(id ORDER BY id),bool_and(text_source='structured'),bool_and(text_complete) FROM source_file`).Scan(&names, &ids, &structured, &complete))
	require.Equal(t, []string{"fw-alpha.json", "fw-beta.json"}, names)
	require.Equal(t, []int64{1, 2}, ids)
	require.True(t, structured)
	require.True(t, complete)
}
