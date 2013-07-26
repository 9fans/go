package draw

import (
	"image"
	"log"
)

type Mouse struct {
	image.Point
	Buttons int
	Msec    uint32
}

// TODO: Mouse field is racy but okay.

type Mousectl struct {
	Mouse
	C       <-chan Mouse
	Resize  <-chan bool
	Display *Display
}

func (d *Display) InitMouse() *Mousectl {
	ch := make(chan Mouse, 0)
	rch := make(chan bool, 2)
	mc := &Mousectl{
		C:       ch,
		Resize:  rch,
		Display: d,
	}
	go mouseproc(mc, d, ch, rch)
	return mc
}

func mouseproc(mc *Mousectl, d *Display, ch chan Mouse, rch chan bool) {
	for {
		m, resized, err := d.conn.ReadMouse()
		if err != nil {
			log.Fatal(err)
		}
		if resized {
			rch <- true
		}
		mm := Mouse{image.Point{m.X, m.Y}, m.Buttons, uint32(m.Msec)}
		ch <- mm
		/*
		 * mc->m is updated after send so it doesn't have wrong value if we block during send.
		 * This means that programs should receive into mc->Mouse (see readmouse() above) if
		 * they want full synchrony.
		 */
		mc.Mouse = mm
	}
}

func (mc *Mousectl) Read() Mouse {
	mc.Display.Flush()
	m := <-mc.C
	mc.Mouse = m
	return m
}
