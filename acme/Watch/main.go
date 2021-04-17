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
//
// The command and arguments are joined by spaces and passed to rc(1)
// to be interpreted as a shell command line.
//
// The command is printed at the top of the window, preceded by a "% " prompt.
// Changing that line changes the command run each time the window is updated.
// Adding other lines beginning with "% " will cause those commands to be run
// as well.
//
// Executing Quit sends a SIGQUIT on systems that support that signal.
// (Go programs receiving that signal will dump goroutine stacks and exit.)
//
// Executing Kill stops any commands being executed. On Unix it sends the commands
// a SIGINT, followed 100ms later by a SIGTERM, followed 100ms later by a SIGKILL.
// On other systems it sends os.Interrupt followed 100ms later by os.Kill
package main // import "9fans.net/go/acme/Watch"

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"9fans.net/go/acme"
)

var args []string
var win *acme.Win
var needrun = make(chan bool, 1)
var recursive = flag.Bool("r", false, "watch all subdirectories recursively")

func usage() {
	fmt.Fprintf(os.Stderr, "usage: Watch [-r] cmd args...\n")
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("Watch: ")
	flag.Usage = usage
	flag.Parse()
	args = flag.Args()

	var err error
	win, err = acme.New()
	if err != nil {
		log.Fatal(err)
	}
	pwd, _ := os.Getwd()
	pwdSlash := strings.TrimSuffix(pwd, "/") + "/"
	win.Name(pwdSlash + "+watch")
	win.Ctl("clean")
	win.Ctl("dumpdir " + pwd)
	cmd := "dump Watch"
	if *recursive {
		cmd += " -r"
	}
	win.Ctl(cmd)
	win.Fprintf("tag", "Get Kill Quit ")
	win.Fprintf("body", "%% %s\n", strings.Join(args, " "))

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
		if ev.Op == "put" && (path.Dir(ev.Name) == pwd || *recursive && strings.HasPrefix(ev.Name, pwdSlash)) {
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
			if string(e.Text) == "Kill" {
				run.Lock()
				cmd := run.cmd
				run.kill = true
				run.Unlock()
				if cmd != nil {
					kill(cmd)
				}
				continue
			}
			if string(e.Text) == "Quit" {
				run.Lock()
				cmd := run.cmd
				run.Unlock()
				if cmd != nil {
					quit(cmd)
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
	id   int
	cmd  *exec.Cmd
	kill bool
}

func runner() {
	for range needrun {
		run.Lock()
		run.id++
		id := run.id
		lastcmd := run.cmd
		run.cmd = nil
		run.kill = false
		run.Unlock()
		if lastcmd != nil {
			kill(lastcmd)
		}
		lastcmd = nil

		runSetup(id)
		go runBackground(id)
	}
}

var cmdRE = regexp.MustCompile(`(?m)^%[ \t]+[^ \t\n].*\n`)

func runSetup(id int) {
	// Remove old output, but leave commands.
	// Running synchronously in runner, so no need to watch run.id.
	data, _ := win.ReadAll("body")
	matches := cmdRE.FindAllIndex(data, -1)
	if len(matches) == 0 {
		// reset window
		win.Addr(",")
		win.Write("data", nil)
		win.Write("body", []byte(fmt.Sprintf("%% %s\n", strings.Join(args, " "))))
	} else {
		end, endByte := utf8.RuneCount(data), len(data)
		for i := len(matches) - 1; i >= 0; i-- {
			m := matches[i]
			mEnd := end - utf8.RuneCount(data[m[1]:endByte])
			mStart := mEnd - utf8.RuneCount(data[m[0]:m[1]])
			if mStart != utf8.RuneCount(data[:m[0]]) || mEnd != utf8.RuneCount(data[:m[1]]) {
				log.Fatal("bad runes")
			}
			if mEnd < end {
				win.Addr("#%d,#%d", mEnd, end)
				win.Write("data", nil)
			}
			end, endByte = mStart, m[0]
		}
		if end > 0 {
			win.Addr(",#%d", end)
			win.Write("data", nil)
		}
	}
	win.Addr("#0")
}

func runBackground(id int) {
	buf := make([]byte, 4096)
	run.Lock()
	for {
		if id != run.id || run.kill {
			run.Unlock()
			return
		}
		q0, _, err := win.ReadAddr()
		if err != nil {
			log.Fatal(err)
		}
		err = win.Addr("#%d/^%%[ \t][^ \t\\n].*\\n/", q0)
		if err != nil {
			run.Unlock()
			log.Print("no command")
			return
		}
		m0, _, err := win.ReadAddr()
		if m0 < q0 {
			// wrapped around
			win.Addr("#%d", q0)
			win.Write("data", []byte("% \n"))
			win.Ctl("clean")
			win.Addr("#%d", m0)
			win.Ctl("dot=addr")
			win.Ctl("show")
			run.Unlock()
			return
		}
		data, err := win.ReadAll("xdata")
		if err != nil {
			log.Fatal(err)
		}
		run.Unlock()

		line := data[1:] // chop %

		// Find the plan9port rc.
		// There may be a different rc in the PATH,
		// but there probably won't be a different 9.
		// Don't just invoke 9, because it will change
		// the PATH.
		var rc string
		if dir := os.Getenv("PLAN9"); dir != "" {
			rc = filepath.Join(dir, "bin/rc")
		} else if nine, err := exec.LookPath("9"); err == nil {
			rc = filepath.Join(filepath.Dir(nine), "rc")
		} else {
			rc = "/usr/local/plan9/bin/rc"
		}

		cmd := exec.Command(rc, "-c", string(line))
		r, w, err := os.Pipe()
		if err != nil {
			log.Fatal(err)
		}
		cmd.Stdout = w
		cmd.Stderr = w
		isolate(cmd)
		err = cmd.Start()
		w.Close()
		run.Lock()
		if run.id != id || run.kill {
			r.Close()
			run.Unlock()
			kill(cmd)
			return
		}
		if err != nil {
			r.Close()
			win.Fprintf("data", "(exec: %s)\n", err)
			continue
		}
		run.cmd = cmd
		run.Unlock()

		bol := true
		for {
			n, err := r.Read(buf)
			if err != nil {
				break
			}
			run.Lock()
			if id == run.id && n > 0 {
				// Insert leading space in front of % at start of line
				// to avoid introducing new commands.
				p := buf[:n]
				if bol && p[0] == '%' {
					win.Write("data", []byte(" "))
				}
				for {
					// invariant: len(p) > 0.
					// invariant: not at beginning of line in acme window output
					// (either not at beginning of line in actual output, or leading space printed).
					i := bytes.Index(p, []byte("\n%"))
					if i < 0 {
						break
					}
					win.Write("data", p[:i+1])
					win.Write("data", []byte(" "))
					p = p[i+1:]
				}
				win.Write("data", p)
				bol = p[len(p)-1] == '\n'
			}
			run.Unlock()
		}
		err = cmd.Wait()
		run.Lock()
		if id == run.id {
			// If output was missing final newline, print trailing backslash and add newline.
			if !bol {
				win.Fprintf("data", "\\\n")
			}
			if err != nil {
				win.Fprintf("data", "(%v)\n", err)
			}
		}
		// Continue loop with lock held
	}
}
