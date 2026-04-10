//go:build !linux && !darwin

package api

func ramStats() (total, used, avail uint64) { return }
