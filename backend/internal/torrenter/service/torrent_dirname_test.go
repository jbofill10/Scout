package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTorrentDirName(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Dexter.S08E05.1080p.WEB-DL", "Dexter.S08E05.1080p.WEB-DL"},
		{"[豌豆字幕组&LoliHouse] 进击的巨人 / Shingeki no Kyojin - 74 [WebRip 1080p]",
			"[豌豆字幕组&LoliHouse] 进击的巨人 - Shingeki no Kyojin - 74 [WebRip 1080p]"},
		{"Show\\Name S01E01", "Show - Name S01E01"},
		{"  ..Odd Title.. ", "Odd Title"},
		{"Bad\x00Control\x1fChars", "BadControlChars"},
		{"///", "fallback-uuid"},
		{"", "fallback-uuid"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, torrentDirName(c.title, "fallback-uuid"), c.title)
	}

	long := torrentDirName(strings.Repeat("进", 120), "x")
	assert.LessOrEqual(t, len(long), maxTorrentDirBytes)
	assert.Equal(t, 0, len(long)%3, "cut on a rune boundary")
}
