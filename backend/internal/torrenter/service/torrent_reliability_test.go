package service

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr/prowlarr"
)

// A title that merely contains the expected name is another show: spin-offs
// and sequels add a trailing word to their parent's title.
func TestScoreNameMatch_WordsNotSubstrings(t *testing.T) {
	q := newRelaxTestHandler(t)
	cases := []struct {
		parsed, expected string
		want             float64
	}{
		{"Dragon Ball", "Dragon Ball", 1.0},
		{"dragon.ball", "Dragon Ball", 1.0},
		{"Dragon Ball DAIMA", "Dragon Ball", 0},
		{"Dragon Ball Super", "Dragon Ball", 0},
		{"Dragon Ball Super Beerus", "Dragon Ball Super", 0},
		{"Dragon Ball", "Dragon Ball Z", 0},
		{"SpongeBob SquarePants (1999)", "SpongeBob SquarePants", 0.8},
		{"SpongeBob", "SpongeBob SquarePants", 0},
		{"Marvel's The Punisher", "The Punisher", 0.8},
		{"Marvels The Punisher", "Marvel's The Punisher", 1.0},
		{"Godfather", "The Godfather", 0.8},
		{"The Godfather Part II", "The Godfather", 0},
		{"South Park America", "South Park", 0},
		{"Shameless US", "Shameless", 0.8},
		{"Shingeki no Kyojin", "Attack on Titan", 0},
		{"Movie & Title", "Movie and Title", 1.0},
	}
	for _, c := range cases {
		assert.InDelta(t, c.want, q.scoreNameMatch(c.parsed, c.expected), 0.001, "%q vs %q", c.parsed, c.expected)
	}
}

func TestSearchNames_DedupesSplitsAndCaps(t *testing.T) {
	req := &tvdb.Media{Name: "Cyberpunk: Edgerunners", Aliases: []string{
		"Cyberpunk: Edgerunners II", "Cyberpunk: Edgerunners 2", "サイバーパンク エッジランナーズ",
		"Cyberpunk: Edgerunners II (サイバーパンク: エッジランナーズ２)", "Cyberpunk: Edgerunners II",
		"サイバーパンク: エッジランナーズ２", "Cyberpunk: Edgerunners", "cyberpunk edgerunners", "Cyberpunk - Edgerunners",
	}}
	names := searchNames(req)
	assert.Equal(t, []string{"Cyberpunk: Edgerunners", "Cyberpunk: Edgerunners II", "Cyberpunk: Edgerunners 2",
		"サイバーパンク エッジランナーズ"}, names, "title first, Latin aliases next, capped at four")

	assert.Equal(t, []string{"Show"}, searchNames(&tvdb.Media{Name: "Show", Aliases: []string{"Show (2019)", "show", ""}}),
		"case and a trailing year do not make a new name")
	assert.Equal(t, []string{"Show", "Other Title"},
		searchNames(&tvdb.Media{Name: "Show", Aliases: []string{"Show (Other Title)"}}),
		"a parenthesised alias contributes the inner title")
}

// Six aliases used to make 47 queries per indexer, more than a rate-limited
// indexer answers inside the search deadline. The ladder now caps names and
// only repeats the common "S##E##" form under every name.
func TestCreateSearchStrategy_ShortLadderForManyAliases(t *testing.T) {
	q := newRelaxTestHandler(t)
	req := &tvdb.Media{Name: "Cyberpunk: Edgerunners", Anime: true, Aliases: []string{
		"Cyberpunk: Edgerunners II", "Cyberpunk: Edgerunners 2", "サイバーパンク エッジランナーズ",
		"Cyberpunk: Edgerunners II (サイバーパンク: エッジランナーズ２)", "サイバーパンク: エッジランナーズ２",
	}}
	ss := q.createSearchStrategy(req, &tvdb.Episode{Id: 1, SeasonNumber: 2, Number: 6, AbsoluteNumber: 16})

	assert.LessOrEqual(t, len(ss), 20, "queries: %v", queriesAtLevel(ss, 0))
	seen := map[string]bool{}
	for _, s := range ss {
		assert.False(t, seen[strings.ToLower(s.Query)], "duplicate query %q", s.Query)
		seen[strings.ToLower(s.Query)] = true
	}
	assert.Contains(t, queriesAtLevel(ss, 0), "Cyberpunk: Edgerunners S02E06")
	assert.Contains(t, queriesAtLevel(ss, 0), "Cyberpunk: Edgerunners 16")
	assert.Contains(t, queriesAtLevel(ss, 1), "Cyberpunk Edgerunners S02E06", "punctuation-stripped form")
	assert.Equal(t, []string{"Cyberpunk: Edgerunners 2x06"}, queriesAtLevel(ss, 1)[:1], "rare forms only under the title")
	assert.NotContains(t, queriesAtLevel(ss, 1), "Cyberpunk: Edgerunners 16", "the unpadded absolute repeats tier 0 from 10 on")
}

