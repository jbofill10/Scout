package service

import (
	"context"
	"syscall"
)

// freeSpaceFn reports the free bytes on the volume holding path. Tests swap it.
var freeSpaceFn = statfsFree

func statfsFree(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}

// DownloadsFreeSpace is the free space of the downloads volume, for the
// settings view.
func DownloadsFreeSpace() (uint64, error) {
	return freeSpaceFn(baseSavePath)
}

// spaceVerdict is the outcome of a disk space check.
type spaceVerdict struct {
	fits    bool
	free    uint64
	reserve uint64
}

// checkDiskSpace decides whether a release of size bytes can be started
// without eating into the configured reserve. Without runtime settings there
// is no reserve and no check; a failed probe lets the download proceed, since
// refusing every download over a stat error would be worse than a full disk.
func (q *QbittHandler) checkDiskSpace(ctx context.Context, size int64) spaceVerdict {
	if q.settings == nil {
		return spaceVerdict{fits: true}
	}
	reserve := q.settings.MinFreeSpaceBytes()
	free, err := freeSpaceFn(baseSavePath)
	if err != nil {
		q.logger.WarnContext(ctx, "Could not read free disk space, allowing the download", "path", baseSavePath, "error", err)
		return spaceVerdict{fits: true, reserve: reserve}
	}
	need := reserve
	if size > 0 {
		need += uint64(size)
	}
	return spaceVerdict{fits: free >= need, free: free, reserve: reserve}
}
