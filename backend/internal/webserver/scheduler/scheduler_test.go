package scheduler

import (
	"log/slog"
	"os"
	"testing"
	"time"

	repoMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SchedulerTestSuite struct {
	suite.Suite
	scheduler *Scheduler
	mockRepo  *repoMocks.SchedulerRepository
	logger    *slog.Logger
	queue     chan tvdb.Media
}

func TestSchedulerSuite(t *testing.T) {
	suite.Run(t, new(SchedulerTestSuite))
}

func (s *SchedulerTestSuite) SetupTest() {
	s.logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	s.mockRepo = repoMocks.NewSchedulerRepository(s.T())
	s.scheduler = NewScheduler(s.mockRepo, s.logger)
	s.queue = make(chan tvdb.Media, 10)
}

func (s *SchedulerTestSuite) TestNewScheduler() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := repoMocks.NewSchedulerRepository(s.T())

	scheduler := NewScheduler(mockRepo, logger)

	s.NotNil(scheduler)
	s.Equal(mockRepo, scheduler.repo)
	s.Equal(logger, scheduler.logger)
}

func (s *SchedulerTestSuite) TestStart_VariousScenarios() {
	tests := []struct {
		name           string
		mediaList      []tvdb.Media
		repoError      error
		expectedQueued int
		description    string
	}{
		{
			name: "media due - immediate send",
			mediaList: []tvdb.Media{
				{
					Name: "Test Show",
				},
			},
			repoError:      nil,
			expectedQueued: 1,
			description:    "Should immediately queue due media returned by repository",
		},
		{
			name:           "no due media",
			mediaList:      []tvdb.Media{},
			repoError:      nil,
			expectedQueued: 0,
			description:    "Should not queue when repository returns no due media",
		},
		{
			name:           "repository error",
			mediaList:      []tvdb.Media{},
			repoError:      assert.AnError,
			expectedQueued: 0,
			description:    "Should handle repository errors gracefully",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			mockRepo := repoMocks.NewSchedulerRepository(s.T())
			scheduler := NewScheduler(mockRepo, logger)

			queue := make(chan tvdb.Media, 10)

			mockRepo.On("GetDueMedia", mock.Anything, mock.Anything).Return(tt.mediaList, tt.repoError)

			scheduler.Start(queue)

			// Collect items with timeout to avoid race conditions
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

func (s *SchedulerTestSuite) TestStart_QueuesAllDueMedia() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := repoMocks.NewSchedulerRepository(s.T())
	scheduler := NewScheduler(mockRepo, logger)

	mediaList := []tvdb.Media{
		{Name: "Show 1"},
		{Name: "Show 2"},
	}

	mockRepo.On("GetDueMedia", mock.Anything, mock.Anything).Return(mediaList, nil)

	queue := make(chan tvdb.Media, 10)
	scheduler.Start(queue)

	// Give time for scheduler to process
	time.Sleep(100 * time.Millisecond)

	queuedCount := 0
	timeout := time.After(50 * time.Millisecond)
collecting:
	for {
		select {
		case <-queue:
			queuedCount++
		case <-timeout:
			break collecting
		}
	}

	s.Equal(2, queuedCount, "All due media returned by repository should be queued")
	// Verify scheduler created timers by checking it doesn't error
	scheduler.Stop()
}

func (s *SchedulerTestSuite) TestStart_RepoCalledWithCurrentTime() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := repoMocks.NewSchedulerRepository(s.T())
	scheduler := NewScheduler(mockRepo, logger)

	mockRepo.On("GetDueMedia", mock.Anything, mock.AnythingOfType("time.Time")).Return([]tvdb.Media{}, nil)

	queue := make(chan tvdb.Media, 10)
	scheduler.Start(queue)

	time.Sleep(100 * time.Millisecond)
	mockRepo.AssertNumberOfCalls(s.T(), "GetDueMedia", 1)
	scheduler.Stop()
}
