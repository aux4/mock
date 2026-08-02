//go:build windows

package main

import (
	"os"
	"syscall"
)

func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func killGroup(pid int) {
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
}
