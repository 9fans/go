// #include "sam.h"

// Sam is a multi-file text editor.
// This is a Go port of the original C version.
// See https://9fans.github.io/plan9port/man/man1/sam.html
// and https://9p.io/sys/doc/sam/sam.pdf for details.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var genbuf [BLOCKSIZE]rune
var genbuf2 [BLOCKSIZE]rune

var iofile IOFile

type IOFile interface {
	io.ReadWriteCloser
	Stat() (os.FileInfo, error)
}

var panicking int
var rescuing int
var genstr String
var rhs String
var curwd String
var cmdstr String
var empty []rune
var curfile *File
var flist *File
var cmd *File
var mainloop int
var tempfile []*File
var quitok bool = true
var downloaded bool
var dflag bool
var Rflag bool
var machine string
var home string
var bpipeok bool
var termlocked int
var samterm string = SAMTERM
var rsamname string = RSAM
var lastfile *File
var disk *Disk
var seq int

var winsize string

var baddir = [9]rune{'<', 'b', 'a', 'd', 'd', 'i', 'r', '>', '\n'}

/* extern  */

func main() {
	var aflag bool
	var Wflag string

	flag.BoolVar(&Dflag, "D", Dflag, "-D") // debug
	flag.BoolVar(&dflag, "d", dflag, "-d")
	flag.BoolVar(&Rflag, "R", Rflag, "-R")
	flag.StringVar(&machine, "r", machine, "-r")
	flag.StringVar(&samterm, "t", samterm, "-t")
	flag.StringVar(&rsamname, "s", rsamname, "-s")
	flag.BoolVar(&aflag, "a", aflag, "-a (for samterm)")
	flag.StringVar(&Wflag, "W", Wflag, "-W (for samterm)")

	flag.Usage = usage
	flag.Parse()

	termargs := []string{"samterm"}
	if aflag {
		termargs = append(termargs, "-a")
	}
	if Wflag != "" {
		termargs = append(termargs, "-W", Wflag)
	}

	Strinit(&cmdstr)
	Strinit0(&lastpat)
	Strinit0(&lastregexp)
	Strinit0(&genstr)
	Strinit0(&rhs)
	Strinit0(&curwd)
	Strinit0(&plan9cmd)
	home, _ = os.UserHomeDir()
	disk = diskinit()
	if home == "" {
		home = "/"
	}
	fileargs := flag.Args()
	if !dflag {
		startup(machine, Rflag, termargs, fileargs)
	}
	siginit()
	getcurwd()
	if len(fileargs) > 0 {
		for i := 0; i < len(fileargs); i++ {
			func() {
				defer func() {
					e := recover()
					if e == nil || e == &mainloop {
						return
					}
					panic(e)
				}()

				t := tmpcstr(fileargs[i])
				Strduplstr(&genstr, t)
				freetmpstr(t)
				fixname(&genstr)
				logsetname(newfile(), &genstr)
			}()
		}
	} else if !downloaded {
		newfile()
	}
	seq++
	if len(file) > 0 {
		current(file[0])
	}

	for {
		func() {
			defer func() {
				e := recover()
				if e == nil || e == &mainloop {
					return
				}
				panic(e)
			}()
			cmdloop()
			trytoquit() /* if we already q'ed, quitok will be TRUE */
			os.Exit(0)
		}()
	}
}

func usage() {
	dprint("usage: sam [-d] [-t samterm] [-s sam name] [-r machine] [file ...]\n")
	os.Exit(2)
}

