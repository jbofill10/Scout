package handlers

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
)

type stubFollows struct {
	followErr error
	found     bool
	gotMedia  tvdb.Media
	gotMiss   bool
}

func (s *stubFollows) Follow(_ context.Context, media tvdb.Media, includeMissing bool) (interactors.FollowResponse, error) {
	s.gotMedia, s.gotMiss = media, includeMissing
	return interactors.FollowResponse{Show: repository.FollowedShow{TvdbID: media.Id}}, s.followErr
}
func (s *stubFollows) Unfollow(context.Context, string) (bool, error) { return s.found, nil }
func (s *stubFollows) List(context.Context) ([]repository.FollowedShow, error) {
	return nil, nil
}
func (s *stubFollows) CheckOne(_ context.Context, id string) (interactors.CheckResult, error) {
	if !s.found {
		return interactors.CheckResult{}, interactors.ErrNothingToControl
	}
	return interactors.CheckResult{TvdbID: id}, nil
}
func (s *stubFollows) CheckAll(context.Context) (interactors.CheckReport, error) {
	return interactors.CheckReport{}, nil
}

func followRouter(stub *stubFollows) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewFollowHandler(stub, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	r := gin.New()
	r.GET("/follows", h.List)
	r.POST("/follows", h.Follow)
	r.DELETE("/follows/:tvdbId", h.Unfollow)
	r.POST("/follows/:tvdbId/check", h.CheckOne)
	return r
}

func TestFollowHandler(t *testing.T) {
	send := func(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	t.Run("follow passes media and the include flag", func(t *testing.T) {
		stub := &stubFollows{}
		w := send(followRouter(stub), "POST", "/follows", `{"media":{"id":"900","mediaName":"Some Show"},"include_missing":true}`)
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "900", stub.gotMedia.Id)
		assert.True(t, stub.gotMiss)
	})

	t.Run("follow needs an id; movies are rejected", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, send(followRouter(&stubFollows{}), "POST", "/follows", `{"media":{}}`).Code)
		assert.Equal(t, http.StatusBadRequest,
			send(followRouter(&stubFollows{followErr: interactors.ErrNotSeries}), "POST", "/follows", `{"media":{"id":"1"}}`).Code)
	})

	t.Run("list is never null", func(t *testing.T) {
		w := send(followRouter(&stubFollows{}), "GET", "/follows", "")
		assert.Equal(t, "[]", strings.TrimSpace(w.Body.String()))
	})

	t.Run("unfollow and check report missing follows", func(t *testing.T) {
		assert.Equal(t, http.StatusNoContent, send(followRouter(&stubFollows{found: true}), "DELETE", "/follows/900", "").Code)
		assert.Equal(t, http.StatusNotFound, send(followRouter(&stubFollows{}), "DELETE", "/follows/900", "").Code)
		assert.Equal(t, http.StatusOK, send(followRouter(&stubFollows{found: true}), "POST", "/follows/900/check", "").Code)
		assert.Equal(t, http.StatusNotFound, send(followRouter(&stubFollows{}), "POST", "/follows/900/check", "").Code)
	})
}
