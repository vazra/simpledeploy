package api

import "syscall"

func statfsBlockSize(s *syscall.Statfs_t) uint64 {
	return uint64(s.Bsize)
}
