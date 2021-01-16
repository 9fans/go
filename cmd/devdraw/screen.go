package main

import (
	"fmt"
	"image"
	godraw "image/draw"
	"log"
	"os"
	"sync"
	"time"

	"9fans.net/go/draw"
	"9fans.net/go/draw/memdraw"
	"golang.org/x/exp/shiny/driver"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
)

var ScreenPix = draw.XBGR32

func gfx_main() {
	driver.Main(shinyMain)
}

var attachChan = make(chan func(screen.Screen) (screen.Window, *Client))
var theWindow screen.Window

func rpc_attach(client *Client, label, winsize string) (*memdraw.Image, error) {
	done := make(chan bool)
	attachChan <- func(s screen.Screen) (screen.Window, *Client) {
		w, err := s.NewWindow(&screen.NewWindowOptions{
			Title: label,
			// TODO winsize
		})
		if err != nil {
			log.Fatal(err)
		}
		theWindow = w

	Loop:
		for {
			switch e := w.NextEvent().(type) {
			default:
				log.Printf("skipping %T %+v\n", e, e)

			case size.Event:
				r := draw.Rect(0, 0, e.WidthPx, e.HeightPx)
				log.Printf("rect %v\n", r)
				i, err := memdraw.AllocImage(r, ScreenPix)
				if err != nil {
					log.Fatal(err)
				}
				client.impl = &theImpl{i: i, rgba: memimageToRGBA(i)}
				client.displaydpi = int(e.PixelsPerPt * 72)
				client.mouserect = i.R
				w.SendFirst(e)
				break Loop
			}
		}
		close(done)
		return w, client
	}
	<-done

	return client.impl.(*theImpl).i, nil
}

func memimageToRGBA(i *memdraw.Image) *image.RGBA {
	return &image.RGBA{
		Pix:    i.BytesAt(i.R.Min),
		Stride: int(i.Width) * 4,
		Rect:   i.R,
	}
}

type theImpl struct {
	i    *memdraw.Image
	b    screen.Buffer
	rgba *image.RGBA
}

func (*theImpl) rpc_setlabel(client *Client, label string) {
	done := make(chan bool)
	theWindow.SendFirst(func() {
		// TODO
		close(done)
	})
	<-done
}

func rpc_shutdown() {
}

func (impl *theImpl) rpc_flush(client *Client, r draw.Rectangle) {
	theWindow.SendFirst(func() {
		// drawlk protects the pixel data in impl.i.
		// In addition to avoiding a technical data race,
		// the lock avoids drawing partial updates, which makes
		// animations like sweeping windows much less flickery.
		drawlk.Lock()
		defer drawlk.Unlock()
		fmt.Fprintf(os.Stderr, "flush %v\n", r)
		//TODO godraw.Draw(impl.b.RGBA(), r, impl.rgba, r.Min, godraw.Src)
		godraw.Draw(impl.b.RGBA(), impl.b.Bounds(), impl.rgba, impl.b.Bounds().Min, godraw.Src)
		theWindow.Upload(image.Point{}, impl.b, impl.b.Bounds())
		theWindow.Publish()
	})
}

func (*theImpl) rpc_resizeimg(client *Client) {
	// TODO
}

var rpcgfxlk sync.Mutex

func rpc_gfxdrawlock() {
	rpcgfxlk.Lock()
}

func rpc_gfxdrawunlock() {
	rpcgfxlk.Unlock()
}

func (*theImpl) rpc_topwin(client *Client) {
}

func (*theImpl) rpc_resizewindow(client *Client, r draw.Rectangle) {
}

func (*theImpl) rpc_setmouse(client *Client, p draw.Point) {
}

func (*theImpl) rpc_setcursor(client *Client, c *draw.Cursor, c2 *draw.Cursor2) {
}

func rpc_getsnarf() []byte {
	return nil
}

func rpc_putsnarf(data []byte) {
}

func (*theImpl) rpc_bouncemouse(client *Client, m draw.Mouse) {
}

