package service

import (
	"fmt"
	"strconv"
	"strings"

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

func (q *QbittHandler) createSearchStrategy(req *tvdb.Media, episode *tvdb.Episode) []*models.SearchStrategy {
	var ss []*models.SearchStrategy

	// Build media names from req.Name + all aliases, with deduplication
	mediaNames := []string{req.Name}
	mediaNames = append(mediaNames, req.Aliases...)

	// Deduplicate
	seen := make(map[string]bool)
	uniqueNames := []string{}
	for _, name := range mediaNames {
		if !seen[name] && name != "" {
			seen[name] = true
			uniqueNames = append(uniqueNames, name)
		}
	}
	showNames := uniqueNames

	paddedSeason := standardizeNumber(episode.SeasonNumber)
	paddedEpisode := standardizeNumber(episode.Number)

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
			RuntimeMinutes: episode.Runtime,
			RelaxLevel:     relax,
		}
	}

	// ----- Tier 0: strict (existing behavior) -----
	if req.Anime {
		for _, showName := range showNames {
			// Only create absolute number strategy if TVDB provided one (absoluteNumber > 0).
			if episode.AbsoluteNumber > 0 {
				ss = append(ss, newAbsoluteStrategy(
					fmt.Sprint(showName, " ", standardizeNumber(episode.AbsoluteNumber)), showName, 0))
			}
		}
	}

	for _, format := range []string{"%s S%sE%s", "%s Season %s Episode %s"} {
		for _, showName := range showNames {
			ss = append(ss, newShowStrategy(
				fmt.Sprintf(format, showName, paddedSeason, paddedEpisode), showName, 0))
		}
	}

	// ----- Tier 1: relaxed -----
	for _, showName := range showNames {
		// "{name} {S}x{EE}" e.g. "Show 1x05"
		ss = append(ss, newShowStrategy(
			fmt.Sprintf("%s %dx%s", showName, episode.SeasonNumber, paddedEpisode), showName, 1))

		// Anime absolute UNPADDED "{name} {absolute}"
		if req.Anime && episode.AbsoluteNumber > 0 {
			ss = append(ss, newAbsoluteStrategy(
				fmt.Sprintf("%s %d", showName, episode.AbsoluteNumber), showName, 1))
		}

		// Punctuation-stripped name variant applied to the S##E## format.
		if stripped := stripNamePunctuation(showName); stripped != "" && stripped != showName {
			ss = append(ss, newShowStrategy(
				fmt.Sprintf("%s S%sE%s", stripped, paddedSeason, paddedEpisode), stripped, 1))
		}
	}

	// ----- Tier 2: broad -----
	for _, showName := range showNames {
		// Loose episode token "{name} E{EE}"
		ss = append(ss, newShowStrategy(
			fmt.Sprintf("%s E%s", showName, paddedEpisode), showName, 2))

		// For anime, name-only — rely on validators/parser to match S/E or absolute.
		if req.Anime {
			if episode.AbsoluteNumber > 0 {
				ss = append(ss, newAbsoluteStrategy(showName, showName, 2))
			} else {
				ss = append(ss, newShowStrategy(showName, showName, 2))
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
