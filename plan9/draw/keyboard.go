package draw

import "log"

type Keyboardctl struct {
	C <-chan rune
}

func (d *Display) InitKeyboard() *Keyboardctl {
	ch := make(chan rune, 20)
	go kbdproc(d, ch)
	return &Keyboardctl{ch}
}

func kbdproc(d *Display, ch chan rune) {
	for {
		r, err := d.conn.ReadKbd()
		if err != nil {
			log.Fatal(err)
		}
		ch <- r
	}
}
