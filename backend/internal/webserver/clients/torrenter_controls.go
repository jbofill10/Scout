package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// ErrItemUnknown is returned when the torrenter has no media for an item and
// none was supplied, so it cannot search for or download anything.
var ErrItemUnknown = errors.New("torrenter has no media for item")

// Cancel asks the torrenter to stop everything in flight for an item.
func (c *TorrenterClient) Cancel(ctx context.Context, itemID string) (controls.CancelResponse, error) {
	var out controls.CancelResponse
	_, err := c.postJSON(ctx, "/downloads/cancel", controls.CancelRequest{ItemID: itemID}, &out)
	return out, err
}

// ListReleases asks the torrenter for every candidate release of an item.
// media may be nil, in which case the torrenter resolves it from its in-flight
// torrent for the item and answers ErrItemUnknown when it has none.
func (c *TorrenterClient) ListReleases(ctx context.Context, itemID string, media *tvdb.Media) (controls.ReleasesResponse, error) {
	var out controls.ReleasesResponse
	status, err := c.postJSON(ctx, "/releases", controls.ReleasesRequest{ItemID: itemID, Media: media}, &out)
	if status == http.StatusNotFound {
		return out, ErrItemUnknown
	}
	return out, err
}

// ManualDownload asks the torrenter to download exactly the chosen release.
func (c *TorrenterClient) ManualDownload(ctx context.Context, itemID string, media *tvdb.Media, sel controls.ReleaseSelection) (dlstatus.EpisodeResult, error) {
	var out dlstatus.EpisodeResult
	status, err := c.postJSON(ctx, "/downloads/manual",
		controls.ManualDownloadRequest{ItemID: itemID, Media: media, Release: sel}, &out)
	if status == http.StatusNotFound {
		return out, ErrItemUnknown
	}
	return out, err
}

// postJSON posts body to the torrenter and decodes a 200 response into out.
// It returns the HTTP status (0 on transport failure) alongside any error, so
// callers can map specific statuses without parsing error strings.
func (c *TorrenterClient) postJSON(ctx context.Context, path string, body any, out any) (int, error) {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return 0, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s%s", c.Host, path), buf)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("torrenter unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return resp.StatusCode, fmt.Errorf("torrenter returned status %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode torrenter response: %w", err)
	}
	return resp.StatusCode, nil
}