func TestWaitingAndErrorStates(t *testing.T) {
	for _, s := range []qbittorrent.TorrentState{qbittorrent.TorrentStateQueuedDl, qbittorrent.TorrentStateCheckingUp,
		qbittorrent.TorrentStateCheckingDl, qbittorrent.TorrentStateCheckingResumeData, qbittorrent.TorrentStateAllocating,
		qbittorrent.TorrentStateMoving, qbittorrent.TorrentStatePausedDl, qbittorrent.TorrentStateStoppedDl} {
		assert.True(t, isWaitingState(s), string(s))
		assert.False(t, isErrorState(s), string(s))
	}
	for _, s := range []qbittorrent.TorrentState{qbittorrent.TorrentStateDownloading, qbittorrent.TorrentStateStalledDl,
		qbittorrent.TorrentStateMetaDl, qbittorrent.TorrentStateForcedDl} {
		assert.False(t, isWaitingState(s), string(s))
		assert.False(t, isErrorState(s), string(s))
	}
	assert.True(t, isErrorState(qbittorrent.TorrentStateError))
	assert.True(t, isErrorState(qbittorrent.TorrentStateMissingFiles))
}

// A torrent queued behind qBittorrent's active-download limit shows no seeds
// and no speed for as long as it waits. That is not a dead swarm: the monitor
// keeps watching, and the download completes once a slot frees up.
func TestMonitor_QueuedTorrentIsNotADeadSwarm(t *testing.T) {
	t.Setenv("DEAD_SWARM_GRACE", "40ms")
	t.Setenv("MONITOR_TIMEOUT", "1h")
	fastPolls(t)
	qb := &fakeQbittorrent{info: func(poll int) []map[string]any {
		if poll < 20 { // ~100ms queued, past the grace period
			return []map[string]any{qbitTorrent("stall-hash", 0, 0, 0, "queuedDL")}
		}
		progress := 0.3 * float64(poll-19)
		if progress >= 1 {
			progress = 1
		}
		return []map[string]any{qbitTorrent("stall-hash", progress, 4, 1_000_000, "downloading")}
	}}
	repo := repomocks.NewRepository(t)
	q := newCancelTestHandler(t, repo, qb)

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	go q.monitorTorrentCompletion(context.Background(), stallTestMatch(), nil, "stall-hash", "uuid-1", time.Now(), done, &wg)

	select {
	case ev := <-done:
		assert.Equal(t, "stall-hash", ev.Hash)
	case <-time.After(5 * time.Second):
		t.Fatal("a queued torrent must be waited for, not given up on")
	}
	wg.Wait()
	assert.Empty(t, qb.deletedHashes(), "nothing was removed from qBittorrent")
}

