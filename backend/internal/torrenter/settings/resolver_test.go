package settings

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	api "github.com/jbofill10/scout/backend/pkg/settings"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestResolver(t *testing.T) (*Resolver, sqlmock.Sqlmock, *time.Time) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := NewResolver(NewStore(db), Defaults{
		MinQuality:        "1080p",
		SubtitleLanguages: []string{"en"},
		PlexSyncDelay:     2 * time.Minute,
		PlexSyncInterval:  30 * time.Minute,
		SubtitlesEnabled:  true,
	}, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	r.now = func() time.Time { return now }
	return r, mock, &now
}

func settingsRows(pairs ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"key", "value"})
	for i := 0; i+1 < len(pairs); i += 2 {
		rows.AddRow(pairs[i], []byte(pairs[i+1]))
	}
	return rows
}

func TestResolver_OverridesWinOverDefaults(t *testing.T) {
	r, mock, _ := newTestResolver(t)
	mock.ExpectQuery(`SELECT key, value FROM Settings`).WillReturnRows(settingsRows(
		api.KeyMinQuality, `"720p"`,
		api.KeySubtitleLanguages, `["en","es"]`,
		api.KeyPlexSyncInterval, `"0s"`,
	))

	assert.Equal(t, "720p", r.MinQuality())
	assert.Equal(t, []string{"en", "es"}, r.SubtitleLanguages())
	assert.False(t, r.SubtitlePreferSDH(), "no override falls back to the default")
	assert.Equal(t, 2*time.Minute, r.PlexSyncDelay())
	assert.Equal(t, time.Duration(0), r.PlexSyncInterval(), "an explicit 0s disables the periodic sync")
	assert.NoError(t, mock.ExpectationsWereMet(), "one load serves every accessor inside the cache window")
}

func TestResolver_CacheExpiresAndSurvivesLoadFailure(t *testing.T) {
	r, mock, now := newTestResolver(t)
	mock.ExpectQuery(`SELECT key, value FROM Settings`).WillReturnRows(settingsRows(api.KeyMinQuality, `"720p"`))
	assert.Equal(t, "720p", r.MinQuality())

	*now = now.Add(cacheTTL + time.Second)
	mock.ExpectQuery(`SELECT key, value FROM Settings`).WillReturnError(assert.AnError)
	assert.Equal(t, "720p", r.MinQuality(), "a failed refresh keeps the last known value")

	*now = now.Add(cacheTTL + time.Second)
	mock.ExpectQuery(`SELECT key, value FROM Settings`).WillReturnRows(settingsRows())
	assert.Equal(t, "1080p", r.MinQuality(), "a removed override falls back to the default")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResolver_ApplyValidatesBeforeWriting(t *testing.T) {
	r, mock, _ := newTestResolver(t)
	bad := "900p"
	_, err := r.Apply(context.Background(), api.Update{MinQuality: &bad})
	var verr *ValidationError
	require.ErrorAs(t, err, &verr)
	assert.Equal(t, api.KeyMinQuality, verr.Field)

	langs := []string{"english"}
	_, err = r.Apply(context.Background(), api.Update{SubtitleLanguages: &langs})
	require.ErrorAs(t, err, &verr)
	assert.Equal(t, api.KeySubtitleLanguages, verr.Field)

	neg := "-1m"
	_, err = r.Apply(context.Background(), api.Update{PlexSyncDelay: &neg})
	require.ErrorAs(t, err, &verr)

	short := "10s"
	_, err = r.Apply(context.Background(), api.Update{PlexSyncInterval: &short})
	require.ErrorAs(t, err, &verr, "an interval under a minute would hammer Plex")

	_, err = r.Apply(context.Background(), api.Update{Reset: []string{"bogus"}})
	require.ErrorAs(t, err, &verr)

	assert.NoError(t, mock.ExpectationsWereMet(), "nothing was written")
}

func TestResolver_ApplyWritesNormalisedValuesAndResets(t *testing.T) {
	r, mock, _ := newTestResolver(t)
	q := "4K"
	langs := []string{" EN", "es", "en", ""}
	zero := "0s"
	mock.ExpectExec(`DELETE FROM Settings WHERE key = \$1`).WithArgs(api.KeyPlexSyncDelay).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO Settings`).WithArgs(api.KeyMinQuality, []byte(`"2160p"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO Settings`).WithArgs(api.KeySubtitleLanguages, []byte(`["en","es"]`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO Settings`).WithArgs(api.KeyPlexSyncInterval, []byte(`"0s"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT key, value FROM Settings`).WillReturnRows(settingsRows(
		api.KeyMinQuality, `"2160p"`, api.KeySubtitleLanguages, `["en","es"]`, api.KeyPlexSyncInterval, `"0s"`))

	view, err := r.Apply(context.Background(), api.Update{
		MinQuality: &q, SubtitleLanguages: &langs, PlexSyncInterval: &zero, Reset: []string{api.KeyPlexSyncDelay},
	})

	require.NoError(t, err)
	assert.Equal(t, "2160p", view.Values.MinQuality)
	assert.Equal(t, []string{"en", "es"}, view.Values.SubtitleLanguages)
	assert.Equal(t, "0s", view.Values.PlexSyncInterval)
	assert.Equal(t, "2m0s", view.Values.PlexSyncDelay, "the reset key shows its default")
	assert.True(t, view.Overridden[api.KeyMinQuality])
	assert.False(t, view.Overridden[api.KeyPlexSyncDelay])
	assert.Equal(t, "1080p", view.Defaults.MinQuality)
	assert.Equal(t, []string{"480p", "720p", "1080p", "2160p"}, view.Options.MinQuality)
	assert.True(t, view.Options.SubtitlesEnabled)
	assert.NoError(t, mock.ExpectationsWereMet())
}
