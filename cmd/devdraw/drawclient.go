//go:build ignore
// +build ignore

// #include <u.h>
// #include <libc.h>
// #include <bio.h>
// #include <draw.h>
// #include <mouse.h>
// #include <cursor.h>
// #include <drawfcall.h>

package main

type Cmd struct {
	cmd *C.char
	fn  func(int, **C.char)
}

var b Biobuf
var fd int
var buf [64 * 1024]uint8

func startsrv() {
	var p [2]int
	if pipe(p) < 0 {
		sysfatal("pipe")
	}
	pid := fork()
	if pid < 0 {
		sysfatal("fork")
	}
	if pid == 0 {
		close(p[0])
		dup(p[1], 0)
		dup(p[1], 1)
		execl("./o.devdraw", "o.devdraw", "-D", nil)
		sysfatal("exec: %r")
	}
	close(p[1])
	fd = p[0]
}

func domsg(m *Wsysmsg) int {
	n := convW2M(m, buf, sizeof(buf))
	fprint(2, "write %d to %d\n", n, fd)
	write(fd, buf, n)
	n = readwsysmsg(fd, buf, sizeof(buf))
	nn := convM2W(buf, n, m)
	assert(nn == n)
	if m.type_ == Rerror {
		return -1
	}
	return 0
}

func cmdinit(argc int, argv **C.char) {
	var m Wsysmsg
	memset(&m, 0, sizeof(m))
	m.type_ = Tinit
	m.winsize = "100x100"
	m.label = "label"
	if domsg(&m) < 0 {
		sysfatal("domsg")
	}
}

func cmdmouse(argc int, argv **C.char) {
	var m Wsysmsg
	memset(&m, 0, sizeof(m))
	m.type_ = Trdmouse
	if domsg(&m) < 0 {
		sysfatal("domsg")
	}
	var tmp1 unknown
	if m.resized != 0 {
		tmp1 = 'r'
	} else {
		tmp1 = 'm'
	}
	print("%c %d %d %d\n", tmp1, m.mouse.xy.x, m.mouse.xy.y, m.mouse.buttons)
}

func cmdkbd(argc int, argv **C.char) {
	var m Wsysmsg
	memset(&m, 0, sizeof(m))
	m.type_ = Trdkbd
	if domsg(&m) < 0 {
		sysfatal("domsg")
	}
	print("%d\n", m.rune_)
}

var cmdtab = [3]Cmd{
	Cmd{"init", cmdinit},
	Cmd{"mouse", cmdmouse},
	Cmd{"kbd", cmdkbd},
}

func main(argc int, argv **C.char) {
	startsrv()

	fprint(2, "started...\n")
	Binit(&b, 0, OREAD)
	for {
		p := Brdstr(&b, '\n', 1)
		if p == nil {
			break
		}
		fprint(2, "%s...\n", p)
		var f [20]*C.char
		nf := tokenize(p, f, len(f))
		var i int
		for i = 0; i < len(cmdtab); i++ {
			if strcmp(cmdtab[i].cmd, f[0]) == 0 {
				cmdtab[i].fn(nf, f)
				break
			}
		}
		if i == len(cmdtab) {
			print("! unrecognized command %s\n", f[0])
		}
		free(p)
	}
	exits(0)
}
