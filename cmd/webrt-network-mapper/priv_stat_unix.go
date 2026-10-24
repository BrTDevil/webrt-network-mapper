//go:build !windows

package main

import (
	"os"
	"syscall"
)

func statOwner(st os.FileInfo) (uint32, bool) {
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return s.Uid, true
}
