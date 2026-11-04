package ratings

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	// DefaultIMDbRatingsURL is IMDb's public ratings table, refreshed daily.
	DefaultIMDbRatingsURL = "https://datasets.imdbws.com/title.ratings.tsv.gz"
	// DatasetRefresh is how often the table is downloaded again.
	DatasetRefresh = 24 * time.Hour
	// DatasetRetry is how soon a failed download is tried again.
	DatasetRetry = 10 * time.Minute

	datasetTimeout = 3 * time.Minute
	// datasetMaxBytes bounds the decompressed table; it is about 25 MB.
	datasetMaxBytes = 256 << 20
	// datasetRows is the expected row count, used to size the table once.
	datasetRows = 1_600_000
)

// Dataset holds IMDb's title.ratings table in memory: every rated title's
// average rating and vote count, keyed by IMDb id. It needs no key, so it
// is the baseline every card can show; the table is about 1.5 million rows
// and takes some 20 MB packed as sorted fixed-size rows.
type Dataset struct {
	url    string
	http   *http.Client
	logger *slog.Logger

	mu     sync.RWMutex
	rows   []imdbRow
	loaded time.Time
}

// imdbRow is one title: the numeric part of its id, its rating in tenths
// and its vote count.
type imdbRow struct {
	id     uint32
	votes  uint32
	rating uint16
}

// NewDatasetHTTPClient returns the traced HTTP client used to download the table.
func NewDatasetHTTPClient() *http.Client {
	return &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: datasetTimeout}
}

// NewDataset returns an empty dataset that downloads from url (empty means
// DefaultIMDbRatingsURL) when loaded.
func NewDataset(url string, httpClient *http.Client, logger *slog.Logger) *Dataset {
	if url == "" {
		url = DefaultIMDbRatingsURL
	}
	return &Dataset{url: url, http: httpClient, logger: logger}
}

// Loaded reports whether a table is held.
func (d *Dataset) Loaded() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.rows) > 0
}

// Rating returns the IMDb rating for a title id, false when the table has
// no row for it (or is not loaded yet).
func (d *Dataset) Rating(imdbID string) (tvdb.Rating, bool) {
	id, ok := parseIMDbID(imdbID)
	if !ok {
		return tvdb.Rating{}, false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	i := sort.Search(len(d.rows), func(i int) bool { return d.rows[i].id >= id })
	if i == len(d.rows) || d.rows[i].id != id {
		return tvdb.Rating{}, false
	}
	row := d.rows[i]
	return tvdb.Rating{
		Source: tvdb.RatingSourceIMDb,
		Value:  float64(row.rating) / 10,
		Votes:  int(row.votes),
	}, true
}

// Load downloads and parses the table, replacing the one held. A failed
// download leaves the previous table in place.
func (d *Dataset) Load(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, datasetTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, nil)
	if err != nil {
		return err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("imdb dataset: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			d.logger.InfoContext(ctx, "Error closing IMDb dataset body", "error", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("imdb dataset: status %d", resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("imdb dataset: gunzip: %w", err)
	}
	started := time.Now()
	rows, err := parseDataset(io.LimitReader(gz, datasetMaxBytes))
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.rows, d.loaded = rows, time.Now()
	d.mu.Unlock()
	d.logger.InfoContext(ctx, "IMDb ratings table loaded", "rows", len(rows), "took", time.Since(started).String())
	return nil
}

// Start keeps the table fresh in the background: a table not loaded yet is
// tried every DatasetRetry, a loaded one is replaced every DatasetRefresh.
// It returns at once and stops when ctx ends.
func (d *Dataset) Start(ctx context.Context) {
	go func() {
		for {
			wait := DatasetRefresh
			if !d.Loaded() {
				wait = DatasetRetry
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if err := d.Load(ctx); err != nil && ctx.Err() == nil {
				d.logger.WarnContext(ctx, "IMDb ratings table refresh failed", "error", err)
			}
		}
	}()
}

// parseDataset reads the TSV (tconst, averageRating, numVotes) into rows
// sorted by id. Rows that do not parse are skipped; an empty result is an error.
func parseDataset(r io.Reader) ([]imdbRow, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	rows := make([]imdbRow, 0, datasetRows)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			if strings.HasPrefix(line, "tconst") {
				continue
			}
		}
		row, ok := parseDatasetLine(line)
		if ok {
			rows = append(rows, row)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("imdb dataset: read: %w", err)
	}
	if len(rows) == 0 {
		return nil, errors.New("imdb dataset: no rows")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	return rows, nil
}

// parseDatasetLine parses "tt0111161\t9.3\t2900000".
func parseDatasetLine(line string) (imdbRow, bool) {
	idText, rest, ok := strings.Cut(line, "\t")
	if !ok {
		return imdbRow{}, false
	}
	ratingText, votesText, ok := strings.Cut(rest, "\t")
	if !ok {
		return imdbRow{}, false
	}
	id, ok := parseIMDbID(idText)
	if !ok {
		return imdbRow{}, false
	}
	rating, err := strconv.ParseFloat(ratingText, 64)
	if err != nil || rating < 0 || rating > 10 {
		return imdbRow{}, false
	}
	votes, err := strconv.ParseUint(strings.TrimSpace(votesText), 10, 32)
	if err != nil {
		return imdbRow{}, false
	}
	return imdbRow{id: id, rating: uint16(rating*10 + 0.5), votes: uint32(votes)}, true
}

// parseIMDbID turns "tt0111161" into 111161.
func parseIMDbID(imdbID string) (uint32, bool) {
	digits, ok := strings.CutPrefix(strings.TrimSpace(imdbID), "tt")
	if !ok || digits == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(digits, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(id), true
}