// A torrent qBittorrent itself has put in an error state is given up on, but
// the release is not blamed: it is not written to the failed list, so a
// later search may pick it again once the client is healthy.
func TestMonitor_ErrorStateGivesUpWithoutBlamingRelease(t *testing.T) {
	t.Setenv("DEAD_SWARM_GRACE", "40ms")
	fastPolls(t)
	qb := &fakeQbittorrent{info: func(int) []map[string]any {
		return []map[string]any{qbitTorrent("stall-hash", 0, 0, 0, "error")}
	}}
	notification := &notifications.Notification{ID: 9, TvdbID: "77"}
	repo := repomocks.NewRepository(t)
	repo.On("InsertDownloadHistory", mock.Anything, "Big Movie", 0, 0, 0, "", "failure",
		mock.MatchedBy(func(r string) bool { return strings.HasPrefix(r, "qBittorrent reported error") })).Return(nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "stall-hash").Return(nil)
	repo.On("GetNotification", mock.Anything, "77").Return(notification, nil)
	var written *notifications.Notification
	repo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		written = n
		return true
	})).Return(nil)
	q := newCancelTestHandler(t, repo, qb)

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	finished := make(chan struct{})
	go func() {
		q.monitorTorrentCompletion(context.Background(), stallTestMatch(), nil, "stall-hash", "uuid-1", time.Now(), done, &wg)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("an errored torrent must be given up on after the grace period")
	}

	assert.Equal(t, []string{"stall-hash"}, qb.deletedHashes())
	require.NotNil(t, written)
	assert.Equal(t, notifications.StatusFailed, written.Status)
	assert.Equal(t, string(dlstatus.CodeMonitorLost), written.FailureCode)
	repo.AssertNotCalled(t, "InsertFailedTorrent", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

// A release given up on long ago is no longer excluded; a recent one is.
func TestLoadExcludedTorrents_ExpiresOldEntries(t *testing.T) {
	t.Setenv("FAILED_TORRENT_TTL", "")
	repo := repomocks.NewRepository(t)
	repo.On("GetFailedTorrents", mock.Anything, "900", 1, 5).Return([]*models.FailedTorrent{
		{Title: "Old.Release.S01E05", FailedAt: time.Now().Add(-30 * 24 * time.Hour)},
		{Title: "Fresh.Release.S01E05", FailedAt: time.Now().Add(-24 * time.Hour)},
		{Title: "Undated.Release.S01E05"},
	}, nil)
	q := newResumeTestHandler(t, repo)

	ex := q.loadExcludedTorrents(context.Background(), "900", 1, 5)

	require.NotNil(t, ex)
	assert.False(t, ex.contains(&prowlarr.Search{Title: "Old.Release.S01E05"}), "expired")
	assert.True(t, ex.contains(&prowlarr.Search{Title: "Fresh.Release.S01E05"}))
	assert.True(t, ex.contains(&prowlarr.Search{Title: "Undated.Release.S01E05"}), "rows without a date are kept")

	repo2 := repomocks.NewRepository(t)
	repo2.On("GetFailedTorrents", mock.Anything, "900", 1, 6).Return([]*models.FailedTorrent{
		{Title: "Old.Release.S01E06", FailedAt: time.Now().Add(-30 * 24 * time.Hour)},
	}, nil)
	q2 := newResumeTestHandler(t, repo2)
	assert.Nil(t, q2.loadExcludedTorrents(context.Background(), "900", 1, 6), "nothing live excludes nothing")
}

func TestExecuteSearchStrategies_SkipsWhenTierBudgetSpent(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {hitFrom(ONE337x_ID, "Some Show S01E05 1080p WEB-GRP")},
	}}
	q := newFallbackTestHandler(t, fake)
	req, strategies := someShowStrategies(q)

	matches, _, allFailed := q.executeSearchStrategies(context.Background(), strategies, req, []int64{ONE337x_ID}, nil,
		time.Now().Add(-time.Second))

	assert.Empty(t, matches)
	assert.False(t, allFailed, "skipped strategies are not indexer failures")
	assert.Empty(t, fake.indexersQueried(), "nothing is sent once the tier's share is spent")
}

// With a slow preferred indexer, the search used to spend the whole deadline
// there and never ask the fallback. Each indexer tier now gets a share of
// the deadline, so the Pirate Bay still answers for the release it carries.
func TestSearchForMatch_FallbackIndexerGetsItsShareOfTheDeadline(t *testing.T) {
	fake := &fakeProwlarr{t: t, delay: 300 * time.Millisecond, results: map[int64][]*prowlarr.Search{
		TPB_ID: {hitWithSeeders(TPB_ID, "Some Show S01E05 1080p WEB-GRP", 80)},
	}}
	q := newFallbackTestHandler(t, fake)
	q.minQualityTier = QualityTier("1080p")
	req, strategies := someShowStrategies(q)
	require.GreaterOrEqual(t, len(strategies), 4, "the preferred tier alone would outrun a 2s deadline at 300ms a query")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	outcome := q.searchForMatch(ctx, strategies, req, nil)

	require.NotNil(t, outcome.match, "the fallback indexer must get its turn; queried: %v", fake.indexersQueried())
	assert.Equal(t, "Some Show S01E05 1080p WEB-GRP", outcome.match.Torrent.Title)
	assert.Contains(t, fake.indexersQueried(), TPB_ID)
	assert.False(t, outcome.erroredAll)
}

