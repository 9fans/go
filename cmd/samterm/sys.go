package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	"9fans.net/go/draw"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
	"9fans.net/go/plumb"
)

var exname string

func usage() {
	fmt.Fprintf(os.Stderr, "usage: samterm -a -W winsize\n")
	os.Exit(2)
}

func getscreen() {
	flag.BoolVar(&autoindent, "a", autoindent, "enable autoindent")
	winsize := flag.String("W", "", "set initial window `size`")
	log.SetPrefix("samterm: ")
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()

	d, err := draw.Init(nil, "", "sam", *winsize)
	if err != nil {
		log.Fatal("drawinit: ", err)
	}
	display = d
	screen = display.ScreenImage
	font = display.Font

	t := os.Getenv("tabstop")
	if t != "" {
		maxtab, _ = strconv.Atoi(t)
	}
	screen.Draw(screen.Clipr, display.White, nil, draw.ZP)
}

func screensize(w *int, h *int) bool {
	if w != nil {
		*w = 0
	}
	if h != nil {
		*h = 0
	}
	return false
}

func snarfswap(fromsam []byte) (fromterm []byte) {
	defer display.WriteSnarf(fromsam)
	_, size, err := display.ReadSnarf(nil)
	if err != nil {
		return nil
	}
	fromterm = make([]byte, size)
	n, size, err := display.ReadSnarf(fromterm)
	if n < size {
		return nil
	}
	return fromterm[:n]
}

func dumperrmsg(count int, typ Hmesg, count0 int, c int) {
	fmt.Fprintf(os.Stderr, "samterm: host mesg: count %d %#x %#x %#x %s...ignored\n", count, typ, count0, c, rcvstring())
}

func removeextern() {
	os.Remove(exname)
}

func extproc(c chan string, fd *os.File) {
	buf := make([]byte, READBUFSIZE)
	for {
		n, err := fd.Read(buf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "samterm: extern read error: %v\n", err)
			return /* not a fatal error */
		}
		c <- string(buf[:n])
	}
}

func plumb2cmd(m *plumb.Message) string {
	act := m.LookupAttr("action")
	if act != "" && act != "showfile" {
		/* can't handle other cases yet */
		return ""
	}
	cmd := "B " + string(m.Data)
	if cmd[len(cmd)-1] != '\n' {
		cmd += "\n"
	}
	addr := m.LookupAttr("addr")
	if addr != "" {
		cmd += addr + "\n"
	}
	return cmd
}

func plumbproc(edit *client.Fid) {
	r := bufio.NewReader(edit)
	for {
		m := new(plumb.Message)
		if err := m.Recv(r); err != nil {
			fmt.Fprintf(os.Stderr, "samterm: plumb read error: %v\n", err)
			return /* not a fatal error */
		}
		if cmd := plumb2cmd(m); cmd != "" {
			plumbc <- cmd
		}
	}
}

func plumbstart() error {
	f, err := plumb.Open("send", plan9.OWRITE)
	if err == nil { /* not open is ok */
		plumbfd = f
	}

	f, err = plumb.Open("edit", plan9.OREAD)
	if err != nil {
		return err
	}

	plumbc = make(chan string)
	go plumbproc(f)
	return nil
}

func hostproc(c chan []byte) {
	var buf [2][READBUFSIZE]byte
	i := 0
	for {
		i = 1 - i /* toggle */
		n, err := hostfd[0].Read(buf[i][:])
		if false {
			fmt.Fprintf(os.Stderr, "hostproc %d\n", n)
		}
		if err != nil {
			if err == io.EOF {
				if exiting != 0 { // TODO(rsc) races
					return
				}
				err = io.ErrUnexpectedEOF
			}
			log.Fatalf("host read error: %v", err)
		}
		if false {
			fmt.Fprintf(os.Stderr, "hostproc send %d\n", i)
		}
		c <- buf[i][:n]
	}
}

func hoststart() {
	hostc = make(chan []byte)
	go hostproc(hostc)
}
