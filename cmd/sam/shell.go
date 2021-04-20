// #include "sam.h"
// #include "parse.h"

package main

import (
	"io"
	"os"
	"os/exec"
)

/* extern var mainloop jmp_buf */

var errfile string
var plan9cmd String /* null terminated */
var plan9buf Buffer

func setname(ecmd *exec.Cmd, f *File) {
	var buf string
	if f != nil {
		buf = string(f.name.s)
	}
	// % to be like acme
	ecmd.Env = append(os.Environ(), "samfile="+buf, "%="+buf)
}

func plan9(f *File, type_ rune, s *String, nest bool) int {
	if len(s.s) == 0 && len(plan9cmd.s) == 0 {
		error_(Enocmd)
	} else if len(s.s) != 0 {
		Strduplstr(&plan9cmd, s)
	}
	/*
		var pipe1 [2]int
		if type_ != '!' && pipe(pipe1) == -1 {
			error_(Epipe)
		}
	*/
	if type_ == '|' {
		snarf(f, addr.r.p1, addr.r.p2, &plan9buf, 1)
	}

	ecmd := exec.Command(SHPATH, "-c", Strtoc(&plan9cmd))
	setname(ecmd, f)

	ecmd.Stdin = os.Stdin
	ecmd.Stdout = os.Stdout
	ecmd.Stderr = os.Stderr

	if downloaded {
		errfile = samerr()
		os.Remove(errfile)
		ecmd.Stderr = nil
		ecmd.Stdout = nil
		ecmd.Stdin = nil
		if fd, err := os.Create(errfile); err == nil {
			ecmd.Stderr = fd
			if type_ == '>' || type_ == '!' {
				ecmd.Stdout = fd
			}
		}
	}

	var stdout IOFile
	if type_ == '<' || type_ == '|' {
		ecmd.Stdout = nil
		p, err := ecmd.StdoutPipe()
		if err != nil {
			error_(Epipe)
		}
		stdout = p.(IOFile)
	}

	var stdin IOFile
	if type_ == '>' || type_ == '|' {
		ecmd.Stdin = nil
		p, err := ecmd.StdinPipe()
		if err != nil {
			error_(Epipe)
		}
		stdin = p.(IOFile)
	}

	if type_ == '|' {
		go func() {
			defer func() {
				stdin.Close()
				e := recover()
				if e == nil {
					return
				}
				if e == &mainloop {
					os.Exit(1)
				}
				panic(e)
			}()

			var m int
			for l := 0; l < plan9buf.nc; l += m {
				m = plan9buf.nc - l
				if m > BLOCKSIZE-1 {
					m = BLOCKSIZE - 1
				}
				bufread(&plan9buf, l, genbuf2[:m])
				c := []byte(string(genbuf2[:m]))
				Write(stdin, c)
				// free(c)
			}
		}()
	}

	xerr := ecmd.Start()

	switch type_ {
	case '<', '|':
		if downloaded && addr.r.p1 != addr.r.p2 {
			outTl(Hsnarflen, addr.r.p2-addr.r.p1)
		}
		snarf(f, addr.r.p1, addr.r.p2, &snarfbuf, 0)
		logdelete(f, addr.r.p1, addr.r.p2)
		iofile = stdout
		f.tdot.p1 = -1
		var nulls bool
		f.ndot.r.p2 = addr.r.p2 + readio(f, &nulls, false, false)
		f.ndot.r.p1 = addr.r.p2
		closeio(-1)

	case '>':
		iofile = stdin
		bpipeok = true
		writeio(f)
		bpipeok = false
		closeio(-1)
	}

	if xerr == nil {
		xerr = ecmd.Wait()
	}
	if type_ == '|' || type_ == '<' {
		if xerr != nil {
			warn(Wbadstatus)
		}
	}
	if downloaded {
		checkerrs()
	}
	if !nest {
		dprint("!\n")
	}
	if xerr == nil {
		return 0
	}
	return -1
}

func checkerrs() {
	if info, err := os.Stat(errfile); err == nil && info.Size() > 0 {
		f, err := os.Open(errfile)
		if err == nil {
			buf := make([]byte, BLOCKSIZE-10)
			n, err := io.ReadFull(f, buf)
			if err == nil && n > 0 {
				nl := 0
				p := 0
				for ; nl < 25 && p < len(buf); p++ {
					if buf[p] == '\n' {
						nl++
					}
				}
				buf = buf[:p]
				dprint("%s", buf)
				if int64(len(buf)) < info.Size()-1 { // TODO(rsc): Why -1
					dprint("(sam: more in %s)\n", errfile)
				}
			}
			f.Close()
		}
	} else {
		os.Remove(errfile)
	}
}