func TestParsePack_SeasonRanges(t *testing.T) {
	cases := []struct {
		title             string
		show              string
		season, seasonEnd int
		epStart, epEnd    int
		complete, series  bool
	}{
		{"SpongeBob SquarePants S01-13 1080p COMPLETE", "SpongeBob SquarePants", 1, 13, 0, 0, true, false},
		{"SpongeBob SquarePants S01-S13 1080p", "SpongeBob SquarePants", 1, 13, 0, 0, false, false},
		{"Spongebob Squarepants Seasons 1-5 720p", "Spongebob Squarepants", 1, 5, 0, 0, false, false},
		{"SpongeBob SquarePants S08 and S09", "SpongeBob SquarePants", 8, 9, 0, 0, false, false},
		{"SpongeBob SquarePants S01 and S05", "SpongeBob SquarePants", 1, 0, 0, 0, false, false},
		{"The Wire Complete Series 1080p BluRay", "The Wire", 0, 0, 0, 0, true, true},
		{"Severance S01E01-E09 720p WEB", "Severance", 1, 0, 1, 9, false, false},
		{"Severance S02 1080p WEB", "Severance", 2, 0, 0, 0, false, false},
	}
	for _, c := range cases {
		pt := parsePack(c.title)
		assert.False(t, pt.ParseError, c.title)
		assert.Equal(t, c.show, pt.ShowName, c.title)
		assert.Equal(t, [2]int{c.season, c.seasonEnd}, [2]int{pt.Season, pt.SeasonEnd}, c.title)
		assert.Equal(t, [2]int{c.epStart, c.epEnd}, [2]int{pt.EpStart, pt.EpEnd}, c.title)
		assert.Equal(t, c.complete, pt.Complete, "complete: "+c.title)
		assert.Equal(t, c.series, pt.Series, "series: "+c.title)
	}
}

func multiSeasonShow() (*tvdb.Media, []int, []tvdb.Episode) {
	eps := []tvdb.Episode{}
	id := 500
	for _, s := range []int{5, 6, 7} {
		for e := 1; e <= 3; e++ {
			id++
			eps = append(eps, tvdb.Episode{Id: id, SeasonNumber: s, Number: e})
		}
	}
	return &tvdb.Media{Id: "75886", Name: "SpongeBob SquarePants", Category: "series"}, []int{5, 6, 7}, eps
}

func TestValidatePack_MultiSeason(t *testing.T) {
	q := newResumeTestHandler(t, repomocks.NewRepository(t))
	q.minQualityTier = QualityTier("1080p")
	show, seasons, eps := multiSeasonShow()
	accept := map[string]bool{
		"SpongeBob SquarePants S01-13 1080p COMPLETE":     true,
		"SpongeBob SquarePants Seasons 5-7 1080p WEB":     true,
		"SpongeBob SquarePants S05-S08 1080p WEB":         true,
		"SpongeBob SquarePants Complete Series 1080p":     true,
		"SpongeBob SquarePants S05 1080p WEB":             false, // one season of three
		"SpongeBob SquarePants S06-07 1080p":              false, // misses season 5
		"SpongeBob SquarePants S07E01 1080p WEB":          false,
		"SpongeBob SquarePants S01-13 720p COMPLETE":      false, // below the floor
		"SpongeBob SquarePants Movie Collection 1080p":    false,
		"Spongebob Squarepants S01 S01 1080p AMZN AAC2.0": false,
	}
	for title, want := range accept {
		ok, why, _, _ := q.validatePack(&prowlarr.Search{Title: title, Seeders: 20}, show, seasons, eps)
		assert.Equal(t, want, ok, "%s: %s", title, why)
	}

	// A single wanted season is covered by a multi-season pack too.
	ok, why, _, _ := q.validatePack(&prowlarr.Search{Title: "SpongeBob SquarePants S01-13 1080p COMPLETE", Seeders: 20},
		show, []int{7}, eps[6:])
	assert.True(t, ok, why)
}

func TestPackQueries_MultiSeason(t *testing.T) {
	show, seasons, _ := multiSeasonShow()
	queries := packQueries(show, seasons)
	assert.Equal(t, []string{"SpongeBob SquarePants S05-S07", "SpongeBob SquarePants complete series",
		"SpongeBob SquarePants seasons 5-7", "SpongeBob SquarePants complete"}, queries)
	assert.Equal(t, []string{"SpongeBob SquarePants S07", "SpongeBob SquarePants season 7", "SpongeBob SquarePants S07 complete"},
		packQueries(show, []int{7}))
}

// An exact-span pack beats one that spans the whole series, and either beats
// per-season packs that cover only part of what is wanted.
func TestSearchForPack_MultiSeasonPrefersExactSpan(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {
			hitWithSeeders(ONE337x_ID, "SpongeBob SquarePants S01-13 1080p COMPLETE", 90),
			hitWithSeeders(ONE337x_ID, "SpongeBob SquarePants S05-S07 1080p WEB-DL", 60),
			hitWithSeeders(ONE337x_ID, "SpongeBob SquarePants S05 1080p WEB-DL", 500),
		},
	}}
	q := newFallbackTestHandler(t, fake)
	q.minQualityTier = QualityTier("1080p")
	show, seasons, eps := multiSeasonShow()

	match := q.searchForPack(context.Background(), show, seasons, eps, nil)

	require.NotNil(t, match)
	assert.Equal(t, "SpongeBob SquarePants S05-S07 1080p WEB-DL", match.Torrent.Title)
	require.NotNil(t, match.Strategy.Pack)
	assert.Equal(t, []int{5, 6, 7}, match.Strategy.Pack.Seasons)
	assert.Equal(t, 5, match.Strategy.Pack.Season)
	assert.Len(t, match.Strategy.Pack.Episodes, 9)
	assert.Equal(t, "501", match.Strategy.EpisodeTvdbID)
}

