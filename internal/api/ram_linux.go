//go:build linux

package api

import "golang.org/x/sys/unix"

func ramStats() (total, used, avail uint64) {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return
	}
	total = info.Totalram * uint64(info.Unit)
	avail = (info.Freeram + info.Bufferram) * uint64(info.Unit)
	used = total - avail
	return
}
