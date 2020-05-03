// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Watch runs a command each time any file in the current directory is written.
//
// Usage:
//
//	Watch cmd [args...]
//
// Watch opens a new acme window named for the current directory
// with a suffix of /+watch. The window shows the execution of the given
// command. Each time any file in that directory is Put from within acme,
// Watch reexecutes the command and updates the window.
package main // import "9fans.net/go/acme/Watch"

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"9fans.net/go/acme"
)

var args []string
var win *acme.Win
var needrun = make(chan bool, 1)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: Watch cmd args...\n")
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("Watch: ")
	flag.Usage = usage
	flag.Parse()
	args = flag.Args()
	if len(args) == 0 {
		usage()
	}

	var err error
	win, err = acme.New()
	if err != nil {
		log.Fatal(err)
	}
	pwd, _ := os.Getwd()
	win.Name(pwd + "/+watch")
	win.Ctl("clean")
	win.Fprintf("tag", "Get ")
	needrun <- true
	go events()
	go runner()

	r, err := acme.Log()
	if err != nil {
		log.Fatal(err)
	}
	for {
		ev, err := r.Read()
		if err != nil {
			log.Fatal(err)
		}
		if ev.Op == "put" && path.Dir(ev.Name) == pwd {
			select {
			case needrun <- true:
			default:
			}
			// slow down any runaway loops
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func events() {
	for e := range win.EventChan() {
		switch e.C2 {
		case 'x', 'X': // execute
			if string(e.Text) == "Get" {
				select {
				case needrun <- true:
				default:
				}
				continue
			}
			if string(e.Text) == "Del" {
				win.Ctl("delete")
			}
		}
		win.WriteEvent(e)
	}
	os.Exit(0)
}

var run struct {
	sync.Mutex
	id int
}

func runner() {
	var lastcmd *exec.Cmd
	for _ = range needrun {
		run.Lock()
		run.id++
		id := run.id
		run.Unlock()
		if lastcmd != nil {
			lastcmd.Process.Kill()
		}
		lastcmd = nil
		cmd := exec.Command(args[0], args[1:]...)
		r, w, err := os.Pipe()
		if err != nil {
			log.Fatal(err)
		}
		win.Addr(",")
		win.Write("data", nil)
		win.Ctl("clean")
		win.Fprintf("body", "$ %s\n", strings.Join(args, " "))
		cmd.Stdout = w
		cmd.Stderr = w
		if err := cmd.Start(); err != nil {
			r.Close()
			w.Close()
			win.Fprintf("body", "%s: %s\n", strings.Join(args, " "), err)
			continue
		}
		lastcmd = cmd
		w.Close()
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := r.Read(buf)
				if err != nil {
					break
				}
				run.Lock()
				if id == run.id {
					win.Write("body", buf[:n])
				}
				run.Unlock()
			}
			if err := cmd.Wait(); err != nil {
				run.Lock()
				if id == run.id {
					win.Fprintf("body", "%s: %s\n", strings.Join(args, " "), err)
				}
				run.Unlock()
			}
			win.Fprintf("body", "$\n")
			win.Fprintf("addr", "#0")
			win.Ctl("dot=addr")
			win.Ctl("show")
			win.Ctl("clean")
		}()
	}
}
