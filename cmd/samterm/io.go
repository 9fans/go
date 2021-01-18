package main

import (
	"log"

	"9fans.net/go/draw"
	"9fans.net/go/plan9/client"
)

var (
	protodebug bool
	cursorfd   int
	plumbfd    *client.Fid
	got        int
	block      int
	kbdc       rune
	resized    bool

	externcmd   []rune
	plumbc      chan string
	hostp       []byte
	hostc       chan []byte
	mousectl    *draw.Mousectl
	mousep      *draw.Mouse
	keyboardctl *draw.Keyboardctl
)

func initio() {
	if protodebug {
		print("mouse\n")
	}
	mousectl = display.InitMouse()
	if mousectl == nil {
		log.Fatalf("mouse init failed")
	}
	mousep = &mousectl.Mouse
	if protodebug {
		print("kbd\n")
	}
	keyboardctl = display.InitKeyboard()
	if keyboardctl == nil {
		log.Fatalf("keyboard init failed")
	}
	if protodebug {
		print("hoststart\n")
	}
	hoststart()
	if protodebug {
		print("plumbstart\n")
	}
	if err := plumbstart(); err != nil {
		if protodebug {
			print("extstart\n")
		}
		extstart()
	}
	if protodebug {
		print("initio done\n")
	}
}

func getmouse() {
	mousectl.Read()
}

func mouseunblock() {
	got &^= 1 << RMouse
}

func kbdblock() { /* ca suffit */
	block = 1<<RKeyboard | 1<<RPlumb
}

func button(but int) int {
	getmouse()
	return mousep.Buttons & (1 << (but - 1))
}

func externload(cmd string) {
	// TODO(rsc): drawtopwindow()
	externcmd = []rune(cmd)
	got |= 1 << RPlumb
}

func waitforio() int {
	if got&^block != 0 {
		return got &^ block
	}

again:
	hostc := hostc
	if block&(1<<RHost) != 0 {
		hostc = nil
	}
	plumbc := plumbc
	if block&(1<<RPlumb) != 0 {
		plumbc = nil
	}
	kbdch := keyboardctl.C
	if block&(1<<RKeyboard) != 0 {
		kbdch = nil
	}
	mousec := mousectl.C
	if block&(1<<RMouse) != 0 {
		mousec = nil
	}
	resizec := mousectl.Resize
	if block&(1<<RResize) != 0 {
		resizec = nil
	}

	display.Flush()
	select {
	case hostp = <-hostc:
		block = 0
		got |= 1 << RHost
	case cmd := <-plumbc:
		externload(cmd)
		got |= 1 << RPlumb
	case r := <-kbdch:
		kbdc = r
		got |= 1 << RKeyboard
	case mousectl.Mouse = <-mousec:
		got |= 1 << RMouse
	case <-resizec:
		resized = true
		/* do the resize in line if we've finished initializing and we're not in a blocking state */
		if hasunlocked && block == 0 && RESIZED() {
			resize()
		}
		goto again
	}
	return got
}

func rcvchar() int {
	if got&(1<<RHost) == 0 {
		return -1
	}
	c := hostp[0]
	hostp = hostp[1:]
	if len(hostp) == 0 {
		got &^= 1 << RHost
	}
	return int(c)
}

func rcvstring() []byte {
	got &^= 1 << RHost
	return hostp
}

func getch() int {
	var c int
	for c = rcvchar(); c == -1; c = rcvchar() {
		block = ^(1 << RHost)
		waitforio()
		block = 0
	}
	return c
}

func externchar() rune {
	for got&(1<<RPlumb)&^block != 0 {
		r := externcmd[0]
		externcmd = externcmd[1:]
		if r == 0 {
			continue
		}
		return r
	}
	return -1
}

var kpeekc rune = -1

func ecankbd() bool {
	if kpeekc >= 0 {
		return true
	}
	select {
	default:
		return false
	case r := <-keyboardctl.C:
		kpeekc = r
		return true
	}
}

func ekbd() rune {
	if kpeekc >= 0 {
		c := kpeekc
		kpeekc = -1
		return c
	}
	return <-keyboardctl.C
}

func kbdchar() rune {
	if c := externchar(); c > 0 {
		return c
	}
	if got&(1<<RKeyboard) != 0 {
		c := kbdc
		kbdc = -1
		got &^= 1 << RKeyboard
		return c
	}

Loop:
	for {
		select {
		default:
			break Loop
		case cmd := <-plumbc:
			externload(cmd)
			c := externchar()
			if c > 0 {
				return c
			}
		}
	}
	if !ecankbd() {
		return -1
	}
	return ekbd()
}

func qpeekc() rune {
	return kbdc
}

func RESIZED() bool {
	if resized {
		if err := display.Attach(draw.RefNone); err != nil {
			panic("can't reattach to window: " + err.Error())
		}
		resized = false
		return true
	}
	return false
}
