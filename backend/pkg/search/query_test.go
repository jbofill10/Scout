package search

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)

func TestParse_PlainTitle(t *testing.T) {
	q := Parse("  The   Office ", now)

	assert.Equal(t, "The Office", q.Raw)
	assert.Equal(t, "The Office", q.Title)
	assert.Empty(t, q.Year)
	assert.Empty(t, q.Filters)
	assert.False(t, q.HasFilters())
}

func TestParse_BracketedYear(t *testing.T) {
	for _, raw := range []string{"X (2022)", "X [2022]", "(2022) X", "X(2022)"} {
		q := Parse(raw, now)
		assert.Equal(t, "X", q.Title, raw)
		assert.Equal(t, "2022", q.Year, raw)
		assert.Empty(t, q.AltTitle, "a bracketed year is unambiguous: no verbatim retry")
		assert.Equal(t, []Filter{{Key: KeyYear, Value: "2022", Inferred: true}}, q.Filters, raw)
	}
}

func TestParse_TrailingYearKeepsVerbatimTitle(t *testing.T) {
	q := Parse("Class of 1984", now)

	assert.Equal(t, "Class of", q.Title)
	assert.Equal(t, "1984", q.Year)
	assert.Equal(t, "Class of 1984", q.AltTitle)
	assert.Equal(t, []Filter{{Key: KeyYear, Value: "1984", Inferred: true}}, q.Filters)
}

func TestParse_NumbersThatAreNotYears(t *testing.T) {
	tests := map[string]string{
		"1917":                 "1917",                 // a lone number is a title
		"2012":                 "2012",                 // same
		"Blade Runner 2049":    "Blade Runner 2049",    // too far ahead
		"Fahrenheit 451":       "Fahrenheit 451",       // three digits
		"Death Race 20000":     "Death Race 20000",     // five digits
		"2001 a space odyssey": "2001 a space odyssey", // year not trailing
	}
	for raw, wantTitle := range tests {
		q := Parse(raw, now)
		assert.Equal(t, wantTitle, q.Title, raw)
		assert.Empty(t, q.Year, raw)
		assert.Empty(t, q.Filters, raw)
	}
}

func TestParse_YearEdgeOfRange(t *testing.T) {
	assert.Equal(t, "2031", Parse("Dune 2031", now).Year, "five years ahead still counts")
	assert.Empty(t, Parse("Dune 2032", now).Year)
	assert.Equal(t, "1880", Parse("Roundhay 1880", now).Year)
	assert.Empty(t, Parse("Roundhay 1879", now).Year)
}

func TestParse_ExplicitYearBeatsInferred(t *testing.T) {
	q := Parse("X 2019 year:2022", now)

	assert.Equal(t, "X 2019", q.Title, "an explicit year leaves the title's number alone")
	assert.Equal(t, "2022", q.Year)
	assert.Empty(t, q.AltTitle)
	assert.Equal(t, []Filter{{Key: KeyYear, Value: "2022"}}, q.Filters)
}

func TestParse_InvalidExplicitYearStaysText(t *testing.T) {
	q := Parse("X year:abc", now)

	assert.Equal(t, "X year:abc", q.Title)
	assert.Empty(t, q.Year)
	assert.Empty(t, q.Filters)
}

func TestParse_DirectorValueRunsToEnd(t *testing.T) {
	q := Parse("X director:ti west", now)

	assert.Equal(t, "X", q.Title)
	assert.Equal(t, "ti west", q.Director)
	assert.Equal(t, []Filter{{Key: KeyDirector, Value: "ti west"}}, q.Filters)
}

func TestParse_ValueStopsAtNextFilter(t *testing.T) {
	q := Parse("X director:ti west year:2022 studio:A24", now)

	assert.Equal(t, "X", q.Title)
	assert.Equal(t, "ti west", q.Director)
	assert.Equal(t, "2022", q.Year)
	assert.Equal(t, "A24", q.Company)
	assert.Equal(t, []Filter{
		{Key: KeyDirector, Value: "ti west"},
		{Key: KeyYear, Value: "2022"},
		{Key: KeyStudio, Value: "A24"},
	}, q.Filters)
}

func TestParse_QuotedValueReturnsToTitle(t *testing.T) {
	for _, raw := range []string{`director:"Ti West" Pearl`, `director: "Ti West" Pearl`} {
		q := Parse(raw, now)
		assert.Equal(t, "Pearl", q.Title, raw)
		assert.Equal(t, "Ti West", q.Director, raw)
	}
}

func TestParse_SpaceAfterColon(t *testing.T) {
	q := Parse("actor: mia goth", now)

	assert.Empty(t, q.Title)
	assert.Equal(t, "mia goth", q.Person)
	assert.Equal(t, RoleActor, q.PersonRole)
}

func TestParse_ActorAliases(t *testing.T) {
	for _, key := range []string{"actor", "cast", "starring", "Actor", "CAST"} {
		q := Parse(key+":Mia Goth", now)
		assert.Equal(t, "Mia Goth", q.Person, key)
		assert.Equal(t, RoleActor, q.PersonRole, key)
		assert.Equal(t, []Filter{{Key: KeyActor, Value: "Mia Goth"}}, q.Filters, key)
	}
}

