package main

import (
	"fmt"
	iopkg "io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"os/user"
)

var inerror = false

/*
 * A reasonable interface to the system calls
 */

func resetsys() {
	inerror = false
}

func syserror(a string, err error) {
	if !inerror {
		inerror = true
		dprint("%s: ", a)
		error_s(Eio, err.Error())
	}
}

func Read(f *os.File, a []byte) int {
	n, err := iopkg.ReadFull(f, a)
	if err != nil {
		if lastfile != nil {
			lastfile.rescuing = 1
		}
		if downloaded {
			fmt.Fprintf(os.Stderr, "read error: %s\n", err)
		}
		rescue()
		os.Exit(1)
	}
	return n
}

func Write(f *os.File, b []byte) int {
	m, err := f.Write(b)
	if err != nil || m != len(b) {
		if err == nil {
			err = iopkg.ErrShortWrite
		}
		syserror("write", err)
	}
	return m
}

func Seek(f *os.File, n int64, w int) {
	if _, err := f.Seek(n, w); err != nil {
		syserror("seek", err)
	}
}

func Close(f *os.File) {
	if err := f.Close(); err != nil {
		syserror("close", err)
	}
}

var samname = []rune("~~sam~~")

var (
	left  = [][]rune{[]rune("{[(<«"), []rune("\n"), []rune("'\"`")}
	right = [][]rune{[]rune("}])>»"), []rune("\n"), []rune("'\"`")}
)

var (
	RSAM    = "sam"
	SAMTERM = "samterm"
	SH      = "sh"
	SHPATH  = "/bin/sh"
	RX      = "ssh"
	RXPATH  = "ssh"
)

func dprint(z string, args ...interface{}) {
	termwrite(fmt.Sprintf(z, args...))
}

func print_ss(s string, a *String, b *String) {
	dprint("?warning: %s: `%s' and `%s'\n", s, a, b)
}

func print_s(s string, a *String) {
	dprint("?warning: %s `%s'\n", s, a)
}

var getuser_user string

func getuser() string {
	if getuser_user != "" {
		u, err := user.Current()
		if err != nil {
			getuser_user = "nobody"
		} else {
			getuser_user = u.Username
		}
	}
	return getuser_user
}

func hup(c chan os.Signal) {
	<-c
	panicking = 1 /* ??? */
	rescue()
	os.Exit(1)
}

var SIGHUP os.Signal

func siginit() {
	signal.Notify(make(chan os.Signal), os.Interrupt)
	if SIGHUP != nil {
		c := make(chan os.Signal, 1)
		signal.Notify(c, SIGHUP)
		go hup(c)
	}
}

func mktemp() *os.File {
	f, err := ioutil.TempFile("", fmt.Sprintf("sam.%d.*", os.Getpid()))
	if err != nil {
		rescue()
		log.Fatalf("creating temp file: %v", err)
	}
	return f
}

func samerr() string {
	return fmt.Sprintf("%s/sam.%s.err", os.TempDir(), os.Getenv("USER"))
}

func tempdisk() *os.File {
	f := mktemp()
	os.Remove(f.Name())
	return f
}
