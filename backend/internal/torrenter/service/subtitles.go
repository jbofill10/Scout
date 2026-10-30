package service

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// SubtitleProvider is a subtitle source (OpenSubtitles today). Search is
// cheap and unauthenticated; Download spends the daily quota.
type SubtitleProvider interface {
	Search(ctx context.Context, params models.SubtitleSearchParams) ([]models.SubtitleCandidate, error)
	Download(ctx context.Context, fileID int) (data []byte, remaining int, err error)
}

// PlexRefresher asks Plex to rescan one directory, so a subtitle written
// after the video was picked up shows in the player without a full scan.
type PlexRefresher interface {
	RefreshPath(ctx context.Context, isMovie bool, dir string) error
}

// SubtitleSvc fetches subtitles for freshly linked media. The flow is
// deterministic: hash the file, ask the provider by id (plus hash), score
// what comes back, download the best candidate per language, sanity check it
// against the runtime, and write it beside the video with Plex's naming.
type SubtitleSvc struct {
	cfg      *models.SubtitlesCfg
	provider SubtitleProvider
	fs       FileSystem
	plex     PlexRefresher
	logger   *slog.Logger
	// quotaExhaustedUntil stops spending searches once the provider has said
	// the daily download limit is reached; it resets on the next UTC day.
	quotaExhaustedUntil time.Time
	now                 func() time.Time
	// settings, when set, overrides cfg.Languages and cfg.PreferSDH at runtime.
	settings SubtitleSettings
}

// SubtitleSettings is the slice of the runtime settings subtitle fetching needs.
type SubtitleSettings interface {
	SubtitleLanguages() []string
	SubtitlePreferSDH() bool
}

func NewSubtitleSvc(cfg *models.SubtitlesCfg, provider SubtitleProvider, fs FileSystem, plex PlexRefresher,
	logger *slog.Logger) *SubtitleSvc {
	return &SubtitleSvc{cfg: cfg, provider: provider, fs: fs, plex: plex, logger: logger, now: time.Now}
}

// SetSettings makes language and SDH choices follow the runtime settings.
func (s *SubtitleSvc) SetSettings(settings SubtitleSettings) {
	s.settings = settings
}

func (s *SubtitleSvc) languages() []string {
	if s.settings != nil {
		return s.settings.SubtitleLanguages()
	}
	return s.cfg.Languages
}

func (s *SubtitleSvc) preferSDH() bool {
	if s.settings != nil {
		return s.settings.SubtitlePreferSDH()
	}
	return s.cfg.PreferSDH
}

