// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux darwin freebsd netbsd openbsd solaris

package main

import (
	"os/exec"
	"syscall"
	"time"
)

func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func quit(cmd *exec.Cmd) {
	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}
	syscall.Kill(-pid, syscall.SIGQUIT)
}

func kill(cmd *exec.Cmd) {
	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}
	syscall.Kill(-pid, syscall.SIGINT)
	time.Sleep(100 * time.Millisecond)
	syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(100 * time.Millisecond)
	syscall.Kill(-pid, syscall.SIGKILL)
}
