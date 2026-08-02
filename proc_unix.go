//go:build !windows

package main

import (
	"os"
	"syscall"
)

// detachAttr puts the child in its own process group so it survives the
// parent exiting and can be killed as a group.
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// killGroup terminates the process group led by pid, then the process itself.
func killGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)
}

var _ = os.Getpid
