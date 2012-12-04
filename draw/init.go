package draw

import (
	"encoding/binary"
	"fmt"
	"image"
	"log"
	"os"
	"strings"
	"sync"

	"code.google.com/p/goplan9/draw/drawfcall"
)

// A Display represents a connection to a display.
type Display struct {
	mu      sync.Mutex
	conn    *drawfcall.Conn
	errch   chan<- error
	bufsize int
	buf     []byte
	imageid uint32
	qmask   *Image
	locking bool

	Image       *Image
	Screen      *Screen
	ScreenImage *Image
	Windows     *Image
	DPI         int // TODO fill in

	White       *Image
	Black       *Image
	Opaque      *Image
	Transparent *Image

	DefaultFont    *Font
	DefaultSubfont *Subfont
}

type Image struct {
	Display *Display
	ID      uint32
	Pix     Pix
	Depth   int
	Repl    bool
	R       image.Rectangle
	Clipr   image.Rectangle
	Next    *Image
	Screen  *Screen
}

type Screen struct {
	Display *Display
	ID      uint32
	Image   *Image
	Fill    *Image
}

const (
	Refbackup = 0
	Refnone   = 1
	Refmesg   = 2
)

const deffontname = "*default*"

// Init connects to a display.
func Init(errch chan<- error, fontname, label, winsize string) (*Display, error) {
	c, err := drawfcall.New()
	if err != nil {
		return nil, err
	}
	d := &Display{
		conn:    c,
		errch:   errch,
		bufsize: 10000,
	}
	d.buf = make([]byte, 0, d.bufsize+5) // 5 for final flush
	if err := c.Init(label, winsize); err != nil {
		c.Close()
		return nil, err
	}

	i, err := d.getimage0(nil)
	if err != nil {
		c.Close()
		return nil, err
	}

	d.Image = i
	d.White, err = d.AllocImage(image.Rect(0, 0, 1, 1), GREY1, true, DWhite)
	if err != nil {
		return nil, err
	}
	d.Black, err = d.AllocImage(image.Rect(0, 0, 1, 1), GREY1, true, DBlack)
	if err != nil {
		return nil, err
	}
	d.Opaque = d.White
	d.Transparent = d.Black

	/*
	 * Set up default font
	 */
	df, err := getdefont(d)
	if err != nil {
		return nil, err
	}
	d.DefaultSubfont = df

	if fontname == "" {
		fontname = os.Getenv("font")
	}

	/*
	 * Build fonts with caches==depth of screen, for speed.
	 * If conversion were faster, we'd use 0 and save memory.
	 */
	var font *Font
	if fontname == "" {
		buf := []byte(fmt.Sprintf("%d %d\n0 %d\t%s\n", df.Height, df.Ascent,
			df.N-1, deffontname))
		//fmt.Printf("%q\n", buf)
		//BUG: Need something better for this	installsubfont("*default*", df);
		font, err = d.BuildFont(buf, deffontname)
	} else {
		font, err = d.OpenFont(fontname) // BUG: grey fonts
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "imageinit: can't open default font %s: %v\n", fontname, err)
		return nil, err
	}
	d.DefaultFont = font

	d.Screen, err = i.AllocScreen(d.White, false)
	if err != nil {
		return nil, err
	}
	d.ScreenImage = d.Image // temporary, for d.ScreenImage.Pix
	d.ScreenImage, err = _allocwindow(nil, d.Screen, i.R, 0, DWhite)
	if err != nil {
		return nil, err
	}
	if err := d.Flush(true); err != nil {
		log.Fatal(err)
	}

	screen := d.ScreenImage
	screen.Draw(screen.R, d.White, nil, image.ZP)
	if err := d.Flush(true); err != nil {
		log.Fatal(err)
	}

	return d, nil
}

func (d *Display) getimage0(i *Image) (*Image, error) {
	if i != nil {
		_freeimage1(i)
		*i = Image{}
	}

	a := d.bufimage(2)
	a[0] = 'J'
	a[1] = 'I'
	if err := d.Flush(false); err != nil {
		fmt.Fprintf(os.Stderr, "cannot read screen info: %v\n", err)
		return nil, err
	}

	info := make([]byte, 12*12)
	n, err := d.conn.ReadDraw(info)
	if err != nil {
		return nil, err
	}
	if n < len(info) {
		return nil, fmt.Errorf("short info from rddraw")
	}

	pix, _ := ParsePix(strings.TrimSpace(string(info[2*12 : 3*12])))
	if i == nil {
		i = new(Image)
	}
	*i = Image{
		Display: d,
		ID:      0,
		Pix:     pix,
		Depth:   pix.Depth(),
		Repl:    atoi(info[3*12:]) > 0,
		R:       ator(info[4*12:]),
		Clipr:   ator(info[8*12:]),
	}
	return i, nil
}

func (d *Display) Attach(ref int) error {
	oi := d.Image
	i, err := d.getimage0(oi)
	if err != nil {
		return err
	}
	d.Image = i
	d.Screen.Free()
	d.Screen, err = i.AllocScreen(d.White, false)
	if err != nil {
		return err
	}
	_freeimage1(d.ScreenImage)
	d.ScreenImage, err = _allocwindow(d.ScreenImage, d.Screen, i.R, ref, DWhite)
	if err != nil {
		log.Fatal("aw", err)
	}
	return err
}

func (d *Display) Close() error {
	if d == nil {
		return nil
	}
	return d.conn.Close()
}

// TODO: lockdisplay unlockdisplay locking

// TODO: drawerror

func (d *Display) flush() error {
	if len(d.buf) == 0 {
		return nil
	}
	_, err := d.conn.WriteDraw(d.buf)
	d.buf = d.buf[:0]
	if err != nil {
		fmt.Fprintf(os.Stderr, "draw flush: %v\n", err)
		return err
	}
	return nil
}

func (d *Display) Flush(visible bool) error {
	if visible {
		d.bufsize++
		a := d.bufimage(1)
		d.bufsize--
		a[0] = 'v'
	}

	return d.flush()
}

func (d *Display) bufimage(n int) []byte {
	if d == nil || n < 0 || n > d.bufsize {
		panic("bad count in bufimage")
	}
	if len(d.buf)+n > d.bufsize {
		if err := d.flush(); err != nil {
			panic("bufimage flush: " + err.Error())
		}
	}
	i := len(d.buf)
	d.buf = d.buf[:i+n]
	return d.buf[i:]
}

const DefaultDPI = 133

func (d *Display) Scale(n int) int {
	if d == nil || d.DPI <= DefaultDPI {
		return n
	}
	return (n*d.DPI + DefaultDPI/2) / DefaultDPI
}

func atoi(b []byte) int {
	i := 0
	for i < len(b) && b[i] == ' ' {
		i++
	}
	n := 0
	for ; i < len(b) && '0' <= b[i] && b[i] <= '9'; i++ {
		n = n*10 + int(b[i]) - '0'
	}
	return n
}

func atop(b []byte) image.Point {
	return image.Pt(atoi(b), atoi(b[12:]))
}

func ator(b []byte) image.Rectangle {
	return image.Rectangle{atop(b), atop(b[2*12:])}
}

func bplong(b []byte, n uint32) {
	binary.LittleEndian.PutUint32(b, n)
}

func bpshort(b []byte, n uint16) {
	binary.LittleEndian.PutUint16(b, n)
}
