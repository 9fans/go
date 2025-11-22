// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !solaris
// +build !linux,!darwin,!freebsd,!netbsd,!openbsd,!solaris

package main

import (
	"os"
	"os/exec"
	"time"
)

func isolate(cmd *exec.Cmd) {
}

func quit(cmd *exec.Cmd) {
}

func kill(cmd *exec.Cmd) {
	cmd.Process.Signal(os.Interrupt)
	time.Sleep(100 * time.Millisecond)
	cmd.Process.Kill()
}