func TestPackFileEpisode_ReadsSeasonFolders(t *testing.T) {
	_, seasons, eps := multiSeasonShow()
	pack := &models.PackInfo{Season: 5, Seasons: seasons, Episodes: eps}
	byKey := packEpisodeIndex(pack)
	parser := NewTorrentParser()

	cases := map[string][2]int{
		"Pack/Season 07/SpongeBob SquarePants - 02.mkv": {7, 2},
		"Pack/S06/E03.mkv": {6, 3},
		"Pack/SpongeBob.SquarePants.S05E01.1080p.WEB.mkv":          {5, 1},
		"Pack/Season 5/SpongeBob.SquarePants.S07E03.1080p.WEB.mkv": {7, 3}, // the file's own numbering wins
	}
	for path, want := range cases {
		ep, ok := packFileEpisode(parser, path, pack, byKey)
		require.True(t, ok, path)
		assert.Equal(t, want, [2]int{ep.SeasonNumber, ep.Number}, path)
	}
	_, ok := packFileEpisode(parser, "Pack/Season 09/SpongeBob SquarePants - 02.mkv", pack, byKey)
	assert.False(t, ok, "an episode of a season that was not wanted")
	assert.Equal(t, 5, packFileSeason("Pack/Extras/thing.mkv", pack), "no season folder: the pack's first season")
}

// Once a pack's file list is known, files holding no wanted episode are
// deselected so the download is only as big as what was asked for.
func TestTrimPackFiles_DeselectsUnwantedFiles(t *testing.T) {
	qb := &fakeQbittorrent{files: []map[string]any{
		{"index": 0, "name": "Pack/Season 05/SpongeBob SquarePants - S05E01.mkv", "size": 100, "priority": 1},
		{"index": 1, "name": "Pack/Season 05/SpongeBob SquarePants - S05E09.mkv", "size": 100, "priority": 1},
		{"index": 2, "name": "Pack/Season 07/SpongeBob SquarePants - S07E03.mkv", "size": 100, "priority": 1},
		{"index": 3, "name": "Pack/Season 11/SpongeBob SquarePants - S11E01.mkv", "size": 100, "priority": 1},
		{"index": 4, "name": "Pack/readme.nfo", "size": 1, "priority": 1},
	}}
	q := newCancelTestHandler(t, repomocks.NewRepository(t), qb)
	_, seasons, eps := multiSeasonShow()
	pack := &models.PackInfo{Season: 5, Seasons: seasons, Episodes: eps}

	done, usable := q.trimPackFiles(context.Background(), "pack-hash", pack)
	assert.True(t, done)
	assert.True(t, usable)
	assert.Equal(t, []string{"1|3|4=0"}, qb.priorities())

	// No file list yet (a magnet still fetching metadata): try again later.
	qb.files = nil
	done, _ = q.trimPackFiles(context.Background(), "pack-hash", pack)
	assert.False(t, done)

	// Nothing matched: the pack is not usable, nothing is deselected.
	qb.files = []map[string]any{
		{"index": 0, "name": "Pack/disc1.iso", "size": 100, "priority": 1},
		{"index": 1, "name": "Pack/disc2.iso", "size": 100, "priority": 1},
	}
	qb.prio = nil
	done, usable = q.trimPackFiles(context.Background(), "pack-hash", pack)
	assert.True(t, done)
	assert.False(t, usable)
	assert.Empty(t, qb.priorities())
}

