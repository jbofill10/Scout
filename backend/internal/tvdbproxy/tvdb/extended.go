package tvdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jbofill10/scout/backend/pkg/media"
)

// translations returns the translated record for a media item along with
// the aliases derived from it, or empty values on error. Movies keep their
// overview and tagline here rather than on the extended record, so callers
// need the record itself too.
func (c *Client) translations(
	ctx context.Context, mediaId string, language string, mediaType string,
) (media.TVDBTranslationData, []media.Alias) {
	// Conditional translations endpoint based on media type
	var translationURL string
	if mediaType == "movie" {
		translationURL = c.cfg.Host + fmt.Sprintf("/movies/%s/translations/%s", mediaId, language)
	} else {
		translationURL = c.cfg.Host + fmt.Sprintf("/series/%s/translations/%s", mediaId, language)
	}

	_, bodyBytes, err := c.authedGet(ctx, translationURL)
	if err != nil {
		c.logger.ErrorContext(ctx, "Failed to fetch translations",
			"media_id", mediaId, "language", language, "error", err)
		return media.TVDBTranslationData{}, []media.Alias{}
	}

	var translationInfo media.TVDBTranslationResponse
	if err := json.Unmarshal(bodyBytes, &translationInfo); err != nil {
		c.logger.ErrorContext(ctx, "Failed to decode translations response",
			"media_id", mediaId, "language", language, "error", err)
		return media.TVDBTranslationData{}, []media.Alias{}
	}

	c.logger.InfoContext(ctx, "Fetched translation",
		"media_id", mediaId, "language", language, "name", translationInfo.Data.Name, "body", translationInfo)
	var aliases []media.Alias
	// Include the translated name itself as an alias (e.g. "Jujutsu Kaisen" from the English translation)
	if translationInfo.Data.Name != "" {
		aliases = append(aliases, media.Alias{
			Language: language,
			Name:     translationInfo.Data.Name,
		})
	}
	for _, alias := range translationInfo.Data.Aliases {
		aliases = append(aliases, media.Alias{
			Language: language,
			Name:     alias,
		})
	}
	return translationInfo.Data, aliases
}

// earliestRelease picks a movie's release date: the global first_release when
// TVDB provides one, otherwise the earliest dated entry in releases.
func earliestRelease(first media.Release, releases []media.Release) string {
	if first.Date != "" {
		return first.Date
	}

	earliest := ""
	for _, release := range releases {
		if release.Date == "" {
			continue
		}
		if earliest == "" || release.Date < earliest {
			earliest = release.Date
		}
	}
	return earliest
}

// Extended fetches the extended record for one series or movie (mediaType
// "movie" selects TVDB's movie endpoints, anything else the series ones) and
// enriches it: English aliases, plus Japanese ones for anime; overview,
// tagline and firstAired filled in for movies; and the episode list for series.
func (c *Client) Extended(
	ctx context.Context, mediaId string, mediaType string,
) (*media.TVDBSeriesExtendedResponse, error) {
	// Conditional endpoint logic based on media type
	var url string
	if mediaType == "movie" {
		url = c.cfg.Host + fmt.Sprintf("/movies/%s/extended", mediaId)
	} else {
		// Default to series for backwards compatibility
		url = c.cfg.Host + fmt.Sprintf("/series/%s/extended", mediaId)
	}

	_, bodyBytes, err := c.authedGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to TVDB: %w", err)
	}

	var info media.TVDBSeriesExtendedResponse
	if err := json.Unmarshal(bodyBytes, &info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Determine if the media is anime by checking genres
	// For movies: check both "Anime" and "Animation" genres
	// For series: check only "Anime" genre (existing behavior)
	isAnime := false
	for _, genre := range info.Data.Genres {
		if mediaType == "movie" {
			if genre.Name == "Anime" || genre.Name == "Animation" {
				isAnime = true
				break
			}
		} else {
			if genre.Name == "Anime" {
				isAnime = true
				break
			}
		}
	}

	// Always fetch English translations
	c.logger.InfoContext(ctx, "Fetching English translations", "media_id", mediaId)
	engTranslation, engAliases := c.translations(ctx, mediaId, "eng", mediaType)
	info.Data.Aliases = append(info.Data.Aliases, engAliases...)

	// Movies carry no inline overview or tagline, so take them from the English
	// translation. Series usually have an overview already; only fill the gaps.
	if info.Data.Overview == "" {
		info.Data.Overview = engTranslation.Overview
	}
	if info.Data.Tagline == "" {
		info.Data.Tagline = engTranslation.Tagline
	}

	// Movies have no firstAired field: their release date lives in first_release,
	// with per-country dates in releases. Downstream scheduling and the detail
	// view both read firstAired, so normalise it here.
	if info.Data.FirstAired == "" {
		info.Data.FirstAired = earliestRelease(info.Data.FirstRelease, info.Data.Releases)
	}

	// If anime, also fetch Japanese translations
	if isAnime {
		c.logger.InfoContext(ctx, "Media is anime, fetching Japanese translations", "media_id", mediaId)
		_, jpnAliases := c.translations(ctx, mediaId, "jpn", mediaType)
		info.Data.Aliases = append(info.Data.Aliases, jpnAliases...)
	}

	// Filter aliases by language: always include 'eng', include 'jpn' if anime
	filteredAliases := []media.Alias{}
	for _, alias := range info.Data.Aliases {
		if alias.Language == "eng" || (isAnime && alias.Language == "jpn") {
			filteredAliases = append(filteredAliases, alias)
		}
	}
	info.Data.Aliases = filteredAliases

	// Fetch episode metadata for TV series to enable episode-level status display
	if mediaType == "series" {
		c.logger.InfoContext(ctx, "Fetching episode metadata for series", "media_id", mediaId)
		seriesMetadata, err := c.SeriesEpisodes(ctx, mediaId)
		if err == nil && len(seriesMetadata.Data.Episodes) > 0 {
			info.Data.Episodes = seriesMetadata.Data.Episodes
			c.logger.InfoContext(ctx, "Successfully fetched episode metadata",
				"media_id", mediaId, "episode_count", len(seriesMetadata.Data.Episodes))
		} else if err != nil {
			c.logger.ErrorContext(ctx, "Failed to fetch episode metadata", "media_id", mediaId, "error", err)
		} else {
			c.logger.WarnContext(ctx, "No episodes found for series", "media_id", mediaId)
		}
	}

	c.logger.InfoContext(ctx, "Fetched extended information", "media_id", mediaId, "is_anime", isAnime)
	return &info, nil
}
