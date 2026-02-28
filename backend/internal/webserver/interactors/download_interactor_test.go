package interactors

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	repositoryMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type schedulerRepoStub struct {
	scheduleCalls int
}

func (s *schedulerRepoStub) Schedule(ctx context.Context, media tvdb.Media, releaseTime time.Time, scheduledTraceID, scheduledSpanID string) error {
	s.scheduleCalls++
	return nil
}

func (s *schedulerRepoStub) GetDueMedia(ctx context.Context, windowEnd time.Time) ([]tvdb.Media, error) {
	return nil, nil
}

func (s *schedulerRepoStub) MarkCompleted(ctx context.Context, media tvdb.Media) error {
	return nil
}

func (s *schedulerRepoStub) RequeueOrFail(
	ctx context.Context,
	media tvdb.Media,
	failureReason string,
	maxRetries int,
	baseDelay time.Duration,
) (bool, int, time.Time, error) {
	return false, 0, time.Time{}, nil
}

func (s *schedulerRepoStub) InsertDownloadHistory(mediaTitle string, season, episode, absoluteEpisode int, status, reason, traceID, spanID string) error {
	return nil
}

func (s *schedulerRepoStub) GetWeeklySchedule() ([]repository.ScheduledDownload, error) {
	return nil, nil
}

func TestNormalizeDateUTC(t *testing.T) {
	// 23:30 in -0800 rolls into next UTC date.
	input := time.Date(2026, 2, 6, 23, 30, 0, 0, time.FixedZone("PST", -8*60*60))
	got := normalizeDateUTC(input)

	assert.Equal(t, time.Date(2026, 2, 7, 0, 0, 0, 0, time.UTC), got)
}

func TestDownloadMovie_SameDayRelease_DownloadsImmediately(t *testing.T) {
	downloadCalls := 0
	tvdbClient := &clients.TVDBProxyClient{
		Host: "tvdb.local",
		Client: &http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "/series/123/extended", r.URL.Path)
				assert.Equal(t, "movie", r.URL.Query().Get("mediaType"))

				body, err := json.Marshal(tvdb.TVDBSeriesExtendedResponse{
					Data: tvdb.TVDBSeriesExtendedData{},
				})
				if err != nil {
					return nil, err
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(string(body))),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	torrenterClient := &clients.TorrenterClient{
		Host: "torrenter.local",
		Client: &http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodPost && r.URL.Path == "/download" {
					downloadCalls++
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}, nil
				}

				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	schedulerRepo := &schedulerRepoStub{}
	notificationRepo := repositoryMocks.NewNotificationRepository(t)
	notificationRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil).Once()

	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	interactor := NewDownloadInteractor(
		schedulerRepo,
		notificationRepo,
		nil,
		nil,
		logger,
		tvdbClient,
		torrenterClient,
	)

	req := tvdb.Media{
		Id:       "123",
		Name:     "Same Day Movie",
		Category: "movie",
		Metadata: tvdb.TVDBSeriesMetadata{
			FirstAired: time.Now().UTC().Format("2006-01-02"),
		},
	}

	err := interactor.DownloadMovie(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, 1, downloadCalls)
	assert.Equal(t, 0, schedulerRepo.scheduleCalls)
}
