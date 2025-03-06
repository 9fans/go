//go:build plan9

// Rot13fs is a demo of a write-read exchange over a synthetic file.
// It serves /dev/rot13. A program opens /dev/rot13, writes data to it,
// then writes a zero-length write to signal that the writes are done,
// and then can read the rot13 of the data back.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"syscall"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/srv9p"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: rot13fs [-m mtpt] [-s srvname]\n")
	flag.PrintDefaults()
	os.Exit(1)
}

var (
	mtpt    = flag.String("m", "/dev", "mount at `mtpt`")
	srvname = flag.String("s", "", "post service at /srv/`name`")
	verbose = flag.Bool("v", false, "print protocol trace on standard error")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("rot13fs: ")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 0 {
		usage()
	}

	args := []string{}
	if *verbose {
		args = append(args, "-v")
	}
	srv9p.PostMountServe(*srvname, *mtpt, syscall.MBEFORE, args, rot13Server)
}

func rot13Server() *srv9p.Server {
	tree := srv9p.NewTree("tyraqn", "tbcure", plan9.DMDIR|0555, nil)
	rot13File, err := tree.Root.Create("rot13", "tbcure", 0666, nil)
	if err != nil {
		log.Fatal(err)
	}

	type rot13State struct {
		mu    sync.Mutex
		req   []byte
		reply []byte
	}

	var mu sync.Mutex
	rot13s := map[*srv9p.Fid]*rot13State{}

	state := func(fid *srv9p.Fid) *rot13State {
		mu.Lock()
		defer mu.Unlock()

		s := rot13s[fid]
		if s == nil {
			s = new(rot13State)
			rot13s[fid] = s
		}
		return s
	}

	srv := &srv9p.Server{
		Tree: tree,
		Clunk: func(fid *srv9p.Fid) {
			mu.Lock()
			delete(rot13s, fid)
			mu.Unlock()
		},
		Write: func(ctx context.Context, fid *srv9p.Fid, data []byte, offset int64) (int, error) {
			if fid.File() != rot13File {
				return 0, fmt.Errorf("unknown file")
			}

			s := state(fid)
			s.mu.Lock()
			defer s.mu.Unlock()

			s.req = append(s.req, data...)
			if len(data) == 0 { // 0-length write is signal to run request
				s.reply = rot13Req(s.req)
			}
			return len(data), nil
		},
		Read: func(ctx context.Context, fid *srv9p.Fid, data []byte, offset int64) (int, error) {
			if fid.File() != rot13File {
				return 0, fmt.Errorf("unknown file")
			}

			s := state(fid)
			s.mu.Lock()
			defer s.mu.Unlock()

			if s.reply == nil {
				return 0, fmt.Errorf("no request written")
			}
			return fid.ReadBytes(data, offset, s.reply)
		},
	}
	if *verbose {
		srv.Trace = os.Stderr
	}
	return srv
}

func rot13Req(data []byte) []byte {
	out := make([]byte, len(data))
	for i, c := range data {
		if 'A' <= c && c <= 'M' || 'a' <= c && c <= 'm' {
			c += 13
		} else if 'N' <= c && c <= 'Z' || 'n' <= c && c <= 'z' {
			c -= 13
		}
		out[i] = c
	}
	return out
}
