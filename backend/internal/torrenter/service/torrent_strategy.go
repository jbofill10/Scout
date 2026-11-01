package service

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// searchRequestFor captures what is needed to search for one item again from
// scratch, so a dead download can be replaced later.
func searchRequestFor(req *tvdb.Media, ss *models.SearchStrategy) *models.SearchRequest {
	media := *req
	media.Metadata.Episodes = nil
	sr := &models.SearchRequest{Media: media}
	if ss != nil && ss.EpisodeMeta != nil && !ss.IsMovie {
		ep := *ss.EpisodeMeta
		sr.Episode = &ep
	}
	return sr
}

// maxSearchNames caps how many names (the title plus aliases) a search
// ladder is built from. TVDB lists per-season titles and other-script
// variants as aliases, and every name multiplies the queries sent to a
// rate-limited indexer: six names took one episode past the search deadline
// before the fallback indexer was ever asked.
const maxSearchNames = 4

// searchNames returns the distinct names to search a show under: the title
// first, then aliases in TVDB order with Latin-script ones ahead of the
// rest, capped at maxSearchNames. Names that differ only by case,
// punctuation or a trailing year count as one, and an alias written as
// "Title (Other-script title)" contributes both titles.
func searchNames(req *tvdb.Media) []string {
	seen := map[string]bool{}
	var latin, other []string
	add := func(name string) {
		name = strings.Join(strings.Fields(name), " ")
		if name == "" {
			return
		}
		key := nameKey(name)
		if seen[key] {
			return
		}
		seen[key] = true
		if hasLatinLetter(name) {
			latin = append(latin, name)
		} else {
			other = append(other, name)
		}
	}

	names := []string{}
	if primary := strings.Join(strings.Fields(req.Name), " "); primary != "" {
		seen[nameKey(primary)] = true
		names = append(names, primary)
	}
	for _, alias := range req.Aliases {
		if i := strings.Index(alias, " ("); i > 0 && strings.HasSuffix(alias, ")") && !trailingYearRe.MatchString(alias) {
			add(alias[:i])
			add(strings.TrimSuffix(alias[i+2:], ")"))
			continue
		}
		add(alias)
	}
	names = append(names, latin...)
	names = append(names, other...)
	if len(names) > maxSearchNames {
		names = names[:maxSearchNames]
	}
	return names
}

// nameKey is the form two names are compared in: case, punctuation and a
// trailing year do not make a different name. A name with no Latin letters
// or digits (a Japanese title, say) is compared as written, lower-cased.
func nameKey(name string) string {
	stripped := stripNamePunctuation(name)
	if key := normalizeMovieName(stripped); key != "" {
		return key
	}
	return strings.ToLower(strings.Join(strings.Fields(stripped), " "))
}

