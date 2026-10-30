package interactors

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeUpgradeSettings struct {
	enabled bool
	cutoff  string
	batch   int
}

func (f fakeUpgradeSettings) UpgradesEnabled() bool { return f.enabled }
func (f fakeUpgradeSettings) UpgradeCutoff() string { return f.cutoff }
func (f fakeUpgradeSettings) UpgradeBatch() int     { return f.batch }

type fakeUpgradeDownloader struct {
	calls   []models.UpgradeInfo
	windows [][2]int
	medias  []tvdb.Media
	result  dlstatus.EpisodeResult
}

func (f *fakeUpgradeDownloader) DownloadUpgrade(_ context.Context, media *tvdb.Media, _ *tvdb.Episode, info models.UpgradeInfo,
	minTier, maxTier int, done chan<- models.TorrentCompleteEvent) (dlstatus.EpisodeResult, error) {
	f.calls = append(f.calls, info)
	f.windows = append(f.windows, [2]int{minTier, maxTier})
	f.medias = append(f.medias, *media)
	close(done)
	return f.result, nil
}

type noopRunner struct{}

func (noopRunner) RunCompletionHandler(_ context.Context, ch <-chan models.TorrentCompleteEvent) {
	for range ch {
	}
}

func TestPlexResolutionTier(t *testing.T) {
	cases := map[string]string{"sd": "480p", "480": "480p", "720": "720p", "1080": "1080p", "4k": "2160p", "": "", "weird": ""}
	for in, want := range cases {
		_, label := PlexResolutionTier(in)
		assert.Equal(t, want, label, in)
	}
}

func upgradeRows() []models.UpgradeMediaRow {
	return []models.UpgradeMediaRow{
		{Kind: "episode", ShowTvdbID: "900", ShowTitle: "Some Show", EpisodeTvdbID: "1", Season: 1, Episode: 1, Resolution: "720", FilePath: "/m/s1e1.mkv"},
		{Kind: "episode", ShowTvdbID: "900", ShowTitle: "Some Show", EpisodeTvdbID: "2", Season: 1, Episode: 2, Resolution: "720", FilePath: "/m/s1e2-720.mkv"},
		{Kind: "episode", ShowTvdbID: "900", ShowTitle: "Some Show", EpisodeTvdbID: "2", Season: 1, Episode: 2, Resolution: "1080", FilePath: "/m/s1e2-1080.mkv"},
		{Kind: "episode", ShowTvdbID: "900", ShowTitle: "Some Show", EpisodeTvdbID: "3", Season: 1, Episode: 3, Resolution: "1080", FilePath: "/m/s1e3.mkv"},
		{Kind: "episode", ShowTvdbID: "900", ShowTitle: "Some Show", EpisodeTvdbID: "4", Season: 1, Episode: 4, Resolution: "", FilePath: "/m/s1e4.mkv"},
		{Kind: "movie", MovieTvdbID: "77", MovieTitle: "A Movie", Year: 1994, Resolution: "sd", FilePath: "/m/movie.mp4"},
		{Kind: "movie", MovieTvdbID: "78", MovieTitle: "Recent", Year: 2024, Resolution: "480", FilePath: "/m/recent.mp4"},
	}
}

func TestUpgradeCandidates(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetUpgradeMedia", mock.Anything).Return(upgradeRows(), nil)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	repo.On("GetUpgradeAttempts", mock.Anything).Return(map[string]models.UpgradeAttempt{
		"78": {ItemKey: "78", Status: "no_release", Attempts: 1, UpdatedAt: now.Add(-24 * time.Hour)}, // retried in a week
	}, nil)
	u := NewUpgradeInteractor(&fakeUpgradeDownloader{}, repo, fakeUpgradeSettings{enabled: true, cutoff: "1080p", batch: 3}, noopRunner{}, 0,
		slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	u.now = func() time.Time { return now }

	view, err := u.Candidates(context.Background())

	require.NoError(t, err)
	keys := []string{}
	for _, c := range view.Candidates {
		keys = append(keys, c.ItemKey)
	}
	assert.Equal(t, []string{"77", "1"}, keys, "worst copies first; the mixed-version episode, the 1080p one, the unknown one and the deferred movie are out")
	assert.Equal(t, 1, view.Deferred)
	assert.Equal(t, "480p", view.Candidates[0].CurrentQuality)
	assert.Equal(t, []string{"/m/movie.mp4"}, view.Candidates[0].Files)
	assert.Equal(t, "900", view.Candidates[1].ShowTvdbID)
	assert.Equal(t, "1080p", view.Candidates[1].TargetQuality)
}

