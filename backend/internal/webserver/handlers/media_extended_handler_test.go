package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
)

// setupMediaExtendedHandler wires the handler to a stub tvdb-proxy and returns
// the proxy request it saw, so tests can assert on what was forwarded.
func setupMediaExtendedHandler(
	t *testing.T,
	proxy http.HandlerFunc,
) (*MediaExtendedHandler, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	server := httptest.NewServer(proxy)
	t.Cleanup(server.Close)

	buf := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(buf, nil))
	host := strings.TrimPrefix(server.URL, "http://")

	return NewMediaExtendedHandler(clients.NewTVDBProxyClient(host), logger), server
}

func TestGetDetails_ReturnsMappedMedia(t *testing.T) {
	handler, _ := setupMediaExtendedHandler(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/series/456/extended", r.URL.Path)
		assert.Equal(t, "movie", r.URL.Query().Get("mediaType"))

		_ = json.NewEncoder(w).Encode(tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{
				Id:         456,
				Name:       "Dune",
				Overview:   "A noble family is thrust into a war.",
				Year:       "2021",
				Runtime:    155,
				FirstAired: "2021-09-15",
				Genres:     []tvdb.Genres{{Name: "Science Fiction"}},
				ContentRatings: []tvdb.ContentRating{
					{Country: "usa", Name: "PG-13"},
				},
			},
		})
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "456"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/media/456/details?mediaType=movie", nil)

	handler.GetDetails(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var details tvdb.Media
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &details))
	assert.Equal(t, "456", details.Id)
	assert.Equal(t, "A noble family is thrust into a war.", details.Overview)
	assert.Equal(t, 155, details.Runtime)
	assert.Equal(t, "PG-13", details.ContentRating)
	assert.Equal(t, []string{"Science Fiction"}, details.Genres)
	assert.Equal(t, "2021-09-15", details.Metadata.FirstAired)
}

func TestGetDetails_DefaultsToSeries(t *testing.T) {
	var seenMediaType string
	handler, _ := setupMediaExtendedHandler(t, func(w http.ResponseWriter, r *http.Request) {
		seenMediaType = r.URL.Query().Get("mediaType")
		_ = json.NewEncoder(w).Encode(tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{Id: 123, Name: "Severance"},
		})
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "123"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/media/123/details", nil)

	handler.GetDetails(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "series", seenMediaType)
}

func TestGetDetails_ProxyFailureReturnsBadGateway(t *testing.T) {
	handler, _ := setupMediaExtendedHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "999"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/media/999/details?mediaType=series", nil)

	handler.GetDetails(c)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}
