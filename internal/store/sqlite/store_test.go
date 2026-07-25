package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	sqlitestore "github.com/mewisme/discloud-go/internal/store/sqlite"
	"github.com/mewisme/discloud-go/internal/store/storetest"
)

func TestStoreContract(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "contract.db")
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=foreign_keys(1)"
	st, err := sqlitestore.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	storetest.Run(t, st)
}

func TestReopenPersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "reopen.db")
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=foreign_keys(1)"
	st, err := sqlitestore.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	st2, err := sqlitestore.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if err := st2.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}