func TestParse_PersonIsAnyRole(t *testing.T) {
	q := Parse("person:Ti West", now)

	assert.Equal(t, "Ti West", q.Person)
	assert.Empty(t, q.PersonRole)
	assert.Equal(t, []Filter{{Key: KeyPerson, Value: "Ti West"}}, q.Filters)
}

func TestParse_LaterPersonFilterReplacesEarlier(t *testing.T) {
	q := Parse("actor:Mia Goth person:Ti West", now)

	assert.Equal(t, "Ti West", q.Person)
	assert.Empty(t, q.PersonRole)
	assert.Equal(t, []Filter{{Key: KeyPerson, Value: "Ti West"}}, q.Filters, "one chip per person slot")
}

func TestParse_CompanyAliases(t *testing.T) {
	assert.Equal(t, "A24", Parse("X company:A24", now).Company)
	assert.Equal(t, "HBO", Parse("Succession network:HBO", now).Company)
	assert.Equal(t, []Filter{{Key: KeyNetwork, Value: "HBO"}}, Parse("Succession network:HBO", now).Filters)
}

func TestParse_CountryAndLanguageAreLowercased(t *testing.T) {
	q := Parse("Dark country:DEU lang:DEU", now)

	assert.Equal(t, "Dark", q.Title)
	assert.Equal(t, "deu", q.Country)
	assert.Equal(t, "deu", q.Language)
	assert.Equal(t, []Filter{{Key: KeyCountry, Value: "DEU"}, {Key: KeyLanguage, Value: "DEU"}}, q.Filters)
}

func TestParse_ColonsInTitlesAreNotFilters(t *testing.T) {
	for _, raw := range []string{"Mission: Impossible", "Re:Zero", "Lone Star: Origins", "Star Wars: Andor"} {
		q := Parse(raw, now)
		assert.Equal(t, raw, q.Title, raw)
		assert.Empty(t, q.Filters, raw)
	}
}

func TestParse_ImdbIdAndUrl(t *testing.T) {
	for _, raw := range []string{
		"tt13560574",
		"https://www.imdb.com/title/tt13560574/",
		"imdb.com/title/tt13560574?ref_=nv_sr_srsg_0",
		"https://m.imdb.com/title/tt13560574",
		"imdb:tt13560574",
		"imdb:https://www.imdb.com/title/tt13560574/",
	} {
		q := Parse(raw, now)
		assert.Equal(t, "tt13560574", q.ImdbID, raw)
		assert.Empty(t, q.Title, raw)
		assert.Equal(t, []Filter{{Key: KeyImdb, Value: "tt13560574"}}, q.Filters, raw)
		assert.False(t, q.IsEmpty(), raw)
	}
}

func TestParse_BadImdbFilterIsDropped(t *testing.T) {
	q := Parse("X imdb:nope", now)

	assert.Equal(t, "X", q.Title)
	assert.Empty(t, q.ImdbID)
	assert.Empty(t, q.Filters)
}

func TestParse_EmptyAndFilterOnly(t *testing.T) {
	assert.True(t, Parse("", now).IsEmpty())
	assert.True(t, Parse("   ", now).IsEmpty())

	yearOnly := Parse("year:2022", now)
	assert.True(t, yearOnly.IsEmpty(), "a year alone has nothing to search")
	assert.True(t, yearOnly.HasFilters())

	assert.False(t, Parse("actor:Mia Goth", now).IsEmpty(), "a person is searchable on its own")
	assert.False(t, Parse("director:Ti West", now).IsEmpty())
}

func TestProxyParams_RoundTrip(t *testing.T) {
	q := Parse(`Class of 1984 director:"Mark Lester" actor:Roddy McDowall studio:Guerrilla country:can lang:eng`, now)

	values := q.ProxyParams("movie")
	assert.Equal(t, "Class of", values.Get(ParamTitle))
	assert.Equal(t, "movie", values.Get(ParamMediaType))
	assert.Equal(t, "Class of 1984", values.Get(ParamAltTitle))
	assert.Equal(t, "1984", values.Get(ParamYear))

	back := FromProxyParams(values)
	q.Raw, q.Filters = "", []Filter{} // not carried across the wire
	assert.Equal(t, q, back)
}

func TestProxyParams_OmitsEmptyFields(t *testing.T) {
	values := Parse("X", now).ProxyParams("series")

	assert.Equal(t, url.Values{ParamTitle: {"X"}, ParamMediaType: {"series"}}, values)
}

func TestFromProxyParams_Trims(t *testing.T) {
	q := FromProxyParams(url.Values{ParamTitle: {"  X "}, ParamYear: {" 2022"}})

	require.Equal(t, "X", q.Title)
	assert.Equal(t, "2022", q.Year)
	assert.NotNil(t, q.Filters)
}

func TestNormalize(t *testing.T) {
	assert.Equal(t, "mia goth", Normalize("  Mia   GOTH "))
	assert.Equal(t, "x", Normalize("X!"))
	assert.Equal(t, "the office us", Normalize("The Office (US)"))
	assert.Equal(t, "", Normalize("..."))
	assert.Equal(t, Normalize("Mia Goth"), Normalize("mia-goth"))
}