// Fetch finds and writes one subtitle per configured language for req.
// Languages that already have a file beside the video are left alone. The
// returned error covers only failures that stopped the whole fetch; a
// language with no acceptable candidate is logged and skipped.
func (s *SubtitleSvc) Fetch(ctx context.Context, req models.SubtitleRequest) ([]models.SubtitleResult, error) {
	if len(s.languages()) == 0 {
		return nil, nil
	}
	if until := s.quotaExhaustedUntil; !until.IsZero() && s.now().Before(until) {
		return nil, fmt.Errorf("subtitle download quota exhausted until %s", until.Format(time.RFC3339))
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	ctx, span := tracer.Start(ctx, "fetch_subtitles")
	defer span.End()

	wanted := s.languagesMissing(req.LibraryPath)
	if len(wanted) == 0 {
		s.logger.InfoContext(ctx, "Subtitles already present, nothing to fetch", "path", req.LibraryPath)
		return nil, nil
	}

	hash, size, err := OpenSubtitlesHash(req.VideoPath)
	if err != nil {
		// A hash only sharpens matching; id and title searches still work.
		s.logger.WarnContext(ctx, "Could not hash video for subtitle lookup", "path", req.VideoPath, "error", err)
		hash = ""
	}

	candidates, err := s.search(ctx, req, wanted, hash)
	if err != nil {
		return nil, err
	}
	s.logger.InfoContext(ctx, "Subtitle search finished",
		telemetry.WithTraceContext(ctx, "title", req.Title, "season", req.Season, "episode", req.Episode,
			"imdb_id", req.ImdbID, "hash", hash, "size", size, "candidates", len(candidates))...)

	var results []models.SubtitleResult
	for _, lang := range wanted {
		ranked := rankCandidates(candidates, req, lang, s.preferSDH())
		if len(ranked) == 0 {
			s.logger.WarnContext(ctx, "No subtitle candidates for language", "language", lang, "title", req.Title,
				"season", req.Season, "episode", req.Episode)
			continue
		}
		res, err := s.downloadBest(ctx, req, lang, ranked)
		if err != nil {
			var apiErr interface{ QuotaExceeded() bool }
			if errors.As(err, &apiErr) && apiErr.QuotaExceeded() {
				s.quotaExhaustedUntil = nextUTCDay(s.now())
				return results, fmt.Errorf("subtitle download quota exceeded; pausing until %s: %w",
					s.quotaExhaustedUntil.Format(time.RFC3339), err)
			}
			s.logger.WarnContext(ctx, "No usable subtitle downloaded", "language", lang, "title", req.Title, "error", err)
			continue
		}
		results = append(results, *res)
	}

	if len(results) > 0 && s.plex != nil {
		if err := s.plex.RefreshPath(ctx, req.IsMovie, filepath.Dir(req.LibraryPath)); err != nil {
			s.logger.WarnContext(ctx, "Plex refresh after subtitle write failed (non-fatal)", "error", err)
		}
	}
	return results, nil
}

// languagesMissing returns the configured languages that have no subtitle file
// beside the video yet, in either plain or SDH form.
func (s *SubtitleSvc) languagesMissing(libraryPath string) []string {
	var missing []string
	for _, lang := range s.languages() {
		if s.fs.Exists(subtitlePath(libraryPath, lang, false)) || s.fs.Exists(subtitlePath(libraryPath, lang, true)) {
			continue
		}
		missing = append(missing, lang)
	}
	return missing
}

// search asks the provider by exact id first (with the hash, so hash matches
// are flagged) and falls back to a title query when no id is known or the id
// search comes back empty.
func (s *SubtitleSvc) search(ctx context.Context, req models.SubtitleRequest, langs []string,
	hash string) ([]models.SubtitleCandidate, error) {
	byID := models.SubtitleSearchParams{Languages: langs, IsMovie: req.IsMovie, MovieHash: hash}
	if req.IsMovie {
		byID.ImdbID = req.ImdbID
	} else {
		byID.ParentImdbID = req.ImdbID
		byID.Season = req.Season
		byID.Episode = req.Episode
	}
	byTitle := models.SubtitleSearchParams{Languages: langs, IsMovie: req.IsMovie, MovieHash: hash,
		Query: req.Title, Year: req.Year, Season: req.Season, Episode: req.Episode}

	var (
		found []models.SubtitleCandidate
		err   error
	)
	if req.ImdbID != "" {
		found, err = s.provider.Search(ctx, byID)
		if err != nil {
			return nil, fmt.Errorf("subtitle search by id: %w", err)
		}
		if len(found) > 0 {
			return found, nil
		}
		s.logger.InfoContext(ctx, "No subtitles by IMDb id, falling back to title search", "imdb_id", req.ImdbID)
	}
	if req.Title == "" {
		return nil, nil
	}
	found, err = s.provider.Search(ctx, byTitle)
	if err != nil {
		return nil, fmt.Errorf("subtitle search by title: %w", err)
	}
	return found, nil
}

// downloadBest walks the ranked candidates, downloading until one passes the
// sanity check or the per-item download cap is hit, then writes it.
func (s *SubtitleSvc) downloadBest(ctx context.Context, req models.SubtitleRequest, lang string,
	ranked []scoredCandidate) (*models.SubtitleResult, error) {
	var lastErr error
	limit := s.cfg.MaxDownloadsPerItem
	if limit <= 0 {
		limit = 1
	}
	for i, sc := range ranked {
		if i >= limit {
			break
		}
		data, remaining, err := s.provider.Download(ctx, sc.FileID)
		if err != nil {
			return nil, err
		}
		if err := CheckSubtitle(data, req.RuntimeMinutes, req.IsMovie); err != nil {
			s.logger.WarnContext(ctx, "Rejected subtitle candidate", "file_id", sc.FileID, "release", sc.Release,
				"score", sc.Score, "error", err)
			lastErr = err
			continue
		}
		path := subtitlePath(req.LibraryPath, lang, sc.HearingImpaired)
		if err := s.fs.WriteFile(path, data); err != nil {
			return nil, fmt.Errorf("write subtitle: %w", err)
		}
		s.logger.InfoContext(ctx, "Wrote subtitle",
			telemetry.WithTraceContext(ctx, "path", path, "language", lang, "release", sc.Release,
				"hash_match", sc.HashMatch, "sdh", sc.HearingImpaired, "score", sc.Score,
				"file_id", sc.FileID, "quota_remaining", remaining)...)
		return &models.SubtitleResult{
			Path: path, Language: lang, HearingImpaired: sc.HearingImpaired, HashMatch: sc.HashMatch,
			Release: sc.Release, FileID: sc.FileID, Remaining: remaining,
		}, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no candidate downloaded")
	}
	return nil, lastErr
}

// subtitlePath is Plex's sidecar naming: "<video>.<lang>.srt", with ".sdh"
// before the extension for hearing-impaired tracks.
func subtitlePath(libraryPath, lang string, sdh bool) string {
	base := strings.TrimSuffix(libraryPath, filepath.Ext(libraryPath))
	if sdh {
		return base + "." + lang + ".sdh.srt"
	}
	return base + "." + lang + ".srt"
}

// nextUTCDay is midnight UTC after t, when OpenSubtitles resets its quota.
func nextUTCDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day()+1, 0, 0, 0, 0, time.UTC)
}