// hasLatinLetter reports whether the name has at least one Latin letter.
func hasLatinLetter(s string) bool {
	for _, r := range s {
		if r < 0x250 && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// createSearchStrategy builds the query ladder for one episode. Every query
// costs a rate-limited indexer call, so the ladder is kept short: the
// "S##E##" form (and the padded absolute number for anime) is tried under
// every name, the rarer forms only under the title, and a query that would
// repeat an earlier one is dropped.
func (q *QbittHandler) createSearchStrategy(req *tvdb.Media, episode *tvdb.Episode) []*models.SearchStrategy {
	names := searchNames(req)
	if len(names) == 0 {
		return nil
	}
	primary := names[0]

	paddedSeason := standardizeNumber(episode.SeasonNumber)
	paddedEpisode := standardizeNumber(episode.Number)

	var ss []*models.SearchStrategy
	seen := map[string]bool{}
	add := func(s *models.SearchStrategy) {
		key := strings.ToLower(s.Query)
		if seen[key] {
			return
		}
		seen[key] = true
		ss = append(ss, s)
	}

	// newShowStrategy builds a seasonal show strategy with the given query and relax level.
	newShowStrategy := func(query, name string, relax int) *models.SearchStrategy {
		return &models.SearchStrategy{
			Query:          query,
			MediaName:      name,
			Season:         episode.SeasonNumber,
			Episode:        episode.Number,
			EpisodeMeta:    episode,
			TvdbId:         req.Id,
			EpisodeTvdbID:  strconv.Itoa(episode.Id),
			ImdbID:         req.ImdbID,
			ReleaseYear:    req.Year,
			RuntimeMinutes: episode.Runtime,
			RelaxLevel:     relax,
		}
	}

	// newAbsoluteStrategy builds an anime absolute-numbered strategy with the
	// authoritative Exclude:["season","episode"] behavior preserved.
	newAbsoluteStrategy := func(query, name string, relax int) *models.SearchStrategy {
		return &models.SearchStrategy{
			Query:          query,
			MediaName:      name,
			Season:         episode.SeasonNumber,
			Episode:        episode.AbsoluteNumber,
			EpisodeMeta:    episode,
			Exclude:        []string{"season", "episode"},
			TvdbId:         req.Id,
			EpisodeTvdbID:  strconv.Itoa(episode.Id),
			ImdbID:         req.ImdbID,
			ReleaseYear:    req.Year,
			RuntimeMinutes: episode.Runtime,
			RelaxLevel:     relax,
		}
	}

	absolute := req.Anime && episode.AbsoluteNumber > 0

	// ----- Tier 0: strict -----
	if absolute {
		for _, name := range names {
			add(newAbsoluteStrategy(fmt.Sprint(name, " ", standardizeNumber(episode.AbsoluteNumber)), name, 0))
		}
	}
	for _, name := range names {
		add(newShowStrategy(fmt.Sprintf("%s S%sE%s", name, paddedSeason, paddedEpisode), name, 0))
	}

	// ----- Tier 1: relaxed -----
	// "{name} {S}x{EE}" e.g. "Show 1x05", and the long form, under the title only.
	add(newShowStrategy(fmt.Sprintf("%s %dx%s", primary, episode.SeasonNumber, paddedEpisode), primary, 1))
	add(newShowStrategy(fmt.Sprintf("%s Season %s Episode %s", primary, paddedSeason, paddedEpisode), primary, 1))
	// Anime absolute UNPADDED "{name} {absolute}" (a repeat of tier 0 from episode 10 on).
	if absolute {
		for _, name := range names {
			add(newAbsoluteStrategy(fmt.Sprintf("%s %d", name, episode.AbsoluteNumber), name, 1))
		}
	}
	// Punctuation-stripped name variants applied to the S##E## format.
	for _, name := range names {
		if stripped := stripNamePunctuation(name); stripped != "" && stripped != name {
			add(newShowStrategy(fmt.Sprintf("%s S%sE%s", stripped, paddedSeason, paddedEpisode), stripped, 1))
		}
	}

	// ----- Tier 2: broad -----
	// Loose episode token "{name} E{EE}", under the title only.
	add(newShowStrategy(fmt.Sprintf("%s E%s", primary, paddedEpisode), primary, 2))
	// For anime, name-only — rely on validators/parser to match S/E or absolute.
	if req.Anime {
		for _, name := range names {
			if episode.AbsoluteNumber > 0 {
				add(newAbsoluteStrategy(name, name, 2))
			} else {
				add(newShowStrategy(name, name, 2))
			}
		}
	}

	return ss
}

// stripNamePunctuation removes punctuation that commonly differs between TVDB
// titles and release titles: colons, apostrophes (straight and curly), commas
// and hyphens, plus any trailing bracketed or parenthesized year (e.g. "(2019)"
// or "[2019]"). Whitespace is collapsed to single spaces.
func stripNamePunctuation(name string) string {
	s := name

	// Strip a trailing bracketed/parenthesized year, e.g. "Show (2019)" / "Show [2019]".
	s = trailingYearRe.ReplaceAllString(s, "")

	// Remove punctuation characters.
	s = strings.Map(func(r rune) rune {
		switch r {
		case ':', '\'', '’', ',', '-':
			return -1
		}
		return r
	}, s)

	// Collapse whitespace.
	return strings.Join(strings.Fields(s), " ")
}

// createMovieSearchStrategy generates search strategies for movie downloads
// Returns array of SearchStrategy with single movie query
// Note: Indexer selection is handled by calcIndexerIDs() based on req.Anime flag
func (q *QbittHandler) createMovieSearchStrategy(req *tvdb.Media) []*models.SearchStrategy {
	var ss []*models.SearchStrategy

	newMovie := func(query string, relax int) *models.SearchStrategy {
		return &models.SearchStrategy{
			Query:          query,
			MediaName:      req.Name,
			Season:         0,   // Movies have no season
			Episode:        0,   // Movies have no episode (used to detect movie vs show)
			EpisodeMeta:    nil, // Movies have no episode metadata
			TvdbId:         req.Id,
			EpisodeTvdbID:  req.Id, // Movies key notifications/EpisodeResult by media id
			ReleaseYear:    req.Year,
			IsMovie:        true, // Flag this as a movie search strategy
			ImdbID:         req.ImdbID,
			RuntimeMinutes: req.Runtime,
			RelaxLevel:     relax,
		}
	}

	// Tier 0: "{name} {year}" (or just name when year unknown).
	tier0 := req.Name
	if req.Year != "" {
		tier0 = fmt.Sprintf("%s %s", req.Name, req.Year)
	}
	ss = append(ss, newMovie(tier0, 0))

	// Tier 1: name-only.
	ss = append(ss, newMovie(req.Name, 1))

	// Tier 2: punctuation-stripped name-only (only if it differs).
	if stripped := stripNamePunctuation(req.Name); stripped != "" && stripped != req.Name {
		ss = append(ss, newMovie(stripped, 2))
	}

	return ss
}
