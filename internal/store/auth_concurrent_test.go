package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/auth"
)

// Concurrent first-user → admin under pg_advisory_xact_lock.
// Requires DATABASE_URL and an empty users table (or DISCLOUD_TEST_WIPE_USERS=1).
func TestCreateUserConcurrentFirstAdmin(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count > 0 {
		if os.Getenv("DISCLOUD_TEST_WIPE_USERS") != "1" {
			t.Skip("users table not empty; set DISCLOUD_TEST_WIPE_USERS=1 to truncate and run")
		}
		if _, err := st.pool.Exec(ctx, `DELETE FROM sessions; DELETE FROM users`); err != nil {
			t.Fatal(err)
		}
	}

	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatal(err)
	}
	const n = 16
	results := make([]User, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	base := time.Now().UnixNano()
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = st.CreateUser(ctx,
				fmt.Sprintf("id-%d-%d", base, i),
				fmt.Sprintf("c%d-%d@ex.com", base, i),
				hash)
		}(i)
	}
	wg.Wait()

	admins := 0
	for i, u := range results {
		if errs[i] != nil {
			t.Fatalf("CreateUser %d: %v", i, errs[i])
		}
		if u.Role == RoleAdmin {
			admins++
		}
	}
	if admins != 1 {
		t.Fatalf("admin count = %d, want 1", admins)
	}
}
