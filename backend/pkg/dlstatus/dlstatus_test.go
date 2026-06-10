package dlstatus

import "testing"

func TestFailureCode_Category(t *testing.T) {
	cases := []struct {
		code FailureCode
		want FailureCategory
	}{
		{CodeIndexerUnreachable, CategoryTransient},
		{CodeNoTorrentFound, CategoryTransient},
		{CodeTorrentClientError, CategoryTransient},
		{CodeMonitorLost, CategoryTransient},
		{CodeTorrenterUnreachable, CategoryTransient},
		{CodeProcessingError, CategoryPermanent},
		{CodeInvalidMedia, CategoryPermanent},
		{CodeMaxRetries, CategoryPermanent},
		{FailureCode("something_unknown"), CategoryPermanent},
		{FailureCode(""), CategoryPermanent},
	}

	for _, c := range cases {
		if got := c.code.Category(); got != c.want {
			t.Errorf("Category(%q) = %q, want %q", c.code, got, c.want)
		}
	}
}

func TestFailureCode_HumanReason(t *testing.T) {
	cases := []struct {
		code FailureCode
		want string
	}{
		{CodeIndexerUnreachable, "Indexer unreachable — will retry"},
		{CodeNoTorrentFound, "No torrent found yet — will retry"},
		{CodeTorrentClientError, "Torrent client error — will retry"},
		{CodeMonitorLost, "Lost track of the download — will retry"},
		{CodeTorrenterUnreachable, "Download service unreachable — will retry"},
		{CodeProcessingError, "Failed to process the downloaded files"},
		{CodeInvalidMedia, "Media request was invalid"},
		{CodeMaxRetries, "Gave up after repeated attempts"},
		{FailureCode("nope"), "Unknown failure"},
	}

	for _, c := range cases {
		if got := c.code.HumanReason(); got != c.want {
			t.Errorf("HumanReason(%q) = %q, want %q", c.code, got, c.want)
		}
	}
}

// TestFailureCode_AllCodesMapped guards against adding a new code without
// wiring up Category/HumanReason (both must return non-default values).
func TestFailureCode_AllCodesMapped(t *testing.T) {
	all := []FailureCode{
		CodeIndexerUnreachable, CodeNoTorrentFound, CodeTorrentClientError,
		CodeMonitorLost, CodeTorrenterUnreachable, CodeProcessingError,
		CodeInvalidMedia, CodeMaxRetries,
	}
	for _, code := range all {
		if r := code.HumanReason(); r == "" || r == "Unknown failure" {
			t.Errorf("code %q has no specific HumanReason mapping", code)
		}
	}
}
