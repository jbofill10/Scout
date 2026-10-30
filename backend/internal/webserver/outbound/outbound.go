// Package outbound delivers download outcomes to places outside the app:
// generic webhooks (Home Assistant among them), ntfy topics and Discord
// webhooks. A dispatcher polls the Notifications table for terminal status
// transitions that have not been delivered yet and posts each one to every
// enabled target that wants that event.
package outbound

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/notifications"
)

// Event is one outcome as targets receive it.
type Event struct {
	Event           string    `json:"event"`
	Status          string    `json:"status"`
	Title           string    `json:"title"`
	Message         string    `json:"message"`
	TvdbID          string    `json:"tvdb_id"`
	MediaTitle      string    `json:"media_title"`
	Category        string    `json:"category"`
	Season          *int      `json:"season,omitempty"`
	Episode         *int      `json:"episode,omitempty"`
	AbsoluteEpisode *int      `json:"absolute_episode,omitempty"`
	IsAnime         bool      `json:"is_anime"`
	PosterURL       string    `json:"poster_url,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	OccurredAt      time.Time `json:"occurred_at"`
	Test            bool      `json:"test,omitempty"`
}

// BuildEvent words a notification for the outside world.
func BuildEvent(n notifications.Notification) Event {
	ev := Event{
		Event:           string(n.Status),
		Status:          string(n.Status),
		TvdbID:          n.TvdbID,
		MediaTitle:      n.MediaTitle,
		Category:        n.Category,
		Season:          n.Season,
		Episode:         n.Episode,
		AbsoluteEpisode: n.AbsoluteEpisode,
		IsAnime:         n.IsAnime,
		PosterURL:       n.PosterURL,
		Reason:          n.Reason,
		OccurredAt:      n.UpdatedAt,
	}
	ev.Title = itemTitle(n)
	switch n.Status {
	case notifications.StatusCompleted:
		ev.Message = "Downloaded and in your library"
		if n.Reason != "" {
			ev.Message = n.Reason
		}
	case notifications.StatusFailed:
		ev.Message = "Gave up"
		if n.Reason != "" {
			ev.Message = n.Reason
		}
	case notifications.StatusCancelled:
		ev.Message = "Cancelled"
	default:
		ev.Message = string(n.Status)
	}
	return ev
}

// itemTitle is "Show S02E03", "Show Episode 12" for anime, or the movie name.
func itemTitle(n notifications.Notification) string {
	if n.Category != "series" {
		return n.MediaTitle
	}
	if n.IsAnime && n.AbsoluteEpisode != nil && *n.AbsoluteEpisode > 0 {
		return fmt.Sprintf("%s Episode %d", n.MediaTitle, *n.AbsoluteEpisode)
	}
	if n.Season != nil && n.Episode != nil {
		return fmt.Sprintf("%s S%02dE%02d", n.MediaTitle, *n.Season, *n.Episode)
	}
	return n.MediaTitle
}

// SampleEvent is what a target test sends.
func SampleEvent(now time.Time) Event {
	season, episode := 1, 1
	return Event{
		Event:      "completed",
		Status:     "completed",
		Title:      "Scout test S01E01",
		Message:    "This is a test from Scout. Deliveries are working.",
		TvdbID:     "0",
		MediaTitle: "Scout test",
		Category:   "series",
		Season:     &season,
		Episode:    &episode,
		OccurredAt: now,
		Test:       true,
	}
}

// Sender delivers one event to one target.
type Sender interface {
	Send(ctx context.Context, target repository.NotificationTarget, ev Event) error
}

// HTTPSender posts events over HTTP in the shape each kind expects.
type HTTPSender struct {
	client *http.Client
}

func NewHTTPSender() *HTTPSender {
	return &HTTPSender{client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *HTTPSender) Send(ctx context.Context, target repository.NotificationTarget, ev Event) error {
	var body []byte
	headers := map[string]string{}
	switch target.Kind {
	case repository.TargetWebhook:
		raw, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		body = raw
		headers["Content-Type"] = "application/json"
	case repository.TargetNtfy:
		body = []byte(ev.Message)
		headers["Content-Type"] = "text/plain; charset=utf-8"
		headers["Title"] = "Scout: " + ev.Title
		headers["Tags"] = ntfyTag(ev.Status)
		headers["Priority"] = ntfyPriority(ev.Status)
	case repository.TargetDiscord:
		raw, err := json.Marshal(map[string]string{"content": fmt.Sprintf("**Scout** · %s\n%s", ev.Title, ev.Message)})
		if err != nil {
			return err
		}
		body = raw
		headers["Content-Type"] = "application/json"
	default:
		return fmt.Errorf("unknown target kind %q", target.Kind)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if target.Token != "" {
		req.Header.Set("Authorization", "Bearer "+target.Token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("%s answered %d: %s", target.Kind, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

func ntfyTag(status string) string {
	switch status {
	case "completed":
		return "white_check_mark"
	case "failed":
		return "x"
	case "cancelled":
		return "no_entry_sign"
	}
	return "information_source"
}

func ntfyPriority(status string) string {
	if status == "failed" {
		return "4"
	}
	return "3"
}

// pendingRepo is the slice of the notification repository the dispatcher uses.
type pendingRepo interface {
	PendingDeliveries(ctx context.Context, limit int) ([]notifications.Notification, error)
	MarkNotified(ctx context.Context, id int, status string) error
}

// targetRepo is the slice of the target repository the dispatcher uses.
type targetRepo interface {
	List(ctx context.Context) ([]repository.NotificationTarget, error)
	RecordResult(ctx context.Context, id int, sentAt time.Time, errMsg string) error
}

// Dispatcher turns undelivered terminal notifications into deliveries.
type Dispatcher struct {
	notifs   pendingRepo
	targets  targetRepo
	sender   Sender
	interval time.Duration
	logger   *slog.Logger
	now      func() time.Time
	batch    int
}

func NewDispatcher(notifs pendingRepo, targets targetRepo, sender Sender, interval time.Duration, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{notifs: notifs, targets: targets, sender: sender, interval: interval, logger: logger, now: time.Now, batch: 50}
}

// Start polls until ctx ends.
func (d *Dispatcher) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(d.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if _, err := d.RunOnce(ctx); err != nil {
					d.logger.ErrorContext(ctx, "Outbound notification poll failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// RunOnce delivers every pending transition once and returns how many
// notifications were processed. A target that fails is recorded and skipped;
// the notification is still marked delivered, so a dead target never stalls
// the queue or causes repeats elsewhere.
func (d *Dispatcher) RunOnce(ctx context.Context) (int, error) {
	pending, err := d.notifs.PendingDeliveries(ctx, d.batch)
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}
	targets, err := d.targets.List(ctx)
	if err != nil {
		return 0, err
	}

	for _, n := range pending {
		ev := BuildEvent(n)
		for _, t := range targets {
			if !t.Enabled || !t.Wants(ev.Event) {
				continue
			}
			sendErr := d.sender.Send(ctx, t, ev)
			errMsg := ""
			if sendErr != nil {
				errMsg = sendErr.Error()
				d.logger.WarnContext(ctx, "Outbound notification failed", "target", t.Name, "kind", t.Kind,
					"event", ev.Event, "title", ev.Title, "error", sendErr)
			} else {
				d.logger.InfoContext(ctx, "Outbound notification sent", "target", t.Name, "kind", t.Kind,
					"event", ev.Event, "title", ev.Title)
			}
			if err := d.targets.RecordResult(ctx, t.ID, d.now(), errMsg); err != nil {
				d.logger.ErrorContext(ctx, "Failed to record delivery result", "error", err, "target", t.Name)
			}
		}
		if err := d.notifs.MarkNotified(ctx, n.ID, string(n.Status)); err != nil {
			d.logger.ErrorContext(ctx, "Failed to mark notification delivered", "error", err, "id", n.ID)
		}
	}
	return len(pending), nil
}

// Test sends a sample event to one target and records the outcome.
func (d *Dispatcher) Test(ctx context.Context, target repository.NotificationTarget) error {
	err := d.sender.Send(ctx, target, SampleEvent(d.now()))
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	if recErr := d.targets.RecordResult(ctx, target.ID, d.now(), errMsg); recErr != nil {
		d.logger.ErrorContext(ctx, "Failed to record test result", "error", recErr, "target", target.Name)
	}
	return err
}
