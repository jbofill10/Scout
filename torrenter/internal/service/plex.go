package service

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"torrenter/internal/models"
)

var (
	mediaSectionBase = "/library/sections"
	libraryTypeMovie = "movie"
	libraryTypeShow  = "show"
)

type PlexHandler struct {
	cfg    *models.PlexCfg
	repo   Repository
	logger *slog.Logger
}

func NewPlexHandler(repo Repository, logger *slog.Logger, cfg *models.PlexCfg) *PlexHandler {
	return &PlexHandler{
		cfg:    cfg,
		repo:   repo,
		logger: logger,
	}
}

func (p *PlexHandler) getLibraries() models.PlexLibrariesResponse {
	url := p.cfg.Host + mediaSectionBase
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		p.logger.Error("Failed to create request for Plex libraries", "error", err)
		os.Exit(1)
	}
	req.Header.Set("X-Plex-Token", p.cfg.Key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		p.logger.Error("Failed to fetch Plex libraries", "error", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Error("Failed to read Plex libraries response", "error", err)
		os.Exit(1)
	}

	var libraryRes models.PlexLibrariesResponse
	err = xml.Unmarshal(bodyBytes, &libraryRes)
	if err != nil {
		p.logger.Error("Failed to unmarshal Plex libraries response", "error", err)
		os.Exit(1)
	}

	p.logger.Debug("Content", "data", libraryRes)

	return libraryRes
}

func (p *PlexHandler) SyncPlexLibrary() {

	libraries := p.getLibraries()
	p.repo.UpsertLibraries(libraries)
	movies := p.getMovies()
	p.repo.UpsertMovies(movies)
	shows := p.getShows()
	if shows != nil {
		p.repo.UpsertShows(shows)
	}

	p.logger.Info("Plex Library Sync Complete...")
}

// getTvdbIdForShow fetches detailed metadata for a show and extracts the TVDB ID from GUIDs
func (p *PlexHandler) getTvdbIdForShow(ratingKey string) string {
	url := fmt.Sprintf("%s/library/metadata/%s?includeGuids=1", p.cfg.Host, ratingKey)

	type MetadataContainer struct {
		XMLName  string             `xml:"MediaContainer"`
		Metadata []models.PlexShow  `xml:"Directory"`
	}

	var container MetadataContainer
	err := p.fetchAndUnmarshal(url, &container)
	if err != nil {
		p.logger.Error("Error fetching metadata for show", "value", ratingKey, "error", err)
		return ""
	}

	if len(container.Metadata) == 0 {
		return ""
	}

	// Extract TVDB ID from Guids
	for _, guid := range container.Metadata[0].Guids {
		if len(guid.ID) > 7 && guid.ID[:7] == "tvdb://" {
			return guid.ID[7:] // Strip "tvdb://" prefix
		}
	}

	return ""
}

func (p *PlexHandler) getMovies() models.PlexMovieLibraryData {
	p.logger.Info("Starting plex movie media sync...")
	movies := models.PlexMovieLibraryData{}

	section, err := p.repo.GetLibraryByType(libraryTypeMovie)

	if err != nil {
		p.logger.Error("Error getting movie sections", "error", err)
		return movies
	}

	url := p.cfg.Host + mediaSectionBase + "/" + fmt.Sprint(section) + "/all"
	err = p.fetchAndUnmarshal(url, &movies)
	if err != nil {
		p.logger.Error("Error fetching movies", "error", err)
		return movies
	}

	return movies
}

func (p *PlexHandler) fetchAndUnmarshal(url string, v interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Plex-Token", p.cfg.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("non-200 response: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return xml.Unmarshal(body, v)
}

func (p *PlexHandler) getShows() *models.PlexShowLibraryData {
	showSection, err := p.repo.GetLibraryByType(libraryTypeShow)
	if err != nil {
		p.logger.Error("Error getting show section", "error", err)
		return nil
	}

	url := fmt.Sprintf("%s/library/sections/%d/all", p.cfg.Host, showSection)
	var showsResp models.PlexShowsResponse
	err = p.fetchAndUnmarshal(url, &showsResp)
	if err != nil {
		p.logger.Error("Error fetching shows", "error", err)
		return nil
	}

	libraryData := &models.PlexShowLibraryData{
		Shows: make([]models.PlexShowData, 0, len(showsResp.Shows)),
	}

	for _, show := range showsResp.Shows {
		// Fetch detailed metadata with external IDs
		tvdbId := p.getTvdbIdForShow(show.ShowKey)

		showData := models.PlexShowData{
			Id:       show.ShowKey,
			Title:    show.Title,
			ShowMeta: show.Key,
			Thumb:    show.Thumb,
			TvdbId:   tvdbId,
			Seasons:  make([]models.PlexSeasonData, 0),
		}

		// Get seasons for this show
		url2 := p.cfg.Host + show.Key
		var seasonsResp models.PlexSeasonsResponse
		err = p.fetchAndUnmarshal(url2, &seasonsResp)
		if err != nil {
			p.logger.Error("Error fetching seasons for show", "field", show.Title, "error", err)
			continue
		}

		for _, season := range seasonsResp.Seasons {
			seasonData := models.PlexSeasonData{
				Id:           season.SeasonKey,
				SeasonMeta:   season.Key,
				SeasonNumber: season.Index,
				Episodes:     make([]models.PlexEpisodeData, 0),
			}

			// Get episodes for this season
			url3 := p.cfg.Host + season.Key
			var episodesResp models.PlexEpisodesResponse
			err = p.fetchAndUnmarshal(url3, &episodesResp)
			if err != nil {
				p.logger.Error("Error fetching episodes for season", "field", season.Title, "error", err)
				continue
			}

			for _, episode := range episodesResp.Videos {
				episodeData := models.PlexEpisodeData{
					Id:            episode.EpisodeKey,
					EpisodeMeta:   episode.Key,
					EpisodeNumber: episode.Index,
					Media:         []models.PlexMediaData{},
				}
				for _, media := range episode.Media {
					mediaData := models.PlexMediaData{
						VideoResolution: media.VideoResolution,
					}
					if len(media.Part) > 0 {
						mediaData.File = media.Part[0].File
						mediaData.Id = media.Part[0].Id
					}
					episodeData.Media = append(episodeData.Media, mediaData)
				}
				seasonData.Episodes = append(seasonData.Episodes, episodeData)
			}
			showData.Seasons = append(showData.Seasons, seasonData)
		}
		libraryData.Shows = append(libraryData.Shows, showData)
	}

	return libraryData
}