func rescue() {
	nblank := 0
	if rescuing++; rescuing > 1 {
		return
	}
	iofile = nil
	for _, f := range file {
		if f == cmd || f.b.nc == 0 || !fileisdirty(f) {
			continue
		}
		if iofile == nil {
			var err error
			iofile, err = os.OpenFile(filepath.Join(home, "sam.save"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
			if err != nil {
				return
			}
		}
		var buf string
		if len(f.name.s) > 0 {
			buf = string(f.name.s)
		} else {
			buf = fmt.Sprintf("nameless.%d", nblank)
			nblank++
		}
		root := os.Getenv("PLAN9")
		if root == "" {
			root = "/usr/local/plan9"
		}
		fmt.Fprintf(iofile, "#!/bin/sh\n%s/bin/samsave '%s' $* <<'---%s'\n", root, buf, buf)
		addr.r.p1 = 0
		addr.r.p2 = f.b.nc
		writeio(f)
		fmt.Fprintf(iofile, "\n---%s\n", (string)(buf))
	}
}

func panic_(format string, args ...interface{}) {
	if panicking++; panicking == 1 {
		s := fmt.Sprintf(format, args...)
		func() {
			defer func() {
				e := recover()
				if e == nil || e == &mainloop {
					return
				}
				panic(e)
			}()

			wasd := downloaded
			downloaded = false
			dprint("sam: panic: %s\n", s)
			if wasd {
				fmt.Fprintf(os.Stderr, "sam: panic: %s\n", s)
			}
			rescue()
			panic("abort")
		}()
	}
}

func hiccough(s string) {
	if rescuing != 0 {
		os.Exit(1)
	}
	if s != "" {
		dprint("%s\n", s)
		// panic(s) // TODO(rsc)
	}
	resetcmd()
	resetxec()
	resetsys()
	if iofile != nil {
		iofile.Close()
	}

	/*
	 * back out any logged changes & restore old sequences
	 */
	for _, f := range file {
		if f == cmd {
			continue
		}
		if f.seq == seq {
			bufdelete(&f.epsilon, 0, f.epsilon.nc)
			f.seq = f.prevseq
			f.dot.r = f.prevdot
			f.mark = f.prevmark
			m := Clean
			if f.prevmod {
				m = Dirty
			}
			state(f, m)
		}
	}

	update()
	if curfile != nil {
		if curfile.unread {
			curfile.unread = false
		} else if downloaded {
			outTs(Hcurrent, curfile.tag)
		}
	}
	panic(&mainloop)
}

func intr() {
	error_(Eintr)
}

func trytoclose(f *File) {
	if f == cmd { /* possible? */
		return
	}
	if f.deleted {
		return
	}
	if fileisdirty(f) && !f.closeok {
		f.closeok = true
		var buf string
		if len(f.name.s) > 0 {
			buf = string(f.name.s)
		} else {
			buf = "nameless file"
		}
		error_s(Emodified, buf)
	}
	f.deleted = true
}

func trytoquit() {
	if !quitok {
		for _, f := range file {
			if f != cmd && fileisdirty(f) {
				quitok = true
				eof = false
				error_(Echanges)
			}
		}
	}
}

func load(f *File) {
	Strduplstr(&genstr, &f.name)
	filename(f)
	if len(f.name.s) > 0 {
		saveaddr := addr
		edit(f, 'I')
		addr = saveaddr
	} else {
		f.unread = false
		f.cleanseq = f.seq
	}

	fileupdate(f, true, true)
}

func cmdupdate() {
	if cmd != nil && cmd.seq != 0 {
		fileupdate(cmd, false, downloaded)
		cmd.dot.r.p2 = cmd.b.nc
		cmd.dot.r.p1 = cmd.dot.r.p2
		telldot(cmd)
	}
}

func delete(f *File) {
	if downloaded && f.rasp != nil {
		outTs(Hclose, f.tag)
	}
	delfile(f)
	if f == curfile {
		current(nil)
	}
}

func update() {
	settempfile()
	anymod := 0
	for _, f := range tempfile {
		if f == cmd { /* cmd gets done in main() */
			continue
		}
		if f.deleted {
			delete(f)
			continue
		}
		if f.seq == seq && fileupdate(f, false, downloaded) {
			anymod++
		}
		if f.rasp != nil {
			telldot(f)
		}
	}
	if anymod != 0 {
		seq++
	}
}

func current(f *File) *File {
	curfile = f
	return curfile
}

func edit(f *File, cmd rune) {
	empty := true
	if cmd == 'r' {
		logdelete(f, addr.r.p1, addr.r.p2)
	}
	if cmd == 'e' || cmd == 'I' {
		logdelete(f, Posn(0), f.b.nc)
		addr.r.p2 = f.b.nc
	} else if f.b.nc != 0 || (f.name.s != nil && Strcmp(&genstr, &f.name) != 0) {
		empty = false
	}
	var err error
	iofile, err = os.Open(genc)
	if err != nil {
		if curfile != nil && curfile.unread {
			curfile.unread = false
		}
		error_r(Eopen, genc, err)
	}
	var nulls bool
	p := readio(f, &nulls, empty, true)
	cp := p
	if cmd == 'e' || cmd == 'I' {
		cp = -1
	}
	closeio(cp)
	if cmd == 'r' {
		f.ndot.r.p1 = addr.r.p2
		f.ndot.r.p2 = addr.r.p2 + p
	} else {
		f.ndot.r.p2 = 0
		f.ndot.r.p1 = f.ndot.r.p2
	}
	f.closeok = empty
	if quitok {
		quitok = empty
	} else {
		quitok = false
	}
	m := Clean
	if !empty && nulls {
		m = Dirty
	}
	state(f, m)
	if empty && !nulls {
		f.cleanseq = f.seq
	}
	if cmd == 'e' {
		filename(f)
	}
}

func getname(f *File, s *String, save bool) int {
	Strzero(&genstr)
	genc = ""
	var c rune
	if s == nil || len(s.s) == 0 { /* no name provided */
		if f != nil {
			Strduplstr(&genstr, &f.name)
		}
	} else {
		c = s.s[0]
		if c != ' ' && c != '\t' {
			error_(Eblank)
		}
		var i int
		for i = 0; i < len(s.s); i++ {
			c = s.s[i]
			if !(c == ' ') && !(c == '\t') {
				break
			}
		}
		for i < len(s.s) && s.s[i] > ' ' {
			Straddc(&genstr, s.s[i])
			i++
		}
		if i != len(s.s) {
			error_(Enewline)
		}
		fixname(&genstr)
		if f != nil && (save || len(f.name.s) == 0) {
			logsetname(f, &genstr)
			if Strcmp(&f.name, &genstr) != 0 {
				f.closeok = false
				quitok = f.closeok
				f.info = nil
				state(f, Dirty) /* if it's 'e', fix later */
			}
		}
	}
	genc = Strtoc(&genstr)
	return len(genstr.s)
}

func filename(f *File) {
	genc = string(genstr.s)
	ch := func(s string, b bool) byte {
		if b {
			return s[1]
		}
		return s[0]
	}
	dprint("%c%c%c %s\n", ch(" '", f.mod), ch("-+", f.rasp != nil), ch(" .", f == curfile), genc)
}

func undostep(f *File, isundo bool) {
	mod := f.mod
	var p1 int
	var p2 int
	fileundo(f, isundo, true, &p1, &p2, true)
	f.ndot = f.dot
	if f.mod {
		f.closeok = false
		quitok = false
	} else {
		f.closeok = true
	}

	if f.mod != mod {
		f.mod = mod
		m := Clean
		if mod {
			m = Dirty
		}
		state(f, m)
	}
}

func undo(isundo bool) int {
	max := undoseq(curfile, isundo)
	if max == 0 {
		return 0
	}
	settempfile()
	for _, f := range tempfile {
		if f != cmd && undoseq(f, isundo) == max {
			undostep(f, isundo)
		}
	}
	return 1
}

func readcmd(s *String) int {
	if flist != nil {
		fileclose(flist)
	}
	flist = fileopen()

	addr.r.p1 = 0
	addr.r.p2 = flist.b.nc
	retcode := plan9(flist, '<', s, false)
	fileupdate(flist, false, false)
	flist.seq = 0
	if flist.b.nc > BLOCKSIZE {
		error_(Etoolong)
	}
	Strzero(&genstr)
	Strinsure(&genstr, flist.b.nc)
	bufread(&flist.b, Posn(0), genbuf[:flist.b.nc])
	copy(genstr.s, genbuf[:])
	return retcode
}

func getcurwd() {
	wd, _ := os.Getwd()
	t := tmpcstr(wd)
	Strduplstr(&curwd, t)
	freetmpstr(t)
	if len(curwd.s) == 0 {
		warn(Wpwd)
	} else if curwd.s[len(curwd.s)-1] != '/' {
		Straddc(&curwd, '/')
	}
}

func cd(str *String) {
	getcurwd()
	var s string
	if getname(nil, str, false) != 0 {
		s = genc
	} else {
		s = home
	}
	if err := os.Chdir(s); err != nil {
		syserror("chdir", err)
	}
	/*
		fd := syscall.Open("/dev/wdir", syscall.O_WRONLY)
		if fd > 0 {
			write(fd, s, strlen(s))
		}
	*/
	dprint("!\n")
	var owd String
	Strinit(&owd)
	Strduplstr(&owd, &curwd)
	getcurwd()
	settempfile()
	/*
	 * Two passes so that if we have open
	 * /a/foo.c and /b/foo.c and cd from /b to /a,
	 * we don't ever have two foo.c simultaneously.
	 */
	for _, f := range tempfile {
		if f != cmd && len(f.name.s) > 0 && f.name.s[0] != '/' {
			Strinsert(&f.name, &owd, Posn(0))
			fixname(&f.name)
			sortname(f)
		}
	}
	for _, f := range tempfile {
		if f != cmd && Strispre(&curwd, &f.name) {
			fixname(&f.name)
			sortname(f)
		}
	}
	Strclose(&owd)
}

func loadflist(s *String) bool {
	var i int
	var c rune
	if len(s.s) > 0 {
		c = s.s[0]
	}
	for i = 0; i < len(s.s) && (s.s[i] == ' ' || s.s[i] == '\t'); i++ {
	}
	if (c == ' ' || c == '\t') && (i >= len(s.s) || s.s[i] != '\n') {
		if i < len(s.s) && s.s[i] == '<' {
			Strdelete(s, 0, int(i)+1)
			readcmd(s)
		} else {
			Strzero(&genstr)
			for ; i < len(s.s); i++ {
				c := s.s[i]
				if c == '\n' {
					break
				}
				Straddc(&genstr, c)
			}
		}
	} else {
		if c != '\n' {
			error_(Eblank)
		}
		Strdupl(&genstr, empty)
	}
	genc = Strtoc(&genstr)
	debug("loadflist %s\n", genc)
	return len(genstr.s) > 0
}

func readflist(readall, delete bool) *File {
	var t String
	Strinit(&t)
	i := 0
	var f *File
	for ; f == nil || readall || delete; i++ { /* ++ skips blank */
		debug("readflist %q\n", string(genstr.s))
		Strdelete(&genstr, Posn(0), i)
		for i = 0; i < len(genstr.s); i++ {
			c := genstr.s[i]
			if c != ' ' && c != '\t' && c != '\n' {
				break
			}
		}
		if i >= len(genstr.s) {
			break
		}
		Strdelete(&genstr, Posn(0), i)
		for i = 0; i < len(genstr.s); i++ {
			c := genstr.s[i]
			if c == ' ' || c == '\t' || c == '\n' {
				break
			}
		}
		if i == 0 {
			break
		}
		Strduplstr(&t, tmprstr(genstr.s[:i]))
		debug("dup %s\n", string(t.s))
		fixname(&t)
		debug("lookfile %s\n", string(t.s))
		f = lookfile(&t)
		if delete {
			if f == nil {
				warn_S(Wfile, &t)
			} else {
				trytoclose(f)
			}
		} else if f == nil && readall {
			f = newfile()
			logsetname(f, &t)
		}
	}
	Strclose(&t)
	return f
}

func tofile(s *String) *File {
	if s.s[0] != ' ' {
		error_(Eblank)
	}
	var f *File
	if !loadflist(s) {
		f = lookfile(&genstr) /* empty string ==> nameless file */
		if f == nil {
			error_s(Emenu, genc)
		}
	} else {
		f = readflist(false, false)
		if f == nil {
			error_s(Emenu, genc)
		}
	}
	return current(f)
}

func getfile(s *String) *File {
	var f *File
	if !loadflist(s) {
		f = newfile()
		logsetname(f, &genstr)
	} else {
		debug("read? %q\n", genc)
		f = readflist(true, false)
		if f == nil {
			error_(Eblank)
		}
	}
	return current(f)
}

func closefiles(f *File, s *String) {
	if len(s.s) == 0 {
		if f == nil {
			error_(Enofile)
		}
		trytoclose(f)
		return
	}
	if s.s[0] != ' ' {
		error_(Eblank)
	}
	if !loadflist(s) {
		error_(Enewline)
	}
	readflist(false, true)
}

func fcopy(f *File, addr2 Address) {
	var ni int
	for p := addr.r.p1; p < addr.r.p2; p += ni {
		ni = addr.r.p2 - p
		if ni > BLOCKSIZE {
			ni = BLOCKSIZE
		}
		bufread(&f.b, p, genbuf[:ni])
		loginsert(addr2.f, addr2.r.p2, tmprstr(genbuf[:ni]).s)
	}
	addr2.f.ndot.r.p2 = addr2.r.p2 + (f.dot.r.p2 - f.dot.r.p1)
	addr2.f.ndot.r.p1 = addr2.r.p2
}

func move(f *File, addr2 Address) {
	if addr.r.p2 <= addr2.r.p2 {
		logdelete(f, addr.r.p1, addr.r.p2)
		fcopy(f, addr2)
	} else if addr.r.p1 >= addr2.r.p2 {
		fcopy(f, addr2)
		logdelete(f, addr.r.p1, addr.r.p2)
	} else {
		error_(Eoverlap)
	}
}

func nlcount(f *File, p0 Posn, p1 Posn) Posn {
	nl := 0

	for p0 < p1 {
		tmp30 := p0
		p0++
		if filereadc(f, tmp30) == '\n' {
			nl++
		}
	}
	return nl
}

func printposn(f *File, charsonly bool) {
	if !charsonly {
		l1 := 1 + nlcount(f, Posn(0), addr.r.p1)
		l2 := l1 + nlcount(f, addr.r.p1, addr.r.p2)
		/* check if addr ends with '\n' */
		if addr.r.p2 > 0 && addr.r.p2 > addr.r.p1 && filereadc(f, addr.r.p2-1) == '\n' {
			l2--
		}
		dprint("%d", l1)
		if l2 != l1 {
			dprint(",%d", l2)
		}
		dprint("; ")
	}
	dprint("#%d", addr.r.p1)
	if addr.r.p2 != addr.r.p1 {
		dprint(",#%d", addr.r.p2)
	}
	dprint("\n")
}

func settempfile() {
	tempfile = append(tempfile[:0], file...)
}
