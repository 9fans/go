package drawfcall

import (
	"image"
	"io"
	"log"
)

const (
	_ = iota
	Rerror
	Trdmouse
	Rrdmouse
	Tmoveto
	Rmoveto
	Tcursor
	Rcursor
	Tbouncemouse
	Rbouncemouse
	Trdkbd
	Rrdkbd
	Tlabel
	Rlabel
	Tinit
	Rinit
	Trdsnarf
	Rrdsnarf
	Twrsnarf
	Rwrsnarf
	Trddraw
	Rrddraw
	Twrdraw
	Rwrdraw
	Ttop
	Rtop
	Tresize
	Rresize
	Tmax
)

const MAXMSG = 4 << 20

type Msg struct {
	Type    uint8
	Tag     uint8
	Mouse   Mouse
	Resized bool
	Cursor  Cursor
	Arrow   bool
	Rune    rune
	Winsize string
	Label   string
	Snarf   []byte
	Error   string
	Data    []byte
	Count   int
	Rect    image.Rectangle
}

type Mouse struct {
	image.Point
	Buttons int
	Msec    int
}

type Cursor struct {
	image.Point
	Clr [32]byte
	Set [32]byte
}

func (m *Msg) Marshal() []byte {
	log.Panicf("Marshal: %v\n", m)
	return nil
}

func ReadMsg(r io.Reader) ([]byte, error) {
	size := make([]byte, 4)
	_, err := io.ReadFull(r, size)
	if err != nil {
		return nil, err
	}
	n, _ := gbit32(size[:])
	buf := make([]byte, n)
	copy(buf, size)
	_, err = io.ReadFull(r, buf[4:])
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return buf, nil
}

func (m *Msg) Unmarshal(b []byte) error {
	log.Panicf("Unmarshal: %q\n", b)
	return nil
}
