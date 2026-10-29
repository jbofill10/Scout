// Package search turns the free text typed into Scout's search box into a
// structured query: the title to look up plus any filters (year, director,
// actor, studio or network, country, language, IMDb id) that came along with
// it. The webserver parses, hands the structure to tvdb-proxy as query
// parameters, and echoes it back to the UI so the interpretation can be shown.
package search

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Canonical filter keys. Aliases typed by the user are folded onto these.
const (
	KeyYear     = "year"
	KeyDirector = "director"
	KeyActor    = "actor"
	KeyPerson   = "person"
	KeyStudio   = "studio"
	KeyNetwork  = "network"
	KeyCountry  = "country"
	KeyLanguage = "language"
	KeyImdb     = "imdb"
)

// filterKeys maps every accepted `key:` prefix onto its canonical key. Short
// or common words ("with", "star", "y") are deliberately absent: a title such
// as "Lone Star: Origins" must not turn into a filter.
var filterKeys = map[string]string{
	"year":     KeyYear,
	"director": KeyDirector,
	"dir":      KeyDirector,
	"actor":    KeyActor,
	"cast":     KeyActor,
	"starring": KeyActor,
	"person":   KeyPerson,
	"studio":   KeyStudio,
	"company":  KeyStudio,
	"network":  KeyNetwork,
	"country":  KeyCountry,
	"lang":     KeyLanguage,
	"language": KeyLanguage,
	"imdb":     KeyImdb,
}

// Role names for Query.PersonRole.
const (
	RoleActor    = "actor"
	RoleDirector = "director"
)

// Filter is one constraint read from the query, kept in typing order so the UI
// can show it as a chip.
type Filter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Inferred is true when the filter was read out of the title text (a
	// trailing or parenthesised year) rather than typed as key:value.
	Inferred bool `json:"inferred,omitempty"`
}

// Query is the structured reading of a search box entry.
type Query struct {
	// Raw is the text as typed, trimmed.
	Raw string `json:"raw"`
	// Title is the free text left once filters and an inline year are removed.
	// It may be empty when the query is only filters.
	Title string `json:"title"`
	// AltTitle is set when Year was inferred from a bare trailing number: the
	// title with that number still in it, to be searched verbatim as well so
	// "Class of 1984" still finds the film called exactly that.
	AltTitle string `json:"-"`
	Year     string `json:"year,omitempty"`
	Director string `json:"director,omitempty"`
	// Person names someone whose credits should be searched. PersonRole limits
	// which credits count: RoleActor for actor:/cast:, empty for person: (any).
	Person     string `json:"person,omitempty"`
	PersonRole string `json:"personRole,omitempty"`
	// Company is a studio (movies) or network (series) name.
	Company  string `json:"company,omitempty"`
	Country  string `json:"country,omitempty"`
	Language string `json:"language,omitempty"`
	ImdbID   string `json:"imdbId,omitempty"`
	// Filters lists every constraint in display order.
	Filters []Filter `json:"filters"`
}

