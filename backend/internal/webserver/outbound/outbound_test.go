package outbound

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capture struct {
	mu      sync.Mutex
	headers http.Header
	body    string
	status  int
}

func (c *capture) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.headers = r.Header.Clone()
		c.body = string(b)
		c.mu.Unlock()
		if c.status != 0 {
			http.Error(w, "nope", c.status)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func intp(v int) *int { return &v }

func episodeDone() notifications.Notification {
	return notifications.Notification{ID: 7, TvdbID: "55", MediaTitle: "Some Show", Category: "series", Season: intp(2), Episode: intp(3),
		Status: notifications.StatusCompleted, UpdatedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
}

func TestBuildEvent_Wording(t *testing.T) {
	ev := BuildEvent(episodeDone())
	assert.Equal(t, "Some Show S02E03", ev.Title)
	assert.Equal(t, "Downloaded and in your library", ev.Message)
	assert.Equal(t, "completed", ev.Event)

	anime := episodeDone()
	anime.IsAnime, anime.AbsoluteEpisode = true, intp(12)
	assert.Equal(t, "Some Show Episode 12", BuildEvent(anime).Title)

	movie := notifications.Notification{MediaTitle: "A Movie", Category: "movie", Status: notifications.StatusFailed, Reason: "Gave up after 5 attempts"}
	ev = BuildEvent(movie)
	assert.Equal(t, "A Movie", ev.Title)
	assert.Equal(t, "Gave up after 5 attempts", ev.Message)

	assert.Equal(t, "Cancelled", BuildEvent(notifications.Notification{Status: notifications.StatusCancelled}).Message)
}

func TestHTTPSender_PayloadPerKind(t *testing.T) {
	ev := BuildEvent(episodeDone())

	t.Run("webhook posts JSON with a bearer token", func(t *testing.T) {
		cap := &capture{}
		srv := httptest.NewServer(cap.handler())
		defer srv.Close()
		err := NewHTTPSender().Send(context.Background(), repository.NotificationTarget{Kind: "webhook", URL: srv.URL, Token: "secret"}, ev)
		require.NoError(t, err)
		assert.Equal(t, "application/json", cap.headers.Get("Content-Type"))
		assert.Equal(t, "Bearer secret", cap.headers.Get("Authorization"))
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(cap.body), &got))
		assert.Equal(t, "completed", got["event"])
		assert.Equal(t, "Some Show S02E03", got["title"])
		assert.Equal(t, float64(2), got["season"])
	})

	t.Run("ntfy sends the message as text with title and tag headers", func(t *testing.T) {
		cap := &capture{}
		srv := httptest.NewServer(cap.handler())
		defer srv.Close()
		failed := episodeDone()
		failed.Status, failed.Reason = notifications.StatusFailed, "No torrent found"
		err := NewHTTPSender().Send(context.Background(), repository.NotificationTarget{Kind: "ntfy", URL: srv.URL}, BuildEvent(failed))
		require.NoError(t, err)
		assert.Equal(t, "No torrent found", cap.body)
		assert.Equal(t, "Scout: Some Show S02E03", cap.headers.Get("Title"))
		assert.Equal(t, "x", cap.headers.Get("Tags"))
		assert.Equal(t, "4", cap.headers.Get("Priority"))
		assert.Empty(t, cap.headers.Get("Authorization"))
	})

	t.Run("discord sends content", func(t *testing.T) {
		cap := &capture{}
		srv := httptest.NewServer(cap.handler())
		defer srv.Close()
		require.NoError(t, NewHTTPSender().Send(context.Background(), repository.NotificationTarget{Kind: "discord", URL: srv.URL}, ev))
		var got map[string]string
		require.NoError(t, json.Unmarshal([]byte(cap.body), &got))
		assert.Equal(t, "**Scout** · Some Show S02E03\nDownloaded and in your library", got["content"])
	})

	t.Run("non-2xx is an error naming the status", func(t *testing.T) {
		cap := &capture{status: http.StatusForbidden}
		srv := httptest.NewServer(cap.handler())
		defer srv.Close()
		err := NewHTTPSender().Send(context.Background(), repository.NotificationTarget{Kind: "webhook", URL: srv.URL}, ev)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "403")
	})

	t.Run("unknown kind", func(t *testing.T) {
		assert.Error(t, NewHTTPSender().Send(context.Background(), repository.NotificationTarget{Kind: "pigeon", URL: "http://x"}, ev))
	})
}

type fakePending struct {
	pending []notifications.Notification
	marked  map[int]string
}

func (f *fakePending) PendingDeliveries(context.Context, int) ([]notifications.Notification, error) {
	return f.pending, nil
}
func (f *fakePending) MarkNotified(_ context.Context, id int, status string) error {
	if f.marked == nil {
		f.marked = map[int]string{}
	}
	f.marked[id] = status
	return nil
}

type fakeTargets struct {
	targets []repository.NotificationTarget
	results map[int]string
}

func (f *fakeTargets) List(context.Context) ([]repository.NotificationTarget, error) {
	return f.targets, nil
}
func (f *fakeTargets) RecordResult(_ context.Context, id int, _ time.Time, errMsg string) error {
	if f.results == nil {
		f.results = map[int]string{}
	}
	f.results[id] = errMsg
	return nil
}

type recordingSender struct {
	sent   []string // "targetID:event"
	failOn int
}

func (r *recordingSender) Send(_ context.Context, t repository.NotificationTarget, ev Event) error {
	r.sent = append(r.sent, string(rune('0'+t.ID))+":"+ev.Event)
	if t.ID == r.failOn {
		return errors.New("boom")
	}
	return nil
}

func TestDispatcher_RunOnce(t *testing.T) {
	done := episodeDone()
	failed := episodeDone()
	failed.ID, failed.Status = 8, notifications.StatusFailed
	notifs := &fakePending{pending: []notifications.Notification{done, failed}}
	targets := &fakeTargets{targets: []repository.NotificationTarget{
		{ID: 1, Name: "all", Kind: "webhook", Enabled: true, Events: []string{"completed", "failed"}},
		{ID: 2, Name: "failures only", Kind: "ntfy", Enabled: true, Events: []string{"failed"}},
		{ID: 3, Name: "disabled", Kind: "discord", Enabled: false, Events: []string{"completed", "failed"}},
	}}
	sender := &recordingSender{failOn: 2}
	d := NewDispatcher(notifs, targets, sender, time.Hour, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))

	n, err := d.RunOnce(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, []string{"1:completed", "1:failed", "2:failed"}, sender.sent,
		"each enabled target gets the events it wants; the disabled one gets nothing")
	assert.Equal(t, map[int]string{7: "completed", 8: "failed"}, notifs.marked, "both transitions are marked delivered, even though one target failed")
	assert.Equal(t, "", targets.results[1])
	assert.Equal(t, "boom", targets.results[2], "the failing target's error is recorded")
}

func TestDispatcher_Test(t *testing.T) {
	targets := &fakeTargets{}
	sender := &recordingSender{}
	d := NewDispatcher(&fakePending{}, targets, sender, time.Hour, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	require.NoError(t, d.Test(context.Background(), repository.NotificationTarget{ID: 4, Kind: "webhook"}))
	assert.Equal(t, []string{"4:completed"}, sender.sent)
	assert.Contains(t, targets.results, 4)
}
