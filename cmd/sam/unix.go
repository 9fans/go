// +build unix

package main

import "syscall"

func init() {
	SIGHUP = syscall.SIGHUP
}
