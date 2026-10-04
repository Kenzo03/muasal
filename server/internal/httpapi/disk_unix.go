//go:build !windows

package httpapi

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// sameDisk reports whether two folders are on one filesystem (MSL-40). Docker
// keeps named volumes on the host's disk, so a default install says yes until
// the backups volume is mounted elsewhere, such as on a NAS.
func sameDisk(a, b string) bool {
	x, errA := os.Stat(a)
	y, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	sx, okX := x.Sys().(*syscall.Stat_t)
	sy, okY := y.Sys().(*syscall.Stat_t)
	return okX && okY && sx.Dev == sy.Dev
}

func diskUse(volume, path string) DiskUse {
	d := DiskUse{Volume: DiskUseVolume(volume), Path: path}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			d.Missing = ptr(true)
		}
		return d
	}
	bs := int64(st.Bsize)
	d.TotalBytes = int64(st.Blocks) * bs
	d.UsedBytes = (int64(st.Blocks) - int64(st.Bfree)) * bs
	return d
}