func shinyMain(s screen.Screen) {
	gfx_started()

	w, client := (<-attachChan)(s)
	close(attachChan)
	defer w.Release()
	impl := client.impl.(*theImpl)

	// TODO call gfx_started

	defer func() {
		if impl.b != nil {
			impl.b.Release()
			impl.b = nil
		}
	}()

	println("SHINYMAIN")

	var buttons int

	for {
		fmt.Fprintf(os.Stderr, "EVWAIT\n")
		e := w.NextEvent()
		fmt.Fprintf(os.Stderr, "EV %T %+v\n", e, e)
		switch e := e.(type) {
		case func():
			e()

		case lifecycle.Event:
			gfx_abortcompose(client)
			if e.To == lifecycle.StageDead {
				return
			}

		case key.Event:
			if e.Direction != key.DirPress {
				break
			}
			ch := e.Rune
			if ch == '\r' {
				ch = '\n'
			}
			if ch == -1 && int(e.Code) < len(codeKeys) {
				ch = codeKeys[e.Code]
			}
			if ch > 0 {
				gfx_keystroke(client, ch)
			}

		case mouse.Event:
			// TODO keyboard modifiers
			// TODO buttons
			if e.Button > 0 {
				if e.Direction == mouse.DirPress {
					buttons |= 1 << (e.Button - 1)
				} else {
					buttons &^= 1 << (e.Button - 1)
				}
			}
			gfx_abortcompose(client)
			fmt.Fprintf(os.Stderr, "mousetrack %d %d %#b\n", int(e.X), int(e.Y), buttons)
			gfx_mousetrack(client, int(e.X), int(e.Y), buttons, uint32(time.Now().UnixNano()/1e6))

		case paint.Event:
			fmt.Fprintf(os.Stderr, "PAINT\n")
			w.Upload(image.Point{}, impl.b, impl.b.Bounds())
			w.Publish()

		case size.Event:
			// TODO call gfx_replacescreenimg
			if impl.b != nil {
				impl.b.Release()
				impl.b = nil
			}
			var err error
			impl.b, err = s.NewBuffer(e.Size())
			if err != nil {
				log.Fatal(err)
			}

			r := draw.Rect(0, 0, e.WidthPx, e.HeightPx)
			if r != impl.i.R {
				i, err := memdraw.AllocImage(r, ScreenPix)
				if err != nil {
					log.Fatal(err)
				}
				impl.i = i
				impl.rgba = memimageToRGBA(i)
				client.mouserect = i.R
				client.displaydpi = int(e.PixelsPerPt * 72)
				gfx_replacescreenimage(client, i)
			} else {
				godraw.Draw(impl.b.RGBA(), r, impl.rgba, r.Min, godraw.Src)
			}

		case error:
			log.Print(e)
		}
	}
}

var codeKeys = [...]rune{
	// CodeCapsLock
	key.CodeF1:  draw.KeyFn | 1,
	key.CodeF2:  draw.KeyFn | 2,
	key.CodeF3:  draw.KeyFn | 3,
	key.CodeF4:  draw.KeyFn | 4,
	key.CodeF5:  draw.KeyFn | 5,
	key.CodeF6:  draw.KeyFn | 6,
	key.CodeF7:  draw.KeyFn | 7,
	key.CodeF8:  draw.KeyFn | 8,
	key.CodeF9:  draw.KeyFn | 9,
	key.CodeF10: draw.KeyFn | 10,
	key.CodeF11: draw.KeyFn | 11,
	key.CodeF12: draw.KeyFn | 12,
	// draw.KeyFn | 13 is where the non-F keys start,
	// so CodeF13 through CodeF24 are not representable

	// CodePause
	key.CodeInsert: draw.KeyInsert,
	key.CodeHome:   draw.KeyHome,
	key.CodePageUp: draw.KeyPageUp,
	// CodeDeleteForward
	key.CodeEnd:        draw.KeyEnd,
	key.CodePageDown:   draw.KeyPageDown,
	key.CodeRightArrow: draw.KeyRight,
	key.CodeLeftArrow:  draw.KeyLeft,
	key.CodeDownArrow:  draw.KeyDown,
	key.CodeUpArrow:    draw.KeyUp,
	// CodeKeypadNumLock
	// CodeHelp
	// CodeMute
	// CodeVolumeUp
	// CodeVolumeDown
	key.CodeLeftControl: draw.KeyCtl,
	key.CodeLeftShift:   draw.KeyShift,
	key.CodeLeftAlt:     draw.KeyAlt,
	// CodeLeftGUI
	key.CodeRightControl: draw.KeyCtl,
	key.CodeRightShift:   draw.KeyShift,
	key.CodeRightAlt:     draw.KeyAlt,
	// CodeRightGUI
}