// A landed multi-season pack links files out of their season folders.
func TestProcessPack_MultiSeasonFolders(t *testing.T) {
	fs := mocks.NewFileSystem(t)
	repo := repomocks.NewRepository(t)
	fs.On("WalkFiles", "/dl/SpongeBob.S01-13").Return([]models.FileEntry{
		{Path: "Season 05/SpongeBob SquarePants - 01.mkv", Size: 100},
		{Path: "Season 07/SpongeBob SquarePants - 03.mkv", Size: 100},
		{Path: "Season 12/SpongeBob SquarePants - 01.mkv", Size: 100},
	}, nil)
	repo.On("GetShowBaseDirectory", mock.Anything, "75886").Return("/lib/SpongeBob", nil)
	fs.On("MkDir", "/lib/SpongeBob/Season 05").Return(nil)
	fs.On("MkDir", "/lib/SpongeBob/Season 07").Return(nil)
	fs.On("HardLink", "/dl/SpongeBob.S01-13/Season 05/SpongeBob SquarePants - 01.mkv",
		"/lib/SpongeBob/Season 05/SpongeBob SquarePants - S05E01.mkv").Return(nil)
	fs.On("HardLink", "/dl/SpongeBob.S01-13/Season 07/SpongeBob SquarePants - 03.mkv",
		"/lib/SpongeBob/Season 07/SpongeBob SquarePants - S07E03.mkv").Return(nil)

	mp := NewMediaProcessSvc(slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)), repo, fs, nil)
	eps := []tvdb.Episode{{Id: 501, SeasonNumber: 5, Number: 1}, {Id: 507, SeasonNumber: 7, Number: 3}, {Id: 508, SeasonNumber: 7, Number: 4}}
	event := &models.TorrentCompleteEvent{SavePath: "/dl/SpongeBob.S01-13", Req: &models.SearchStrategy{
		MediaName: "SpongeBob SquarePants", TvdbId: "75886", EpisodeTvdbID: "501", Season: 5, Episode: 1, EpisodeMeta: &eps[0],
		Pack: &models.PackInfo{Season: 5, Seasons: []int{5, 7}, Episodes: eps},
	}}

	err := mp.ProcessDownloadedTorrent(context.Background(), event)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"501": "linked", "507": "linked", "508": "not in the season pack"}, event.PackResults)
	fs.AssertNumberOfCalls(t, "HardLink", 2)
}

func packTestMatch() *models.TorrentMatch {
	eps := []tvdb.Episode{{Id: 101, SeasonNumber: 5, Number: 1}, {Id: 102, SeasonNumber: 7, Number: 3}}
	return &models.TorrentMatch{
		Strategy: &models.SearchStrategy{MediaName: "SpongeBob SquarePants", TvdbId: "75886", EpisodeTvdbID: "101",
			Season: 5, Episode: 1, EpisodeMeta: &eps[0], Pack: &models.PackInfo{Season: 5, Seasons: []int{5, 7}, Episodes: eps}},
		Torrent: &prowlarr.Search{Title: "SpongeBob SquarePants S01-13 1080p COMPLETE", InfoHash: "pack-hash"},
	}
}

func packTestFiles() []map[string]any {
	return []map[string]any{
		{"index": 0, "name": "Pack/Season 05/SpongeBob SquarePants - S05E01.mkv", "size": 100, "priority": 1},
		{"index": 1, "name": "Pack/Season 07/SpongeBob SquarePants - S07E03.mkv", "size": 100, "priority": 1},
		{"index": 2, "name": "Pack/Season 11/SpongeBob SquarePants - S11E01.mkv", "size": 800, "priority": 1},
	}
}

// A pack is added paused. Once its file list is known the unwanted files are
// deselected, the free space is checked against what is left, and only then
// is the torrent started; it completes like any other download.
func TestMonitor_PackIsTrimmedCheckedThenStarted(t *testing.T) {
	fastPolls(t)
	orig := freeSpaceFn
	freeSpaceFn = func(string) (uint64, error) { return 500, nil }
	t.Cleanup(func() { freeSpaceFn = orig })

	qb := &fakeQbittorrent{files: packTestFiles()}
	qb.info = func(poll int) []map[string]any {
		started := len(qb.resumedHashes()) > 0
		if !started {
			size := int64(1000)
			if len(qb.priorities()) > 0 {
				size = 200 // qBittorrent reports the size of the files left selected
			}
			tr := qbitTorrent("pack-hash", 0, 0, 0, "pausedDL")
			tr["size"] = size
			return []map[string]any{tr}
		}
		return []map[string]any{qbitTorrent("pack-hash", 1, 3, 0, "uploading")}
	}
	repo := repomocks.NewRepository(t)
	q := newCancelTestHandler(t, repo, qb)
	q.settings = &fakePickerSettings{reserve: 100}

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	go q.monitorTorrentCompletion(context.Background(), packTestMatch(), nil, "pack-hash", "uuid-1", time.Now(), done, &wg)

	select {
	case ev := <-done:
		assert.Equal(t, "pack-hash", ev.Hash)
	case <-time.After(5 * time.Second):
		t.Fatal("a trimmed pack that fits must be started and complete")
	}
	wg.Wait()
	assert.Equal(t, []string{"2=0"}, qb.priorities(), "the season 11 file is deselected")
	assert.Equal(t, []string{"pack-hash"}, qb.resumedHashes())
	assert.Empty(t, qb.deletedHashes())
}

