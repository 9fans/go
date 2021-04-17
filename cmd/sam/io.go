package main

import (
	"log"
	"os"
	"os/exec"
	"unicode/utf8"
)

const (
	NSYSFILE = 3
	NOFILE   = 128
)

func checkqid(f *File) {
	w := whichmenu(f)
	for i := 1; i < len(file); i++ {
		g := file[i]
		if w == i {
			continue
		}
		if os.SameFile(f.info, g.info) {
			warn_SS(Wdupfile, &f.name, &g.name)
		}
	}
}

var genc string

func writef(f *File) {
	newfile := 0
	samename := Strcmp(&genstr, &f.name) == 0
	name := Strtoc(&f.name)
	info, err := os.Stat(name)
	if err != nil {
		newfile++
	} else if samename && (!os.SameFile(f.info, info) || f.info.Size() != info.Size() || !f.info.ModTime().Equal(info.ModTime())) {
		f.info = info
		warn_S(Wdate, &genstr)
		return
	}
	genc = string(genstr.s)
	io, err = os.Create(genc)
	if err != nil {
		error_r(Ecreate, genc, err)
	}
	dprint("%s: ", genc)
	if info, err := io.Stat(); err == nil && info.Mode()&os.ModeAppend != 0 && info.Size() > 0 {
		error_(Eappend)
	}
	n := writeio(f)
	if len(f.name.s) == 0 || samename {
		if addr.r.p1 == 0 && addr.r.p2 == f.b.nc {
			f.cleanseq = f.seq
		}
		mod := Clean
		if f.cleanseq != f.seq {
			mod = Dirty
		}
		state(f, mod)
	}
	if newfile != 0 {
		dprint("(new file) ")
	}
	if addr.r.p2 > 0 && filereadc(f, addr.r.p2-1) != '\n' {
		warn(Wnotnewline)
	}
	closeio(n)
	if len(f.name.s) == 0 || samename {
		if info, err := os.Stat(name); err == nil {
			f.info = info
			checkqid(f)
		}
	}
}

func readio(f *File, nulls *bool, setdate, toterm bool) Posn {
	p := addr.r.p2
	*nulls = false
	b := 0
	var nt Posn
	if f.unread {
		nt = bufload(&f.b, 0, io, nulls)
		if toterm {
			raspload(f)
		}
	} else {
		var nr int
		for nt = 0; ; nt += nr {
			var buf [BLOCKSIZE]byte
			n, err := io.Read(buf[b:])
			if err != nil || n == 0 {
				break
			}
			n += b
			b = 0
			nr := 0
			s := buf[:n]
			for len(s) > 0 {
				if s[0] < utf8.RuneSelf {
					if s[0] != 0 {
						genbuf[nr] = rune(s[0])
						nr++
					} else {
						*nulls = true
					}
					s = s[1:]
					continue
				}
				if utf8.FullRune(s) {
					r, w := utf8.DecodeRune(s)
					if r != 0 {
						genbuf[nr] = r
						nr++
					} else {
						*nulls = true
					}
					s = s[w:]
					continue
				}
				b = copy(buf[:], s)
				break
			}
			loginsert(f, p, genbuf[:nr])
		}
	}
	if b != 0 {
		*nulls = true
	}
	if *nulls {
		warn(Wnulls)
	}
	if setdate {
		if info, err := io.Stat(); err == nil {
			f.info = info
			checkqid(f)
		}
	}
	return nt
}

func writeio(f *File) Posn {
	p := addr.r.p1
	for p < addr.r.p2 {
		var n int
		if addr.r.p2-p > BLOCKSIZE {
			n = BLOCKSIZE
		} else {
			n = addr.r.p2 - p
		}
		bufread(&f.b, p, genbuf[:n])
		c := []byte(string(genbuf[:n])) // TODO(rsc)
		if nw, err := io.Write(c); err != nil || nw != len(c) {
			// free(c)
			if p > 0 {
				p += n
			}
			break
		}
		// free(c)
		p += n
	}
	return p - addr.r.p1
}

func closeio(p Posn) {
	io.Close()
	io = nil
	if p >= 0 {
		dprint("#%d\n", p)
	}
}

var remotefd0 = os.Stdin
var remotefd1 = os.Stdout

func bootterm(machine string, argv []string) {
	if machine != "" {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin = remotefd0
		cmd.Stdout = remotefd1
		if err := cmd.Start(); err != nil {
			log.Fatal(err)
		}
		remotefd0.Close()
		remotefd1.Close()
		if err := cmd.Wait(); err != nil {
			log.Fatalf("samterm: %v", err)
		}
		os.Exit(0)
	}

	cmd := exec.Command(samterm, argv[1:]...)
	r1, w1, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}
	r2, w2, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}
	cmd.Stdin = r1
	cmd.Stdout = w2
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		log.Fatalf("samterm: %v", err)
	}
	r1.Close()
	w2.Close()

	os.Stdin = r2
	os.Stdout = w1
}

func connectto(machine string, files []string) {
	av := append([]string{RX, machine, rsamname, "-R"}, files...)
	cmd := exec.Command(av[0], av[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	err = cmd.Start()
	if err != nil {
		log.Fatalf("%s: %v", RX, err)
	}
	remotefd0 = stdout.(*os.File)
	remotefd1 = stdin.(*os.File)
}

func startup(machine string, Rflag bool, argv []string, files []string) {
	if machine != "" {
		connectto(machine, files)
	}
	if !Rflag {
		bootterm(machine, argv)
	}
	downloaded = true
	outTs(Hversion, VERSION)
}
