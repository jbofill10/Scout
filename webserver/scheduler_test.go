package main

import (
	"log"
	"os"
	"testing"
	"time"

	tvdb "shared/media"
	"webserver/mocks"

	"github.com/stretchr/testify/assert"
)

func TestNewScheduler(t *testing.T) {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	mockRepo := mocks.NewSchedulerRepository(t)

	scheduler := NewScheduler(mockRepo, logger)

	assert.NotNil(t, scheduler)
	assert.Equal(t, mockRepo, scheduler.repo)
	assert.Equal(t, logger, scheduler.logger)
}

func TestScheduler_Start(t *testing.T) {
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
							{Aired: time.Now().Add(50 * time.Millisecond).Format("2006-01-02")},
						},
					},
				},
			},
			repoError:      nil,
			expectedQueued: 1,
			description:    "Should schedule media for future release",
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
		t.Run(tt.name, func(t *testing.T) {
			logger := log.New(os.Stdout, "", log.LstdFlags)
			mockRepo := mocks.NewSchedulerRepository(t)
			scheduler := NewScheduler(mockRepo, logger)

			queue := make(chan tvdb.Media, 10)

			mockRepo.On("GetDueMedia").Return(tt.mediaList, tt.repoError)

			scheduler.Start(queue)

			// Give goroutines time to execute
			time.Sleep(100 * time.Millisecond)

			close(queue)

			// Count queued items
			queuedCount := 0
			for range queue {
				queuedCount++
			}

			assert.Equal(t, tt.expectedQueued, queuedCount, tt.description)
		})
	}
}

func TestScheduler_Start_MultipleFutureMedia(t *testing.T) {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	mockRepo := mocks.NewSchedulerRepository(t)
	scheduler := NewScheduler(mockRepo, logger)

	// Create media with staggered future release times
	mediaList := []tvdb.Media{
		{
			Name: "Show 1",
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{{Aired: time.Now().Add(50 * time.Millisecond).Format("2006-01-02")}},
			},
		},
		{
			Name: "Show 2",
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{{Aired: time.Now().Add(100 * time.Millisecond).Format("2006-01-02")}},
			},
		},
	}

	mockRepo.On("GetDueMedia").Return(mediaList, nil)

	queue := make(chan tvdb.Media, 10)
	scheduler.Start(queue)

	// Wait for all scheduled items
	time.Sleep(200 * time.Millisecond)

	close(queue)

	queuedCount := 0
	for range queue {
		queuedCount++
	}

	assert.Equal(t, 2, queuedCount, "Should queue all future media after their release time")
}

func TestScheduler_Start_MixedDueAndFutureMedia(t *testing.T) {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	mockRepo := mocks.NewSchedulerRepository(t)
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
				Episodes: []tvdb.Episode{{Aired: time.Now().Add(50 * time.Millisecond).Format("2006-01-02")}},
			},
		},
	}

	mockRepo.On("GetDueMedia").Return(mediaList, nil)

	queue := make(chan tvdb.Media, 10)
	scheduler.Start(queue)

	// Wait for all items
	time.Sleep(150 * time.Millisecond)

	close(queue)

	queuedCount := 0
	for range queue {
		queuedCount++
	}

	assert.Equal(t, 2, queuedCount, "Should queue both due and future media")
}