// A pack whose wanted files still do not fit is removed before it starts;
// every episode of it is reported short of disk space, and the release is
// not blamed.
func TestMonitor_PackRemovedWhenTrimmedFilesDoNotFit(t *testing.T) {
	fastPolls(t)
	orig := freeSpaceFn
	freeSpaceFn = func(string) (uint64, error) { return 250, nil }
	t.Cleanup(func() { freeSpaceFn = orig })

	qb := &fakeQbittorrent{files: packTestFiles()}
	qb.info = func(int) []map[string]any {
		tr := qbitTorrent("pack-hash", 0, 0, 0, "pausedDL")
		tr["size"] = int64(200)
		return []map[string]any{tr}
	}
	repo := repomocks.NewRepository(t)
	repo.On("InsertDownloadHistory", mock.Anything, "SpongeBob SquarePants", 5, 1, 0, "", "failure",
		"not enough free disk space for the pack's wanted files").Return(nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "pack-hash").Return(nil)
	statuses := map[string]notifications.NotificationStatus{}
	for _, id := range []string{"101", "102"} {
		id := id
		repo.On("GetNotification", mock.Anything, id).Return(&notifications.Notification{TvdbID: id}, nil)
	}
	repo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		statuses[n.TvdbID] = n.Status
		return true
	})).Return(nil)
	q := newCancelTestHandler(t, repo, qb)
	q.settings = &fakePickerSettings{reserve: 100}

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	finished := make(chan struct{})
	go func() {
		q.monitorTorrentCompletion(context.Background(), packTestMatch(), nil, "pack-hash", "uuid-1", time.Now(), done, &wg)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("a pack that does not fit must be given up on")
	}

	assert.Equal(t, []string{"pack-hash"}, qb.deletedHashes())
	assert.Empty(t, qb.resumedHashes())
	assert.Equal(t, map[string]notifications.NotificationStatus{"101": notifications.StatusSearching, "102": notifications.StatusSearching},
		statuses, "both pack episodes wait for the retry engine")
	repo.AssertNotCalled(t, "InsertFailedTorrent", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

// A file named for a run of episodes holds every wanted episode in the run.
func TestPackFileEpisodes_MultiEpisodeFile(t *testing.T) {
	_, seasons, eps := multiSeasonShow()
	pack := &models.PackInfo{Season: 5, Seasons: seasons, Episodes: eps}
	byKey := packEpisodeIndex(pack)
	parser := NewTorrentParser()

	got := packFileEpisodes(parser, "Pack/Season 07/SpongeBob SquarePants S07E01-E02 1080p.mkv", pack, byKey)
	require.Len(t, got, 2)
	assert.Equal(t, [2]int{7, 1}, [2]int{got[0].SeasonNumber, got[0].Number})
	assert.Equal(t, [2]int{7, 2}, [2]int{got[1].SeasonNumber, got[1].Number})

	got = packFileEpisodes(parser, "Pack/Season 07/SpongeBob SquarePants S07E03-E04 1080p.mkv", pack, byKey)
	require.Len(t, got, 1, "only episode 3 of the run was wanted")
	assert.Equal(t, 3, got[0].Number)

	assert.Empty(t, packFileEpisodes(parser, "Pack/Season 07/SpongeBob SquarePants S07E10-E11 1080p.mkv", pack, byKey))
}

// A landed multi-episode file is linked once under Plex's multi-episode name
// and both episodes are reported linked.
func TestProcessPack_MultiEpisodeFileLinkedAsRange(t *testing.T) {
	fs := mocks.NewFileSystem(t)
	repo := repomocks.NewRepository(t)
	fs.On("WalkFiles", "/dl/SpongeBob.S07").Return([]models.FileEntry{
		{Path: "SpongeBob.SquarePants.S07E01-E02.1080p.WEB.mkv", Size: 100},
	}, nil)
	repo.On("GetShowBaseDirectory", mock.Anything, "75886").Return("/lib/SpongeBob", nil)
	fs.On("MkDir", "/lib/SpongeBob/Season 07").Return(nil)
	fs.On("HardLink", "/dl/SpongeBob.S07/SpongeBob.SquarePants.S07E01-E02.1080p.WEB.mkv",
		"/lib/SpongeBob/Season 07/SpongeBob SquarePants - S07E01-E02.mkv").Return(nil)

	mp := NewMediaProcessSvc(slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)), repo, fs, nil)
	eps := []tvdb.Episode{{Id: 701, SeasonNumber: 7, Number: 1}, {Id: 702, SeasonNumber: 7, Number: 2}, {Id: 703, SeasonNumber: 7, Number: 3}}
	event := &models.TorrentCompleteEvent{SavePath: "/dl/SpongeBob.S07", Req: &models.SearchStrategy{
		MediaName: "SpongeBob SquarePants", TvdbId: "75886", EpisodeTvdbID: "701", Season: 7, Episode: 1, EpisodeMeta: &eps[0],
		Pack: &models.PackInfo{Season: 7, Seasons: []int{7}, Episodes: eps},
	}}

	require.NoError(t, mp.ProcessDownloadedTorrent(context.Background(), event))
	assert.Equal(t, map[string]string{"701": "linked", "702": "linked", "703": "not in the season pack"}, event.PackResults)
	fs.AssertNumberOfCalls(t, "HardLink", 1)
}

