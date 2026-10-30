package handlers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTargetRepo struct {
	targets map[int]repository.NotificationTarget
	nextID  int
	updated *repository.NotificationTarget
	kept    bool
}

func newFakeTargetRepo() *fakeTargetRepo {
	return &fakeTargetRepo{targets: map[int]repository.NotificationTarget{}, nextID: 1}
}
func (f *fakeTargetRepo) List(context.Context) ([]repository.NotificationTarget, error) {
	out := []repository.NotificationTarget{}
	for _, t := range f.targets {
		out = append(out, t)
	}
	return out, nil
}
func (f *fakeTargetRepo) Get(_ context.Context, id int) (*repository.NotificationTarget, error) {
	t, ok := f.targets[id]
	if !ok {
		return nil, nil
	}
	return &t, nil
}
func (f *fakeTargetRepo) Create(_ context.Context, t repository.NotificationTarget) (repository.NotificationTarget, error) {
	t.ID = f.nextID
	f.nextID++
	t.HasToken = t.Token != "" // as the real repository does
	f.targets[t.ID] = t
	return t, nil
}
func (f *fakeTargetRepo) Update(_ context.Context, t repository.NotificationTarget, keepToken bool) (bool, error) {
	_, ok := f.targets[t.ID]
	f.updated, f.kept = &t, keepToken
	if ok {
		f.targets[t.ID] = t
	}
	return ok, nil
}
func (f *fakeTargetRepo) Delete(_ context.Context, id int) (bool, error) {
	_, ok := f.targets[id]
	delete(f.targets, id)
	return ok, nil
}
func (f *fakeTargetRepo) RecordResult(context.Context, int, time.Time, string) error { return nil }

type stubTester struct{ err error }

func (s *stubTester) Test(context.Context, repository.NotificationTarget) error { return s.err }

func targetRouter(repo *fakeTargetRepo, tester *stubTester) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewNotificationTargetHandler(repo, tester, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	r := gin.New()
	r.POST("/notification-targets", h.Create)
	r.PUT("/notification-targets/:id", h.Update)
	r.DELETE("/notification-targets/:id", h.Delete)
	r.POST("/notification-targets/:id/test", h.Test)
	return r
}

func TestNotificationTargetHandler(t *testing.T) {
	send := func(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	t.Run("create validates", func(t *testing.T) {
		r := targetRouter(newFakeTargetRepo(), &stubTester{})
		assert.Equal(t, http.StatusBadRequest, send(r, "POST", "/notification-targets", `{"name":"","kind":"ntfy","url":"https://x","events":["completed"]}`).Code)
		assert.Equal(t, http.StatusBadRequest, send(r, "POST", "/notification-targets", `{"name":"a","kind":"pigeon","url":"https://x","events":["completed"]}`).Code)
		assert.Equal(t, http.StatusBadRequest, send(r, "POST", "/notification-targets", `{"name":"a","kind":"ntfy","url":"ftp://x","events":["completed"]}`).Code)
		assert.Equal(t, http.StatusBadRequest, send(r, "POST", "/notification-targets", `{"name":"a","kind":"ntfy","url":"https://x","events":[]}`).Code)
		assert.Equal(t, http.StatusBadRequest, send(r, "POST", "/notification-targets", `{"name":"a","kind":"ntfy","url":"https://x","events":["exploded"]}`).Code)
	})

	t.Run("create normalises and never returns the token", func(t *testing.T) {
		repo := newFakeTargetRepo()
		w := send(targetRouter(repo, &stubTester{}), "POST", "/notification-targets",
			`{"name":" Phone ","kind":"NTFY","url":"https://ntfy.sh/scout","token":"tk","events":["Failed","failed","completed"]}`)
		require.Equal(t, http.StatusCreated, w.Code)
		assert.NotContains(t, w.Body.String(), `"tk"`)
		assert.Contains(t, w.Body.String(), `"has_token":true`)
		stored := repo.targets[1]
		assert.Equal(t, "Phone", stored.Name)
		assert.Equal(t, "ntfy", stored.Kind)
		assert.Equal(t, []string{"failed", "completed"}, stored.Events)
		assert.True(t, stored.Enabled)
	})

	t.Run("update keeps the token when none is sent", func(t *testing.T) {
		repo := newFakeTargetRepo()
		repo.targets[1] = repository.NotificationTarget{ID: 1, Name: "a", Kind: "webhook", URL: "https://x", Token: "old", Events: []string{"completed"}, Enabled: true}
		r := targetRouter(repo, &stubTester{})
		assert.Equal(t, http.StatusOK, send(r, "PUT", "/notification-targets/1", `{"name":"b","kind":"webhook","url":"https://y","events":["completed"],"enabled":false}`).Code)
		assert.True(t, repo.kept)
		assert.False(t, repo.updated.Enabled)
		assert.Equal(t, http.StatusOK, send(r, "PUT", "/notification-targets/1", `{"name":"b","kind":"webhook","url":"https://y","events":["completed"],"token":""}`).Code)
		assert.False(t, repo.kept, "an explicit empty token clears it")
		assert.Equal(t, http.StatusNotFound, send(r, "PUT", "/notification-targets/9", `{"name":"b","kind":"webhook","url":"https://y","events":["completed"]}`).Code)
	})

	t.Run("delete and test", func(t *testing.T) {
		repo := newFakeTargetRepo()
		repo.targets[1] = repository.NotificationTarget{ID: 1, Kind: "webhook", URL: "https://x", Events: []string{"completed"}}
		assert.Equal(t, http.StatusOK, send(targetRouter(repo, &stubTester{}), "POST", "/notification-targets/1/test", "").Code)
		assert.Equal(t, http.StatusBadGateway, send(targetRouter(repo, &stubTester{err: errors.New("403")}), "POST", "/notification-targets/1/test", "").Code)
		assert.Equal(t, http.StatusNotFound, send(targetRouter(repo, &stubTester{}), "POST", "/notification-targets/2/test", "").Code)
		assert.Equal(t, http.StatusNoContent, send(targetRouter(repo, &stubTester{}), "DELETE", "/notification-targets/1", "").Code)
		assert.Equal(t, http.StatusNotFound, send(targetRouter(repo, &stubTester{}), "DELETE", "/notification-targets/1", "").Code)
	})
}
