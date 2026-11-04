package interactors

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	repoMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/settings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func dueRow(id, epID, season, num int, airedOn string) repository.DueItem {
	m := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	m.Metadata.Episodes = []tvdb.Episode{{Id: epID, SeasonNumber: season, Number: num, Aired: airedOn}}
	return repository.DueItem{ID: id, Media: m}
}

func packsOn() *settings.View {
	return &settings.View{Values: settings.Values{SeasonPacks: true, SeasonPackMinEpisodes: 2}}
}

func groupInteractor(repo *fakeSchedulerRepo, disp *fakeDispatcher, notif repository.NotificationRepository) *DownloadInteractor {
	return &DownloadInteractor{repo: repo, notificationRepo: notif, logger: testLogger(), torrenterClient: disp}
}

func threeRows() (repository.DueItem, []repository.DueItem) {
	return dueRow(1, 11, 1, 1, "2007-10-01"), []repository.DueItem{
		dueRow(2, 12, 1, 2, "2007-10-08"),
		dueRow(3, 13, 7, 1, "2012-10-01"),
	}
}

func TestProcessDueBatch_PackCoversAll(t *testing.T) {
	first, sibs := threeRows()
	repo := &fakeSchedulerRepo{siblings: sibs}
	disp := &fakeDispatcher{settingsView: packsOn(), respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
		var rs []dlstatus.EpisodeResult
		for _, ep := range req.Metadata.Episodes {
			rs = append(rs, epResult(strconv.Itoa(ep.Id), dlstatus.OutcomeDownloading, ""))
		}
		return dlstatus.DownloadResponse{Results: rs}
	}}
	i := groupInteractor(repo, disp, repoMocks.NewNotificationRepository(t))

	i.processDueBatch(first)

	assert.Equal(t, 1, repo.claimCalls)
	assert.Equal(t, "555", repo.claimShowID)
	assert.Equal(t, 1, repo.claimExcept)
	payloads := disp.payloads()
	require.Len(t, payloads, 1)
	assert.True(t, payloads[0].PackOnly)
	assert.Len(t, payloads[0].Metadata.Episodes, 3)
	assert.ElementsMatch(t, []int{1, 2, 3}, repo.completedIDs)
}

func TestProcessDueBatch_NoPackFallsBackPerEpisode(t *testing.T) {
	first, sibs := threeRows()
	repo := &fakeSchedulerRepo{siblings: sibs}
	disp := &fakeDispatcher{settingsView: packsOn(), respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
		if req.PackOnly {
			return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
				epResult("11", dlstatus.OutcomeDownloading, ""),
				epResult("12", dlstatus.OutcomeDownloading, ""),
				epResult("13", dlstatus.OutcomeFailed, dlstatus.CodeNoPack),
			}}
		}
		return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
			epResult("13", dlstatus.OutcomeFailed, dlstatus.CodeNoTorrentFound),
		}}
	}}
	notif := repoMocks.NewNotificationRepository(t)
	notif.On("GetNotification", mock.Anything, "13").
		Return(&notifications.Notification{ID: 9, TvdbID: "13", Status: notifications.StatusSearching}, nil)
	notif.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil)
	i := groupInteractor(repo, disp, notif)

	i.processDueBatch(first)

	payloads := disp.payloads()
	require.Len(t, payloads, 2)
	assert.True(t, payloads[0].PackOnly)
	assert.False(t, payloads[1].PackOnly)
	require.Len(t, payloads[1].Metadata.Episodes, 1)
	assert.Equal(t, 13, payloads[1].Metadata.Episodes[0].Id)
	assert.ElementsMatch(t, []int{1, 2}, repo.completedIDs)
	assert.Equal(t, []int{3}, repo.recordedIDs)
	assert.Empty(t, repo.permFailedIDs)
}

func TestProcessDueBatch_PacksOffPerEpisode(t *testing.T) {
	first, sibs := threeRows()
	repo := &fakeSchedulerRepo{siblings: sibs}
	disp := &fakeDispatcher{
		settingsView: &settings.View{Values: settings.Values{SeasonPacks: false}},
		respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
			return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
				epResult(strconv.Itoa(req.Metadata.Episodes[0].Id), dlstatus.OutcomeDownloading, ""),
			}}
		},
	}
	i := groupInteractor(repo, disp, repoMocks.NewNotificationRepository(t))

	i.processDueBatch(first)

	payloads := disp.payloads()
	require.Len(t, payloads, 3)
	for _, p := range payloads {
		assert.False(t, p.PackOnly)
		assert.Len(t, p.Metadata.Episodes, 1)
	}
	assert.ElementsMatch(t, []int{1, 2, 3}, repo.completedIDs)
}

func TestProcessDueBatch_MovieAndNoSiblings(t *testing.T) {
	ok := dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{epResult("100", dlstatus.OutcomeDownloading, "")}}

	repo := &fakeSchedulerRepo{}
	disp := &fakeDispatcher{resp: ok}
	i := groupInteractor(repo, disp, repoMocks.NewNotificationRepository(t))
	i.processDueBatch(repository.DueItem{ID: 5, Media: tvdb.Media{Id: "9", Category: "movie"}})
	assert.Equal(t, 0, repo.claimCalls, "movies never claim siblings")
	assert.Equal(t, []int{5}, repo.completedIDs)

	repo = &fakeSchedulerRepo{}
	disp = &fakeDispatcher{resp: ok}
	i = groupInteractor(repo, disp, repoMocks.NewNotificationRepository(t))
	i.processDueBatch(dueRow(6, 100, 1, 1, "2007-10-01"))
	assert.Equal(t, 1, repo.claimCalls)
	assert.Equal(t, 1, disp.callCount())
	assert.False(t, disp.payload().PackOnly)
	assert.Equal(t, []int{6}, repo.completedIDs)
}

