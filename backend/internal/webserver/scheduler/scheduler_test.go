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
			name: "media already due - immediate send",
			mediaList: []tvdb.Media{
				{
					Name: "Test Show",
					Metadata: tvdb.TVDBSeriesMetadata{
						Episodes: []tvdb.Episode{
							{Aired: time.Now().AddDate(0, 0, -1).Format("2006-01-02")},
						},
					},
				},
			},
			repoError:      nil,
			expectedQueued: 1,
			description:    "Should immediately queue media that has already aired",
		},
		{
			name: "media in future - scheduled send",
			mediaList: []tvdb.Media{
				{
					Name: "Future Show",
					Metadata: tvdb.TVDBSeriesMetadata{
						Episodes: []tvdb.Episode{
							{Aired: time.Now().AddDate(0, 0, 2).Format("2006-01-02")},
						},
					},
				},
			},
			repoError:      nil,
			expectedQueued: 0, // Won't queue - outside 24hr polling window
			description:    "Should schedule media for future release (beyond polling window)",
		},
		{
			name: "invalid aired date",
			mediaList: []tvdb.Media{
				{
					Name: "Invalid Show",
					Metadata: tvdb.TVDBSeriesMetadata{
						Episodes: []tvdb.Episode{
							{Aired: "invalid-date"},
						},
					},
				},
			},
			repoError:      nil,
			expectedQueued: 0,
			description:    "Should skip media with invalid aired date",
		},
		{
			name: "empty episodes",
			mediaList: []tvdb.Media{
				{Name: "Empty Show", Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}},
			},
			repoError:      nil,
			expectedQueued: 0,
			description:    "Should not queue media with no episodes",
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

func (s *SchedulerTestSuite) TestStart_MultipleFutureMedia() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := repoMocks.NewSchedulerRepository(s.T())
	scheduler := NewScheduler(mockRepo, logger)

	// Create media with staggered future release times (2 and 3 days ahead, beyond 24hr window)
	mediaList := []tvdb.Media{
		{
			Name: "Show 1",
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{{Aired: time.Now().AddDate(0, 0, 2).Format("2006-01-02")}},
			},
		},
		{
			Name: "Show 2",
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{{Aired: time.Now().AddDate(0, 0, 3).Format("2006-01-02")}},
			},
		},
	}

	mockRepo.On("GetDueMedia", mock.Anything, mock.Anything).Return(mediaList, nil)

	queue := make(chan tvdb.Media, 10)
	scheduler.Start(queue)

	// Give time for scheduler to process and create timers
	time.Sleep(100 * time.Millisecond)

	// Since both are tomorrow/day after, they won't queue during this test
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

	s.Equal(0, queuedCount, "Future media should be scheduled but not queued immediately")
	// Verify scheduler created timers by checking it doesn't error
	scheduler.Stop()
}

func (s *SchedulerTestSuite) TestStart_MixedDueAndFutureMedia() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := repoMocks.NewSchedulerRepository(s.T())
	scheduler := NewScheduler(mockRepo, logger)

	mediaList := []tvdb.Media{
		{
			Name: "Already Due",
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{{Aired: time.Now().AddDate(0, 0, -1).Format("2006-01-02")}},
			},
		},
		{
			Name: "Future",
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{{Aired: time.Now().AddDate(0, 0, 1).Format("2006-01-02")}},
			},
		},
	}

	mockRepo.On("GetDueMedia", mock.Anything, mock.Anything).Return(mediaList, nil)

	queue := make(chan tvdb.Media, 10)
	scheduler.Start(queue)

	// Collect items with timeout
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
