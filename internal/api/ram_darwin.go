//go:build darwin

package api

import "golang.org/x/sys/unix"

func ramStats() (total, used, avail uint64) {
	total, _ = unix.SysctlUint64("hw.memsize")
	return
}
