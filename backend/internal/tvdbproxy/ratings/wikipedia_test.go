package ratings

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTomatometerFromWikitext(t *testing.T) {
	cases := map[string]struct {
		text string
		want float64
		ok   bool
	}{
		"template positional": {`Reception. {{Rotten Tomatoes prose|93|7.8|250}} Critics liked it.`, 93, true},
		"template named":      {`{{Rotten Tomatoes prose|score=41|average=5.1|count=120}}`, 41, true},
		"template alias":      {`{{RT prose|86|7.2|90}}`, 86, true},
		"template empty falls back to prose": {
			`{{Rotten Tomatoes prose}} On [[Rotten Tomatoes]], the film holds an approval rating of 77% based on 200 reviews.`,
			77, true},
		"prose critics first": {
			`On the [[review aggregator]] website [[Rotten Tomatoes]], 94% of 200 critics' reviews are positive, ` +
				`with an average rating of 8.1/10. Audiences polled by [[CinemaScore]] gave it an "A".`, 94, true},
		"audience sentence skipped": {
			`On [[Rotten Tomatoes]], the film has an audience score of 95%. On Rotten Tomatoes, 31% of 80 critics' reviews are positive.`,
			31, true},
		"no score": {`The film was released on [[Rotten Tomatoes]] day.`, 0, false},
	}
	for name, c := range cases {
		got, ok := tomatometerFromWikitext(c.text)
		assert.Equal(t, c.ok, ok, name)
		assert.Equal(t, c.want, got, name)
	}
}

func TestMetascoreFromWikitext(t *testing.T) {
	got, ok := metascoreFromWikitext(`{{Metacritic film prose|81|50}}`)
	assert.True(t, ok)
	assert.Equal(t, 81.0, got)

	got, ok = metascoreFromWikitext(`[[Metacritic]], which uses a weighted average, assigned the film a score of 70 out of 100 based on 40 critics.`)
	assert.True(t, ok)
	assert.Equal(t, 70.0, got)

	_, ok = metascoreFromWikitext(`Metacritic listed it among the most anticipated films.`)
	assert.False(t, ok)
}

const sampleMediaWiki = `{"batchcomplete":true,"query":{
 "normalized":[{"fromencoded":false,"from":"wicker (film)","to":"Wicker (film)"}],
 "redirects":[{"from":"Sinners (film)","to":"Sinners (2025 film)"}],
 "pages":[
  {"pageid":1,"ns":0,"title":"Wicker (film)","revisions":[{"slots":{"main":{"contentmodel":"wikitext","content":"On [[Rotten Tomatoes]], 94% of 120 critics' reviews are positive. [[Metacritic]] assigned a score of 70 out of 100."}}}]},
  {"pageid":2,"ns":0,"title":"Sinners (2025 film)","revisions":[{"slots":{"main":{"content":"{{Rotten Tomatoes prose|97|8.4|400}} {{Metacritic film prose|84|60}}"}}}]},
  {"ns":0,"title":"Nothing (film)","missing":true}
 ]}}`

func TestProseScores(t *testing.T) {
	var seen *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		_, _ = w.Write([]byte(sampleMediaWiki))
	}))
	defer server.Close()
	source := NewProse(server.URL, server.Client(), slog.Default())

	scores, err := source.Scores(context.Background(), []string{"wicker (film)", "Sinners (film)", "Nothing (film)"})

	require.NoError(t, err)
	require.NotNil(t, seen)
	assert.Equal(t, "wicker (film)|Sinners (film)|Nothing (film)", seen.URL.Query().Get("titles"))
	assert.Equal(t, "revisions", seen.URL.Query().Get("prop"))
	assert.Equal(t, []tvdb.Rating{
		{Source: tvdb.RatingSourceRottenTomatoes, Value: 94}, {Source: tvdb.RatingSourceMetacritic, Value: 70},
	}, scores["wicker (film)"], "answered under the normalised title")
	assert.Equal(t, []tvdb.Rating{
		{Source: tvdb.RatingSourceRottenTomatoes, Value: 97}, {Source: tvdb.RatingSourceMetacritic, Value: 84},
	}, scores["Sinners (film)"], "answered under the redirect target")
	_, ok := scores["Nothing (film)"]
	assert.False(t, ok)
}

func TestProseScoresErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	source := NewProse(server.URL, server.Client(), slog.Default())

	_, err := source.Scores(context.Background(), []string{"A"})

	assert.Error(t, err)
}

func TestProseScoresAsksNothingForNoTitles(t *testing.T) {
	source := NewProse("http://127.0.0.1:1", http.DefaultClient, slog.Default())

	scores, err := source.Scores(context.Background(), nil)

	require.NoError(t, err)
	assert.Empty(t, scores)
}

func TestTemplateScoreAndPercent(t *testing.T) {
	for _, args := range []string{"", "score=abc", "score=101|count=3", "average=7.1|count=3", "-5|7.1|3"} {
		_, ok := templateScore(args)
		assert.False(t, ok, args)
	}
	value, ok := templateScore(" 88 | 7.9 | 120 ")
	assert.True(t, ok)
	assert.Equal(t, 88.0, value)
	value, ok = templateScore("count=120|score= 64 ")
	assert.True(t, ok)
	assert.Equal(t, 64.0, value)
	_, ok = percent("100.5")
	assert.False(t, ok)
	value, ok = percent("100")
	assert.True(t, ok)
	assert.Equal(t, 100.0, value)
}