// ---- scoring ----

type scoredCandidate struct {
	models.SubtitleCandidate
	Score float64
}

var releaseSplit = regexp.MustCompile(`[\s._\-\[\]()]+`)

// releaseTokens lower-cases a release name and splits it on scene separators,
// so "The.Movie.2014.1080p.BluRay.x264.YIFY" and "The Movie (2014) [1080p] [YTS.MX]"
// compare token by token.
func releaseTokens(name string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range releaseSplit.Split(strings.ToLower(name), -1) {
		if tok != "" {
			out[tok] = true
		}
	}
	return out
}

var (
	resolutionTokens = []string{"2160p", "1080p", "720p", "480p", "4k"}
	sourceTokens     = []string{"bluray", "brrip", "bdrip", "web", "webrip", "webdl", "web-dl", "hdtv", "dvdrip",
		"amzn", "nf"}
	codecTokens = []string{"x264", "x265", "h264", "h265", "hevc", "avc"}
	// groupTokens are the release groups whose rips share timing with each
	// other across resolutions, so a 720p YTS subtitle fits a 1080p YTS file.
	groupAliases = map[string]string{"yify": "yts", "yts": "yts", "mx": "yts", "ag": "yts", "am": "yts", "lt": "yts"}
)

func hasAny(tokens map[string]bool, candidates []string) string {
	for _, c := range candidates {
		if tokens[c] {
			return c
		}
	}
	return ""
}

func releaseGroup(tokens map[string]bool) string {
	for tok := range tokens {
		if g, ok := groupAliases[tok]; ok {
			return g
		}
	}
	return ""
}

// scoreCandidate rates how well c fits req. ok is false when the candidate
// must not be used at all: wrong language, wrong episode, machine output,
// foreign-parts-only. Higher is better; a provider hash match dominates, then
// release-name agreement, then popularity. SDH is a mild penalty unless asked
// for, so it still wins when it is the only option.
func scoreCandidate(c models.SubtitleCandidate, req models.SubtitleRequest, lang string,
	preferSDH bool) (float64, bool) {
	if !strings.EqualFold(c.Language, lang) {
		return 0, false
	}
	if c.MachineTranslated || c.AITranslated || c.ForeignPartsOnly {
		return 0, false
	}
	if !req.IsMovie {
		if c.Season != 0 && c.Season != req.Season {
			return 0, false
		}
		if c.Episode != 0 && c.Episode != req.Episode {
			return 0, false
		}
	}

	score := 0.0
	if c.HashMatch {
		score += 100
	}

	want := releaseTokens(req.ReleaseName)
	have := releaseTokens(c.Release)
	if len(want) > 0 && len(have) > 0 {
		if g := releaseGroup(want); g != "" && g == releaseGroup(have) {
			score += 40
		}
		if r := hasAny(want, resolutionTokens); r != "" && have[r] {
			score += 10
		}
		if src := hasAny(want, sourceTokens); src != "" && have[src] {
			score += 10
		}
		if cod := hasAny(want, codecTokens); cod != "" && have[cod] {
			score += 5
		}
	}

	if c.FromTrusted {
		score += 5
	}
	score += 3 * math.Log10(float64(c.DownloadCount)+1)
	score += c.Ratings / 2

	if c.HearingImpaired != preferSDH {
		score -= 15
	}
	return score, true
}

