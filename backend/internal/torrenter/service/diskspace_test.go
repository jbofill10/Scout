package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakePickerSettings struct {
	quality string
	reserve uint64
}

func (f *fakePickerSettings) MinQuality() string        { return f.quality }
func (f *fakePickerSettings) MinFreeSpaceBytes() uint64 { return f.reserve }

func TestCheckDiskSpace(t *testing.T) {
	q := &QbittHandler{logger: slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))}
	const gb = uint64(1) << 30
	orig := freeSpaceFn
	defer func() { freeSpaceFn = orig }()

	t.Run("no settings means no check", func(t *testing.T) {
		freeSpaceFn = func(string) (uint64, error) { return 0, nil }
		assert.True(t, q.checkDiskSpace(context.Background(), int64(50*gb)).fits)
	})

	q.settings = &fakePickerSettings{reserve: 10 * gb}

	t.Run("fits when size plus reserve is available", func(t *testing.T) {
		freeSpaceFn = func(string) (uint64, error) { return 25 * gb, nil }
		assert.True(t, q.checkDiskSpace(context.Background(), int64(15*gb)).fits)
		v := q.checkDiskSpace(context.Background(), int64(16*gb))
		assert.False(t, v.fits, "would eat into the reserve")
		assert.Equal(t, 25*gb, v.free)
		assert.Equal(t, 10*gb, v.reserve)
	})

	t.Run("unknown size needs only the reserve", func(t *testing.T) {
		freeSpaceFn = func(string) (uint64, error) { return 10 * gb, nil }
		assert.True(t, q.checkDiskSpace(context.Background(), 0).fits)
		freeSpaceFn = func(string) (uint64, error) { return 9 * gb, nil }
		assert.False(t, q.checkDiskSpace(context.Background(), 0).fits)
	})

	t.Run("probe failure allows the download", func(t *testing.T) {
		freeSpaceFn = func(string) (uint64, error) { return 0, errors.New("no such volume") }
		assert.True(t, q.checkDiskSpace(context.Background(), int64(100*gb)).fits)
	})
}