func TestUpgradeRun_BatchWindowAndBookkeeping(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetUpgradeMedia", mock.Anything).Return(upgradeRows(), nil)
	repo.On("GetUpgradeAttempts", mock.Anything).Return(map[string]models.UpgradeAttempt{}, nil)
	recorded := []models.UpgradeAttempt{}
	repo.On("RecordUpgradeAttempt", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		recorded = append(recorded, args.Get(1).(models.UpgradeAttempt))
	}).Return(nil)
	settled := map[string]string{}
	repo.On("SetUpgradeAttemptStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		settled[args.String(1)] = args.String(2)
	}).Return(nil)
	repo.On("GetNotification", mock.Anything, mock.Anything).Return(nil, nil)
	created := []*notifications.Notification{}
	repo.On("CreateNotification", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		created = append(created, args.Get(1).(*notifications.Notification))
	}).Return(nil)

	dl := &fakeUpgradeDownloader{result: dlstatus.EpisodeResult{Outcome: dlstatus.OutcomeDownloading}}
	u := NewUpgradeInteractor(dl, repo, fakeUpgradeSettings{enabled: true, cutoff: "1080p", batch: 1}, noopRunner{}, 0,
		slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))

	result, err := u.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Started)
	assert.Equal(t, []string{"A Movie (480p)"}, result.Items)

	require.Eventually(t, func() bool {
		u.mu.Lock()
		defer u.mu.Unlock()
		return !u.running
	}, 2*time.Second, 5*time.Millisecond)

	require.Len(t, dl.calls, 1)
	assert.Equal(t, "480p", dl.calls[0].FromQuality)
	assert.Equal(t, []string{"/m/movie.mp4"}, dl.calls[0].OldFiles)
	assert.Equal(t, [2]int{3, 4}, dl.windows[0], "above 480p (tier 2) up to 1080p (tier 4)")
	assert.Equal(t, "movie", dl.medias[0].Category)
	assert.Equal(t, "1994", dl.medias[0].Year)
	require.Len(t, recorded, 1)
	assert.Equal(t, "searching", recorded[0].Status)
	assert.Equal(t, "downloading", settled["77"])
	require.Len(t, created, 1)
	assert.Equal(t, notifications.StatusSearching, created[0].Status)
	assert.Equal(t, "Upgrading from 480p", created[0].Reason)

	// Off means no pass at all.
	off := NewUpgradeInteractor(dl, repo, fakeUpgradeSettings{enabled: false, cutoff: "1080p", batch: 1}, noopRunner{}, 0,
		slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	_, err = off.Run(context.Background())
	assert.ErrorIs(t, err, ErrUpgradesOff)
}

func TestUpgradeRun_NoReleaseIsRecordedForRetry(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetUpgradeMedia", mock.Anything).Return(upgradeRows()[:1], nil)
	repo.On("GetUpgradeAttempts", mock.Anything).Return(map[string]models.UpgradeAttempt{}, nil)
	repo.On("RecordUpgradeAttempt", mock.Anything, mock.Anything).Return(nil)
	settled := map[string]string{}
	reasons := map[string]string{}
	repo.On("SetUpgradeAttemptStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		settled[args.String(1)] = args.String(2)
		reasons[args.String(1)] = args.String(3)
	}).Return(nil)
	existing := &notifications.Notification{ID: 9, TvdbID: "1", Status: notifications.StatusCompleted}
	repo.On("GetNotification", mock.Anything, "1").Return(existing, nil)
	updates := []notifications.NotificationStatus{}
	repo.On("UpdateNotification", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		updates = append(updates, args.Get(1).(*notifications.Notification).Status)
	}).Return(nil)

	dl := &fakeUpgradeDownloader{result: dlstatus.EpisodeResult{Outcome: dlstatus.OutcomeFailed, Code: dlstatus.CodeNoTorrentFound}}
	u := NewUpgradeInteractor(dl, repo, fakeUpgradeSettings{enabled: true, cutoff: "1080p", batch: 5}, noopRunner{}, 0,
		slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	_, err := u.Run(context.Background())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		u.mu.Lock()
		defer u.mu.Unlock()
		return !u.running
	}, 2*time.Second, 5*time.Millisecond)

	assert.Equal(t, "no_release", settled["1"])
	assert.Contains(t, reasons["1"], "No release above 720p")
	assert.Equal(t, []notifications.NotificationStatus{notifications.StatusSearching, notifications.StatusFailed}, updates,
		"the existing notification is reused, not duplicated")
	assert.Equal(t, "900", dl.medias[0].Id, "episodes are requested under the show id")
	require.Len(t, dl.medias[0].Metadata.Episodes, 1)
}