var (
	imdbIDPattern  = regexp.MustCompile(`^tt\d{5,10}$`)
	imdbURLPattern = regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.|m\.)?imdb\.com/title/(tt\d{5,10})(?:[/?#].*)?$`)
	// A year in brackets anywhere in the title: "X (2022)", "Dune [2021]".
	bracketYear = regexp.MustCompile(`\s*[\(\[](\d{4})[\)\]]`)
	fourDigits  = regexp.MustCompile(`^\d{4}$`)
	keyPrefix   = regexp.MustCompile(`^([A-Za-z]+):(.*)$`)
	spaces      = regexp.MustCompile(`\s+`)
)

// Parse reads raw into a Query. now bounds which four-digit numbers count as
// release years (1880 up to five years ahead), so "Blade Runner 2049" and
// "1917" stay titles.
func Parse(raw string, now time.Time) Query {
	raw = strings.TrimSpace(spaces.ReplaceAllString(raw, " "))
	q := Query{Raw: raw, Filters: []Filter{}}
	if raw == "" {
		return q
	}

	if id := imdbIDFrom(raw); id != "" {
		q.ImdbID = id
		q.Filters = append(q.Filters, Filter{Key: KeyImdb, Value: id})
		return q
	}

	titleWords, filters := splitFilters(tokenize(raw))
	for _, f := range filters {
		if f.Key == KeyYear && !validYear(f.Value, now) {
			// Not a year after all: give the text back to the title.
			titleWords = append(titleWords, "year:"+f.Value)
			continue
		}
		q.apply(f)
	}

	title := strings.Join(titleWords, " ")
	if q.Year == "" {
		title = q.inferYear(title, now)
	}
	q.Title = strings.TrimSpace(title)
	return q
}

// apply records one typed filter, replacing an earlier value for the same key.
func (q *Query) apply(f Filter) {
	f.Value = strings.TrimSpace(f.Value)
	if f.Value == "" {
		return
	}
	switch f.Key {
	case KeyYear:
		q.Year = f.Value
	case KeyDirector:
		q.Director = f.Value
	case KeyActor:
		q.Person, q.PersonRole = f.Value, RoleActor
	case KeyPerson:
		q.Person, q.PersonRole = f.Value, ""
	case KeyStudio, KeyNetwork:
		q.Company = f.Value
	case KeyCountry:
		q.Country = strings.ToLower(f.Value)
	case KeyLanguage:
		q.Language = strings.ToLower(f.Value)
	case KeyImdb:
		if id := imdbIDFrom(f.Value); id != "" {
			q.ImdbID = id
			f.Value = id
		} else {
			return
		}
	}
	q.setFilter(f)
}

// setFilter adds f to the display list, or replaces the entry with the same
// key (or the same person/company slot) so one chip shows per constraint.
func (q *Query) setFilter(f Filter) {
	sameSlot := func(key string) bool {
		if key == f.Key {
			return true
		}
		people := map[string]bool{KeyActor: true, KeyPerson: true}
		companies := map[string]bool{KeyStudio: true, KeyNetwork: true}
		return (people[key] && people[f.Key]) || (companies[key] && companies[f.Key])
	}
	for idx := range q.Filters {
		if sameSlot(q.Filters[idx].Key) {
			q.Filters[idx] = f
			return
		}
	}
	q.Filters = append(q.Filters, f)
}

// inferYear pulls a release year out of the title text. A bracketed year is
// unambiguous and is simply removed. A bare trailing year ("X 2022") is taken
// as a year too, but the untouched title is kept as AltTitle so a film whose
// name really ends in a number is still found.
func (q *Query) inferYear(title string, now time.Time) string {
	if m := bracketYear.FindStringSubmatchIndex(title); m != nil {
		year := title[m[2]:m[3]]
		if validYear(year, now) {
			q.Year = year
			q.Filters = append(q.Filters, Filter{Key: KeyYear, Value: year, Inferred: true})
			return strings.TrimSpace(title[:m[0]] + " " + title[m[1]:])
		}
	}

	words := strings.Fields(title)
	if len(words) < 2 {
		return title
	}
	last := words[len(words)-1]
	if !validYear(last, now) {
		return title
	}
	q.Year = last
	q.AltTitle = title
	q.Filters = append(q.Filters, Filter{Key: KeyYear, Value: last, Inferred: true})
	return strings.Join(words[:len(words)-1], " ")
}

// splitFilters separates key:value filters from title words. Filter values run
// until the next filter key or the end of the text, so "X director:ti west"
// reads as title "X" with director "ti west". A quoted value ends at its
// closing quote, after which words belong to the title again.
func splitFilters(tokens []string) (title []string, filters []Filter) {
	var open *Filter // unquoted filter still collecting words
	for _, tok := range tokens {
		if m := keyPrefix.FindStringSubmatch(tok); m != nil {
			if key, ok := filterKeys[strings.ToLower(m[1])]; ok {
				filters = append(filters, Filter{Key: key})
				f := &filters[len(filters)-1]
				open = nil
				switch rest := m[2]; {
				case rest == "":
					open = f // value starts at the next token
				case isQuoted(rest):
					f.Value = unquote(rest)
				default:
					f.Value = rest
					open = f
				}
				continue
			}
		}
		if open != nil {
			if open.Value == "" && isQuoted(tok) {
				open.Value = unquote(tok)
				open = nil
				continue
			}
			open.Value = strings.TrimSpace(open.Value + " " + unquote(tok))
			continue
		}
		title = append(title, unquote(tok))
	}
	return title, filters
}

// tokenize splits on whitespace while keeping a double-quoted run together
// with whatever it is attached to, so `director:"Ti West"` is one token.
func tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case unicode.IsSpace(r) && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func isQuoted(s string) bool {
	return strings.HasPrefix(s, `"`)
}

func unquote(s string) string {
	return strings.TrimSpace(strings.Trim(s, `"`))
}

func validYear(s string, now time.Time) bool {
	if !fourDigits.MatchString(s) {
		return false
	}
	year, _ := strconv.Atoi(s)
	return year >= 1880 && year <= now.Year()+5
}

// imdbIDFrom returns the title id in an IMDb id or title URL, or "".
func imdbIDFrom(s string) string {
	s = strings.TrimSpace(s)
	if imdbIDPattern.MatchString(s) {
		return s
	}
	if m := imdbURLPattern.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// IsEmpty reports whether nothing searchable was typed.
func (q Query) IsEmpty() bool {
	return q.Title == "" && q.Person == "" && q.Director == "" && q.ImdbID == ""
}

// HasFilters reports whether any constraint accompanies the title.
func (q Query) HasFilters() bool {
	return len(q.Filters) > 0
}

// Proxy query parameter names. mediaName and mediaType predate this package
// and are kept for compatibility with the tvdb-proxy API.
const (
	ParamTitle      = "mediaName"
	ParamMediaType  = "mediaType"
	ParamAltTitle   = "altName"
	ParamYear       = "year"
	ParamDirector   = "director"
	ParamPerson     = "person"
	ParamPersonRole = "personRole"
	ParamCompany    = "company"
	ParamCountry    = "country"
	ParamLanguage   = "language"
	ParamImdb       = "imdb"
)

// ProxyParams encodes the query as the parameters tvdb-proxy's search endpoint
// accepts. Empty fields are omitted.
func (q Query) ProxyParams(mediaType string) url.Values {
	values := url.Values{}
	set := func(key, value string) {
		if value != "" {
			values.Set(key, value)
		}
	}
	set(ParamTitle, q.Title)
	set(ParamMediaType, mediaType)
	set(ParamAltTitle, q.AltTitle)
	set(ParamYear, q.Year)
	set(ParamDirector, q.Director)
	set(ParamPerson, q.Person)
	set(ParamPersonRole, q.PersonRole)
	set(ParamCompany, q.Company)
	set(ParamCountry, q.Country)
	set(ParamLanguage, q.Language)
	set(ParamImdb, q.ImdbID)
	return values
}

// FromProxyParams is the inverse of ProxyParams, used by tvdb-proxy to read
// a structured query off the request. Filters is left empty: the proxy does
// not display the query.
func FromProxyParams(values url.Values) Query {
	return Query{
		Title:      strings.TrimSpace(values.Get(ParamTitle)),
		AltTitle:   strings.TrimSpace(values.Get(ParamAltTitle)),
		Year:       strings.TrimSpace(values.Get(ParamYear)),
		Director:   strings.TrimSpace(values.Get(ParamDirector)),
		Person:     strings.TrimSpace(values.Get(ParamPerson)),
		PersonRole: strings.TrimSpace(values.Get(ParamPersonRole)),
		Company:    strings.TrimSpace(values.Get(ParamCompany)),
		Country:    strings.TrimSpace(values.Get(ParamCountry)),
		Language:   strings.TrimSpace(values.Get(ParamLanguage)),
		ImdbID:     strings.TrimSpace(values.Get(ParamImdb)),
		Filters:    []Filter{},
	}
}

// Normalize folds a name for comparison: lower case, letters and digits only,
// single spaces. "Mia  Goth" and "mia goth" compare equal; so do "X" and "X!".
func Normalize(s string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			b.WriteRune(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}
