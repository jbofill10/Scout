package service

import (
	"context"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr/prowlarr"
)

// shortAddRecovery makes a failed add give up on qBittorrent in well under a
// second instead of two minutes, and restores the production values after.
func shortAddRecovery(t *testing.T) {
	t.Helper()
	window, poll := addRecoveryWindow, addRecoveryPoll
	addRecoveryWindow, addRecoveryPoll = 600*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { addRecoveryWindow, addRecoveryPoll = window, poll })
}

func tempSavePath(t *testing.T) {
	t.Helper()
	orig := baseSavePath
	baseSavePath = t.TempDir()
	t.Cleanup(func() { baseSavePath = orig })
}

// qBittorrent answered the add with an error (or not at all) but carried it
// out: the torrent shows up with the tracking tag a moment later and the
// download goes on as if the add had succeeded, with the hash qBittorrent
// reports. Before this, such a torrent downloaded untracked and was never
// linked (Knife Edge S01E08 and JoJo's S05E13 on 2026-11-04).
func TestDownloadTorrent_RecoversAddQbittorrentTookLate(t *testing.T) {
	tempSavePath(t)
	qb := &fakeQbittorrent{addStatus: 500, info: func(poll int) []map[string]any {
		if poll < 3 {
			return nil // not there yet: qBittorrent is still chewing on the add
		}
		return []map[string]any{{"hash": "4f5282d5b54ada0d8e846adbc6f04bacc18baba3", "name": "JoJos S05E13", "state": "downloading"}}
	}}
	q := newCancelTestHandler(t, repo(t), qb)

	hash, uuid, err := q.downloadTorrent(context.Background(),
		&prowlarr.Search{Title: "JoJos S05E13", DownloadURL: "http://indexer/x.torrent"}, false)

	require.NoError(t, err)
	assert.Equal(t, "4f5282d5b54ada0d8e846adbc6f04bacc18baba3", hash)
	assert.NotEmpty(t, uuid)
	assert.GreaterOrEqual(t, qb.polls, 3, "it kept looking until the torrent appeared")
}

// A failed add that never shows up in qBittorrent is a failed add.
func TestDownloadTorrent_FailsWhenNothingArrives(t *testing.T) {
	tempSavePath(t)
	q := newCancelTestHandler(t, repo(t), &fakeQbittorrent{addStatus: 500})

	start := time.Now()
	_, _, err := q.downloadTorrent(context.Background(),
		&prowlarr.Search{Title: "x", GUID: "magnet:?xt=urn:btih:abc", InfoHash: "abc"}, false)

	require.Error(t, err)
	assert.GreaterOrEqual(t, time.Since(start), addRecoveryWindow, "it waited the whole recovery window first")
}

// The search deadline running out underneath the add is a slow torrent client,
// not a cancel: the hand-off after the add still happens on a live context and
// the hash resolution is not cut short.
func TestDownloadTorrent_HashResolutionOutlivesTheSearchDeadline(t *testing.T) {
	tempSavePath(t)
	qb := &fakeQbittorrent{info: func(int) []map[string]any {
		return []map[string]any{{"hash": "cafe", "name": "x", "state": "downloading"}}
	}}
	q := newCancelTestHandler(t, repo(t), qb)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	// Let the deadline expire before the add goes out: the add fails on the
	// expired context, the recovery lookup runs on one that is still alive.
	time.Sleep(200 * time.Millisecond)

	hash, _, err := q.downloadTorrent(ctx, &prowlarr.Search{Title: "x", DownloadURL: "http://indexer/x.torrent"}, false)

	require.NoError(t, err)
	assert.Equal(t, "cafe", hash)
}

// A cancelled request does not keep its torrent: the add fails at once, and a
// torrent qBittorrent carries out anyway under the tag is removed, not monitored.
func TestDownloadTorrent_CancelledAddDiscardsALateArrival(t *testing.T) {
	tempSavePath(t)
	qb := &fakeQbittorrent{addStatus: 500, info: func(int) []map[string]any {
		return []map[string]any{{"hash": "late", "name": "x", "state": "downloading"}}
	}}
	q := newCancelTestHandler(t, repo(t), qb)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := q.downloadTorrent(ctx, &prowlarr.Search{Title: "x", DownloadURL: "http://indexer/x.torrent"}, false)

	require.Error(t, err)
	assert.Eventually(t, func() bool {
		for _, h := range qb.deletedHashes() {
			if h == "late" {
				return true
			}
		}
		return false
	}, 3*time.Second, 20*time.Millisecond, "the late torrent is deleted with its files")
}

func TestAddFailureCode(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()

	assert.Equal(t, dlstatus.CodeCancelled, addFailureCode(cancelled), "a user cancel is not retried")
	assert.Equal(t, dlstatus.CodeTorrentClientError, addFailureCode(expired), "a deadline that ran out is retried")
	assert.Equal(t, dlstatus.CodeTorrentClientError, addFailureCode(context.Background()))
}

// An add that fails after the search deadline expired is reported as a
// transient torrent-client error, which the webserver retries, not as a cancel,
// which it does not.
func TestInitiateDownloadAndMonitor_ExpiredDeadlineIsTransient(t *testing.T) {
	tempSavePath(t)
	r := repo(t)
	r.On("InsertDownloadHistory", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	q := newCancelTestHandler(t, r, &fakeQbittorrent{addStatus: 500})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	match := &models.TorrentMatch{
		Torrent:  &prowlarr.Search{Title: "x", GUID: "magnet:?xt=urn:btih:abc", InfoHash: "abc"},
		Strategy: &models.SearchStrategy{EpisodeTvdbID: "55", Season: 1, Episode: 5},
	}

	result := q.initiateDownloadAndMonitor(ctx, match, nil, make(chan models.TorrentCompleteEvent, 1), nil)

	assert.Equal(t, dlstatus.OutcomeFailed, result.Outcome)
	assert.Equal(t, dlstatus.CodeTorrentClientError, result.Code)
}

func repo(t *testing.T) *repomocks.Repository {
	r := repomocks.NewRepository(t)
	r.On("GetNotification", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	r.On("InsertActiveTorrent", mock.Anything, mock.Anything).Return(nil).Maybe()
	return r
}