// A pack-only request never searches per episode: with packs off, every
// episode comes back as no_pack for the caller to send one at a time.
func TestHandleDownload_PackOnlyAnswersNoPack(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("EpisodeExistsByTvdbId", mock.Anything, "75886", mock.Anything, mock.Anything).Return(false, nil)
	repo.On("GetActiveTorrents", mock.Anything).Return(nil, nil)
	q := newResumeTestHandler(t, repo)
	q.inflight = newInflightRegistry()
	req := &tvdb.Media{Id: "75886", Name: "SpongeBob SquarePants", Category: "series", PackOnly: true}
	req.Metadata.Episodes = []tvdb.Episode{{Id: 701, SeasonNumber: 7, Number: 1}, {Id: 702, SeasonNumber: 7, Number: 2}}
	done := make(chan models.TorrentCompleteEvent, 1)

	results, err := q.HandleDownload(context.Background(), req, done)

	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, dlstatus.OutcomeFailed, r.Outcome)
		assert.Equal(t, dlstatus.CodeNoPack, r.Code)
	}
	assert.True(t, closed(t, done), "done is closed once the pack phase is over")
}

// A pack added from a magnet has no file list while paused: qBittorrent only
// fetches metadata for a running torrent. The monitor starts it, trims once
// the files are known, checks the space, and the download completes.
func TestMonitor_PausedMagnetPackIsStartedForMetadata(t *testing.T) {
	fastPolls(t)
	orig := freeSpaceFn
	freeSpaceFn = func(string) (uint64, error) { return 500, nil }
	t.Cleanup(func() { freeSpaceFn = orig })

	qb := &fakeQbittorrent{}
	qb.info = func(poll int) []map[string]any {
		resumed := len(qb.resumedHashes())
		switch {
		case resumed == 0: // paused magnet, no metadata yet
			tr := qbitTorrent("pack-hash", 0, 0, 0, "pausedDL")
			tr["size"] = int64(0)
			return []map[string]any{tr}
		case len(qb.priorities()) == 0: // started: metadata arrives, files appear
			qb.mu.Lock()
			qb.files = packTestFiles()
			qb.mu.Unlock()
			tr := qbitTorrent("pack-hash", 0, 2, 100, "downloading")
			tr["size"] = int64(1000)
			return []map[string]any{tr}
		case resumed < 2: // trimmed, waiting for the space check to start it
			tr := qbitTorrent("pack-hash", 0, 2, 100, "downloading")
			tr["size"] = int64(200)
			return []map[string]any{tr}
		}
		return []map[string]any{qbitTorrent("pack-hash", 1, 3, 0, "uploading")}
	}
	repo := repomocks.NewRepository(t)
	q := newCancelTestHandler(t, repo, qb)
	q.settings = &fakePickerSettings{reserve: 100}

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	go q.monitorTorrentCompletion(context.Background(), packTestMatch(), nil, "pack-hash", "uuid-1", time.Now(), done, &wg)

	select {
	case ev := <-done:
		assert.Equal(t, "pack-hash", ev.Hash)
	case <-time.After(5 * time.Second):
		t.Fatal("a paused magnet pack must be started for its metadata and complete")
	}
	wg.Wait()
	assert.Equal(t, []string{"2=0"}, qb.priorities(), "trimmed once the files were known")
	assert.GreaterOrEqual(t, len(qb.resumedHashes()), 2, "started for metadata, then started after the space check")
	assert.Empty(t, qb.deletedHashes())
}
