package media

import (
	"encoding/json"
	"testing"
)

func TestExtendedDataImdbID(t *testing.T) {
	cases := []struct {
		name string
		ids  []RemoteID
		want string
	}{
		{"picks the IMDb entry by type", []RemoteID{{Id: "348350", Type: 10, SourceName: "TheMovieDB.com"}, {Id: "tt3778644", Type: 2, SourceName: "IMDB"}}, "tt3778644"},
		{"falls back to the source name", []RemoteID{{Id: "tt0121955", Type: 99, SourceName: "imdb"}}, "tt0121955"},
		{"ignores an IMDb entry that is not a title id", []RemoteID{{Id: "nm0000001", Type: 2, SourceName: "IMDB"}}, ""},
		{"none", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := TVDBSeriesExtendedData{RemoteIds: tc.ids}
			if got := d.ImdbID(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// TVDB sends remoteIds[].id as a string even for numeric services; decoding
// the extended record must not choke on it.
func TestExtendedResponseDecodesRemoteIds(t *testing.T) {
	raw := `{"data":{"id":60,"name":"Solo","year":"2018","runtime":135,"remoteIds":[
		{"id":"10.5240/1FB1-0A32-9B27-9553-8E45-V","type":13,"sourceName":"EIDR"},
		{"id":"tt3778644","type":2,"sourceName":"IMDB"},
		{"id":"348350","type":10,"sourceName":"TheMovieDB.com"}]}}`
	var resp TVDBSeriesExtendedResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if got := resp.Data.ImdbID(); got != "tt3778644" {
		t.Fatalf("got %q", got)
	}
	if resp.Data.Runtime != 135 {
		t.Fatalf("runtime %d", resp.Data.Runtime)
	}
}

func TestMediaImdbIDRoundTrips(t *testing.T) {
	m := Media{Id: "1", Name: "x", ImdbID: "tt1"}
	b, _ := json.Marshal(m)
	var back Media
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.ImdbID != "tt1" {
		t.Fatalf("got %q", back.ImdbID)
	}
	// Omitted when empty, so stored rows and API payloads do not grow a blank field.
	b, _ = json.Marshal(Media{Id: "1"})
	if string(b) != "" && json.Valid(b) && containsKey(b, "imdbId") {
		t.Fatalf("empty ImdbID should be omitted: %s", b)
	}
}

func containsKey(b []byte, key string) bool {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	_, ok := m[key]
	return ok
}