// rankCandidates filters to usable candidates for lang and sorts them best
// first, breaking ties by download count so the order is stable.
func rankCandidates(cands []models.SubtitleCandidate, req models.SubtitleRequest, lang string,
	preferSDH bool) []scoredCandidate {
	var ranked []scoredCandidate
	for _, c := range cands {
		if score, ok := scoreCandidate(c, req, lang, preferSDH); ok {
			ranked = append(ranked, scoredCandidate{SubtitleCandidate: c, Score: score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].DownloadCount > ranked[j].DownloadCount
	})
	return ranked
}

// ---- sanity check ----

var srtTimestamp = regexp.MustCompile(`(\d+):(\d{2}):(\d{2})[,.](\d{1,3})\s*-->`)

// CheckSubtitle rejects a downloaded file that is not a plausible SRT for a
// video of runtimeMinutes: too few cues, or a last cue far outside the
// runtime (a subtitle for a different cut, or a different episode). With an
// unknown runtime only the cue count is checked.
func CheckSubtitle(data []byte, runtimeMinutes int, isMovie bool) error {
	matches := srtTimestamp.FindAllSubmatch(data, -1)
	minCues := 20
	if isMovie {
		minCues = 50
	}
	if len(matches) < minCues {
		return fmt.Errorf("only %d cues, expected at least %d", len(matches), minCues)
	}
	if runtimeMinutes <= 0 {
		return nil
	}
	last := matches[len(matches)-1]
	lastSec := atoi(last[1])*3600 + atoi(last[2])*60 + atoi(last[3])
	runtime := runtimeMinutes * 60
	// Credits usually run a few minutes with no dialogue, so the last cue
	// landing anywhere in the back half is fine; past the runtime is not.
	if lastSec > runtime+120 {
		return fmt.Errorf("last cue at %ds is past the %ds runtime", lastSec, runtime)
	}
	if lastSec < runtime/2 {
		return fmt.Errorf("last cue at %ds covers under half of the %ds runtime", lastSec, runtime)
	}
	return nil
}

func atoi(b []byte) int {
	n := 0
	for _, ch := range b {
		n = n*10 + int(ch-'0')
	}
	return n
}

// ---- hashing ----

const hashChunkSize = 65536

// OpenSubtitlesHash computes the OpenSubtitles file hash: the file size plus
// the sum of the first and last 64 KiB read as little-endian uint64s, mod
// 2^64. It is what lets the provider match a subtitle to this exact rip.
func OpenSubtitlesHash(path string) (hash string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	size = info.Size()
	if size < hashChunkSize*2 {
		return "", size, fmt.Errorf("file too small to hash (%d bytes)", size)
	}
	h, err := hashReader(f, size)
	if err != nil {
		return "", size, err
	}
	return fmt.Sprintf("%016x", h), size, nil
}

func hashReader(r io.ReadSeeker, size int64) (uint64, error) {
	sum := uint64(size)
	buf := make([]byte, hashChunkSize)
	for _, offset := range []int64{0, size - hashChunkSize} {
		if _, err := r.Seek(offset, io.SeekStart); err != nil {
			return 0, err
		}
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, err
		}
		for i := 0; i < hashChunkSize; i += 8 {
			sum += binary.LittleEndian.Uint64(buf[i : i+8])
		}
	}
	return sum, nil
}