func TestProcessDueItem_OldContentCap(t *testing.T) {
	noResult := dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
		epResult("11", dlstatus.OutcomeFailed, dlstatus.CodeNoTorrentFound),
	}}
	newNotif := func() *repoMocks.NotificationRepository {
		n := repoMocks.NewNotificationRepository(t)
		n.On("GetNotification", mock.Anything, "11").
			Return(&notifications.Notification{ID: 9, TvdbID: "11", Status: notifications.StatusSearching}, nil)
		n.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil)
		return n
	}

	repo := &fakeSchedulerRepo{}
	i := groupInteractor(repo, &fakeDispatcher{resp: noResult}, newNotif())
	row := dueRow(1, 11, 1, 1, "2007-10-01")
	row.Attempts = 1
	i.processDueItem(context.Background(), row)
	require.NotNil(t, repo.permFailedID)
	assert.Equal(t, string(dlstatus.CodeMaxRetries), repo.permFailedCode)
	assert.Nil(t, repo.recordedID)

	repo = &fakeSchedulerRepo{}
	i = groupInteractor(repo, &fakeDispatcher{resp: noResult}, newNotif())
	row = dueRow(1, 11, 1, 1, time.Now().Add(-24*time.Hour).Format("2006-01-02"))
	row.Attempts = 1
	i.processDueItem(context.Background(), row)
	require.NotNil(t, repo.recordedID)
	assert.Nil(t, repo.permFailedID)
}

func TestProcessDueBatch_PackFailureSettlesRowsWithoutPerEpisode(t *testing.T) {
	// A pack-only answer that fails for a reason other than no_pack (here the
	// disk guard) is the group's answer: every row is rescheduled with backoff
	// and no episode is searched one at a time in the same pass.
	first, sibs := threeRows()
	repo := &fakeSchedulerRepo{siblings: sibs}
	disp := &fakeDispatcher{settingsView: packsOn(), respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
		var rs []dlstatus.EpisodeResult
		for _, ep := range req.Metadata.Episodes {
			rs = append(rs, epResult(strconv.Itoa(ep.Id), dlstatus.OutcomeFailed, dlstatus.CodeInsufficientDiskSpace))
		}
		return dlstatus.DownloadResponse{Results: rs}
	}}
	notif := repoMocks.NewNotificationRepository(t)
	notif.On("GetNotification", mock.Anything, mock.Anything).
		Return(&notifications.Notification{ID: 9, Status: notifications.StatusSearching}, nil)
	notif.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil)
	i := groupInteractor(repo, disp, notif)

	i.processDueBatch(first)

	require.Len(t, disp.payloads(), 1, "the pack request is the only call")
	assert.ElementsMatch(t, []int{1, 2, 3}, repo.recordedIDs)
	assert.Equal(t, string(dlstatus.CodeInsufficientDiskSpace), repo.recordedCode)
	assert.Empty(t, repo.completedIDs)
	assert.Empty(t, repo.permFailedIDs)
}

func TestProcessDueBatch_ClaimErrorDispatchesRowAlone(t *testing.T) {
	first, _ := threeRows()
	repo := &fakeSchedulerRepo{siblingErr: assert.AnError}
	disp := &fakeDispatcher{settingsView: packsOn(), resp: dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
		epResult("11", dlstatus.OutcomeDownloading, ""),
	}}}
	i := groupInteractor(repo, disp, repoMocks.NewNotificationRepository(t))

	i.processDueBatch(first)

	require.Equal(t, 1, disp.callCount())
	assert.False(t, disp.payload().PackOnly)
	assert.Equal(t, []int{1}, repo.completedIDs)
}

func TestProcessDueGroup_OldContentCapAppliesPerRow(t *testing.T) {
	// Rows in a group keep their own ladder: an old episode with no release
	// exhausts after its second attempt while a recent one is rescheduled.
	old := dueRow(1, 11, 1, 1, "2007-10-01")
	old.Attempts = 1
	recent := dueRow(2, 12, 1, 2, time.Now().Add(-48*time.Hour).Format("2006-01-02"))
	recent.Attempts = 1
	repo := &fakeSchedulerRepo{siblings: []repository.DueItem{recent}}
	disp := &fakeDispatcher{settingsView: packsOn(), respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
		var rs []dlstatus.EpisodeResult
		for _, ep := range req.Metadata.Episodes {
			code := dlstatus.CodeNoPack
			if !req.PackOnly {
				code = dlstatus.CodeNoTorrentFound
			}
			rs = append(rs, epResult(strconv.Itoa(ep.Id), dlstatus.OutcomeFailed, code))
		}
		return dlstatus.DownloadResponse{Results: rs}
	}}
	notif := repoMocks.NewNotificationRepository(t)
	notif.On("GetNotification", mock.Anything, mock.Anything).
		Return(&notifications.Notification{ID: 9, Status: notifications.StatusSearching}, nil)
	notif.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil)
	i := groupInteractor(repo, disp, notif)

	i.processDueBatch(old)

	require.Len(t, disp.payloads(), 3, "pack request, then one request per episode")
	assert.Equal(t, []int{1}, repo.permFailedIDs)
	assert.Equal(t, string(dlstatus.CodeMaxRetries), repo.permFailedCode)
	assert.Equal(t, []int{2}, repo.recordedIDs)
}
