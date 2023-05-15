// +build unix

package main

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func extstart() {
	user := os.Getenv("USER")
	if user == "" {
		return
	}
	disp := os.Getenv("DISPLAY")
	if disp != "" {
		exname = fmt.Sprintf("/tmp/.sam.%s.%s", user, disp)
	} else {
		exname = fmt.Sprintf("/tmp/.sam.%s", user)
	}
	err := syscall.Mkfifo(exname, 0600)
	if err != nil {
		if !os.IsExist(err) {
			return
		}
		st, err := os.Stat(exname)
		if err != nil {
			return
		}
		if st.Mode()&fs.ModeNamedPipe == 0 {
			removeextern()
			if err := syscall.Mkfifo(exname, 0600); err != nil {
				return
			}
		}
	}

	fd, err := syscall.Open(exname, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		removeextern()
		return
	}

	// Turn off no-delay and provide ourselves as a lingering
	// writer so as not to get end of file on read.
	flags, err := unix.FcntlInt(uintptr(fd), syscall.F_GETFL, 0)
	if err != nil {
		goto Fail
	}
	if _, err := unix.FcntlInt(uintptr(fd), syscall.F_SETFL, flags & ^syscall.O_NONBLOCK); err != nil {
		goto Fail
	}
	if _, err := syscall.Open(exname, syscall.O_WRONLY, 0); err != nil {
		goto Fail
	}

	plumbc = make(chan string)
	go extproc(plumbc, os.NewFile(uintptr(fd), exname))
	// atexit(removeextern)
	return

Fail:
	syscall.Close(fd)
	removeextern()
}
