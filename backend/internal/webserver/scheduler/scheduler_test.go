package scheduler

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	schedMocks "github.com/jbofill10/scout/backend/internal/webserver/scheduler/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SchedulerTestSuite struct {
	suite.Suite
	scheduler *Scheduler
	mockRepo  *schedMocks.SchedulerRepository
	logger    *slog.Logger
	queue     chan repository.DueItem
}

func TestSchedulerSuite(t *testing.T) {
	suite.Run(t, new(SchedulerTestSuite))
}

func (s *SchedulerTestSuite) SetupTest() {
	s.logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	s.mockRepo = schedMocks.NewSchedulerRepository(s.T())
	s.scheduler = NewScheduler(s.mockRepo, s.logger)
	s.queue = make(chan repository.DueItem, 10)
}

// dueItem builds a DueItem with a release/due time derived from an aired date.
func dueItem(name, aired string) repository.DueItem {
	due, _ := time.Parse("2006-01-02", aired)
	return repository.DueItem{
		Media: tvdb.Media{
			Name: name,
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{{Aired: aired}},
			},
		},
		ReleaseTime: due,
		DueAt:       due,
	}
}

func (s *SchedulerTestSuite) TestNewScheduler() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := schedMocks.NewSchedulerRepository(s.T())

	scheduler := NewScheduler(mockRepo, logger)

	s.NotNil(scheduler)
	s.Equal(mockRepo, scheduler.repo)
	s.Equal(logger, scheduler.logger)
}

func (s *SchedulerTestSuite) TestStart_VariousScenarios() {
	tests := []struct {
		name           string
		mediaList      []repository.DueItem
		repoError      error
		expectedQueued int
		expectQueued   bool // expect MarkQueued to be invoked
		description    string
	}{
		{
			name:           "media already due - immediate send",
			mediaList:      []repository.DueItem{dueItem("Test Show", time.Now().AddDate(0, 0, -1).Format("2006-01-02"))},
			repoError:      nil,
			expectedQueued: 1,
			expectQueued:   true,
			description:    "Should immediately queue media that has already aired",
		},
		{
			name:           "media in future - scheduled send",
			mediaList:      []repository.DueItem{dueItem("Future Show", time.Now().AddDate(0, 0, 2).Format("2006-01-02"))},
			repoError:      nil,
			expectedQueued: 0,
			description:    "Should schedule media for future release (beyond polling window)",
		},
		{
			name:           "empty episodes",
			mediaList:      []repository.DueItem{{Media: tvdb.Media{Name: "Empty Show"}}},
			repoError:      nil,
			expectedQueued: 0,
			description:    "Should not queue media with no episodes and no movie category",
		},
		{
			name:           "repository error",
			mediaList:      []repository.DueItem{},
			repoError:      assert.AnError,
			expectedQueued: 0,
			description:    "Should handle repository errors gracefully",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			mockRepo := schedMocks.NewSchedulerRepository(s.T())
			scheduler := NewScheduler(mockRepo, logger)

			queue := make(chan repository.DueItem, 10)

			mockRepo.On("ResetStaleQueued", mock.Anything, mock.Anything).Return(int64(0), nil)
			mockRepo.On("GetDueMedia", mock.Anything, mock.Anything).Return(tt.mediaList, tt.repoError)
			if tt.expectQueued {
				mockRepo.On("MarkQueued", mock.Anything, mock.Anything).Return(nil)
			}

			scheduler.Start(queue)

			timeout := time.After(150 * time.Millisecond)
			queuedCount := 0
		collecting:
			for {
				select {
				case <-queue:
					queuedCount++
				case <-timeout:
					break collecting
				}
			}

			s.Equal(tt.expectedQueued, queuedCount, tt.description)
		})
	}
}

func (s *SchedulerTestSuite) TestStop_Idempotent() {
	s.scheduler.Stop()
	s.NotPanics(func() {
		s.scheduler.Stop()
	}, "Stop() must be safe to call multiple times")
}

func (s *SchedulerTestSuite) TestStart_MixedDueAndFutureMedia() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := schedMocks.NewSchedulerRepository(s.T())
	scheduler := NewScheduler(mockRepo, logger)

	mediaList := []repository.DueItem{
		dueItem("Already Due", time.Now().AddDate(0, 0, -1).Format("2006-01-02")),
		dueItem("Future", time.Now().AddDate(0, 0, 1).Format("2006-01-02")),
	}

	mockRepo.On("ResetStaleQueued", mock.Anything, mock.Anything).Return(int64(0), nil)
	mockRepo.On("GetDueMedia", mock.Anything, mock.Anything).Return(mediaList, nil)
	mockRepo.On("MarkQueued", mock.Anything, mock.Anything).Return(nil)

	queue := make(chan repository.DueItem, 10)
	scheduler.Start(queue)

	timeout := time.After(100 * time.Millisecond)
	queuedCount := 0
collecting:
	for {
		select {
		case <-queue:
			queuedCount++
		case <-timeout:
			break collecting
		}
	}

	s.Equal(1, queuedCount, "Should queue only the already-due media, not future media")
	scheduler.Stop()
}
