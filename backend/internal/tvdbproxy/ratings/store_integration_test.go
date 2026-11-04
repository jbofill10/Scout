package ratings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/pkg/dbmigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
)

// openTestStore connects to the PostgreSQL named by DB_HOST/DB_USER/DB_NAME
// (skipping the test without them), migrates it and clears the test's keys.
func openTestStore(t *testing.T) *PostgresStore {
	t.Helper()
	host, user := os.Getenv("DB_HOST"), os.Getenv("DB_USER")
	pass, name := os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME")
	if host == "" || user == "" || name == "" {
		t.Skip("DB_HOST/DB_USER/DB_NAME not set; skipping ratings store integration test")
	}
	dsn := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable", host, user, pass, name)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, dbmigrate.RunWithConnStr(ctx, dsn, slog.Default()))
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM RatingsCache WHERE key LIKE 'test:%'`)
	require.NoError(t, err)
	store := NewPostgresStore(db)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM RatingsCache WHERE key LIKE 'test:%'`)
		_ = store.Close()
	})
	return store
}

func TestPostgresStoreRoundTrips(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	live := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	old := time.Now().Add(-200 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	require.NoError(t, store.Put(ctx, map[string]Entry{
		"test:a": {Value: json.RawMessage(`[{"source":"rotten_tomatoes","value":97}]`), Expires: live},
		"test:b": {Value: json.RawMessage(`null`), Expires: old},
		"test:c": {Value: json.RawMessage(`"tt0111161"`), Expires: live},
	}))

	got, err := store.Get(ctx, []string{"test:a", "test:b", "test:missing"})
	require.NoError(t, err)
	require.Len(t, got, 2, "expired entries are returned too; absent ones are not")
	assert.JSONEq(t, `[{"source":"rotten_tomatoes","value":97}]`, string(got["test:a"].Value))
	assert.True(t, live.Equal(got["test:a"].Expires))
	assert.JSONEq(t, `null`, string(got["test:b"].Value))

	// A second Put replaces the entry.
	require.NoError(t, store.Put(ctx, map[string]Entry{
		"test:a": {Value: json.RawMessage(`[{"source":"rotten_tomatoes","value":80}]`), Expires: live},
	}))
	got, err = store.Get(ctx, []string{"test:a"})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"source":"rotten_tomatoes","value":80}]`, string(got["test:a"].Value))

	pruned, err := store.Prune(ctx, time.Now().Add(-90*24*time.Hour))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, pruned, int64(1), "the long-expired entry goes")
	got, err = store.Get(ctx, []string{"test:b", "test:c"})
	require.NoError(t, err)
	assert.NotContains(t, got, "test:b")
	assert.Contains(t, got, "test:c")

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 2)

	empty, err := store.Get(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
	require.NoError(t, store.Put(ctx, nil))
}

func TestPersistedCachesShareOnePostgresStore(t *testing.T) {
	store := openTestStore(t)
	c := newAsyncCache[[]string](2, time.Second)
	c.persist = &persistence{store: store, prefix: "test:", logger: slog.Default()}

	got, ok := c.get(context.Background(), "k", func(context.Context) ([]string, time.Duration) {
		return []string{"x", "y"}, time.Hour
	})
	require.True(t, ok)
	assert.Equal(t, []string{"x", "y"}, got)

	fresh := newAsyncCache[[]string](2, time.Second)
	fresh.persist = c.persist
	got, ok = fresh.get(context.Background(), "k", func(context.Context) ([]string, time.Duration) {
		t.Fatal("the stored answer should have served")
		return nil, 0
	})
	require.True(t, ok)
	assert.Equal(t, []string{"x", "y"}, got)
}
