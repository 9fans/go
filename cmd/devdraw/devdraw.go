/*
 * /dev/draw simulator -- handles the messages prepared by the draw library.
 * Doesn't simulate the file system part, just the messages.
 */

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"unsafe"

	"9fans.net/go/draw"
	"9fans.net/go/draw/memdraw"
)

var drawdebug int

var drawlk sync.Mutex

func draw_initdisplaymemimage(c *Client, m *memdraw.Image) {
	c.screenimage = m
	m.ScreenRef = 1
	c.slot = 0
	c.clientid = 1
	c.op = draw.SoverD
}

// gfx_replacescreenimage replaces c's screen image with m.
// It is called by the host driver on the main host thread.
func gfx_replacescreenimage(c *Client, m *memdraw.Image) {
	drawlk.Lock()
	om := c.screenimage
	c.screenimage = m
	m.ScreenRef = 1
	if om != nil {
		om.ScreenRef--
		if om.ScreenRef == 0 {
			memdraw.Free(om)
		}
	}
	drawlk.Unlock()
	gfx_mouseresized(c)
}

func drawrefreshscreen(l *DImage, client *Client) {
	for l != nil && l.dscreen == nil {
		l = l.fromname
	}
	if l != nil && l.dscreen.owner != client {
		l.dscreen.owner.refreshme = 1
	}
}

func drawrefresh(m *memdraw.Image, r draw.Rectangle, v interface{}) {
	if v == nil {
		return
	}
	x := v.(*Refx)
	c := x.client
	d := x.dimage
	var ref *Refresh
	for ref = c.refresh; ref != nil; ref = ref.next {
		if ref.dimage == d {
			draw.CombineRect(&ref.r, r)
			return
		}
	}
	ref = new(Refresh)
	if ref != nil {
		ref.dimage = d
		ref.r = r
		ref.next = c.refresh
		c.refresh = ref
	}
}

func addflush(c *Client, r draw.Rectangle) {
	if !draw.RectClip(&r, c.screenimage.R) {
		return
	}

	if c.flushrect.Min.X >= c.flushrect.Max.X {
		c.flushrect = r
		c.waste = 0
		return
	}
	nbb := c.flushrect
	draw.CombineRect(&nbb, r)
	ar := r.Dx() * r.Dy()
	abb := c.flushrect.Dx() * c.flushrect.Dy()
	anbb := nbb.Dx() * nbb.Dy()
	/*
	 * Area of new waste is area of new bb minus area of old bb,
	 * less the area of the new segment, which we assume is not waste.
	 * This could be negative, but that's OK.
	 */
	c.waste += anbb - abb - ar
	if c.waste < 0 {
		c.waste = 0
	}
	/*
	 * absorb if:
	 *	total area is small
	 *	waste is less than half total area
	 * 	rectangles touch
	 */
	if anbb <= 1024 || c.waste*2 < anbb || draw.RectXRect(c.flushrect, r) {
		c.flushrect = nbb
		return
	}
	/* emit current state */
	fr := c.flushrect
	c.flushrect = r
	c.waste = 0
	if fr.Min.X < fr.Max.X {
		// Unlock drawlk because rpc_flush may want to run on gfx thread,
		// and gfx thread might be blocked on drawlk trying to install a new screen
		// during a resize.
		rpc_gfxdrawunlock()
		drawlk.Unlock()
		c.impl.rpc_flush(c, fr)
		drawlk.Lock()
		rpc_gfxdrawlock()
	}
}

func dstflush(c *Client, dstid int, dst *memdraw.Image, r draw.Rectangle) {
	if dstid == 0 {
		draw.CombineRect(&c.flushrect, r)
		return
	}
	/* how can this happen? -rsc, dec 12 2002 */
	if dst == nil {
		fmt.Fprintf(os.Stderr, "nil dstflush\n")
		return
	}
	l := dst.Layer
	if l == nil {
		return
	}
	for {
		if l.Screen.Image.Data != c.screenimage.Data {
			return
		}
		r = r.Add(l.Delta)
		l = l.Screen.Image.Layer
		if l == nil {
			break
		}
	}
	addflush(c, r)
}

func drawflush(c *Client) {
	r := c.flushrect
	c.flushrect = draw.Rect(10000, 10000, -10000, -10000)
	if r.Min.X < r.Max.X {
		// Unlock drawlk because rpc_flush may want to run on gfx thread,
		// and gfx thread might be blocked on drawlk trying to install a new screen
		// during a resize.
		rpc_gfxdrawunlock()
		drawlk.Unlock()
		c.impl.rpc_flush(c, r)
		drawlk.Lock()
		rpc_gfxdrawlock()
	}
}

func drawlookupname(client *Client, str string) *DName {
	for i := 0; i < len(client.name); i++ {
		name := &client.name[i]
		if name.name == str {
			return name
		}
	}
	return nil
}

func drawgoodname(client *Client, d *DImage) int {
	/* if window, validate the screen's own images */
	if d.dscreen != nil {
		if drawgoodname(client, d.dscreen.dimage) == 0 || drawgoodname(client, d.dscreen.dfill) == 0 {
			return 0
		}
	}
	if d.name == "" {
		return 1
	}
	n := drawlookupname(client, d.name)
	if n == nil || n.vers != d.vers {
		return 0
	}
	return 1
}

func drawlookup(client *Client, id int, checkname int) *DImage {
	d := client.dimage[id&HASHMASK]
	for d != nil {
		if d.id == id {
			/*
			 * BUG: should error out but too hard.
			 * Return 0 instead.
			 */
			if checkname != 0 && drawgoodname(client, d) == 0 {
				return nil
			}
			return d
		}
		d = d.next
	}
	return nil
}

func drawlookupdscreen(c *Client, id int) *DScreen {
	s := c.dscreen
	for s != nil {
		if s.id == id {
			return s
		}
		s = s.next
	}
	return nil
}

func drawlookupscreen(client *Client, id int, cs **CScreen) *DScreen {
	s := client.cscreen
	for s != nil {
		if s.dscreen.id == id {
			*cs = s
			return s.dscreen
		}
		s = s.next
	}
	/* caller must check! */
	return nil
}

func drawinstall(client *Client, id int, i *memdraw.Image, dscreen *DScreen) *memdraw.Image {
	d := new(DImage)
	if d == nil {
		return nil
	}
	d.id = id
	d.ref = 1
	d.name = ""
	d.vers = 0
	d.image = i
	if i.ScreenRef != 0 {
		i.ScreenRef++
	}
	d.fchar = nil
	d.fromname = nil
	d.dscreen = dscreen
	d.next = client.dimage[id&HASHMASK]
	client.dimage[id&HASHMASK] = d
	return i
}

func drawinstallscreen(client *Client, d *DScreen, id int, dimage *DImage, dfill *DImage, public int) *memdraw.Screen {
	c := new(CScreen)
	if dimage != nil && dimage.image != nil && dimage.image.Pix == 0 {
		fmt.Fprintf(os.Stderr, "bad image %p in drawinstallscreen", dimage.image)
		panic("drawinstallscreen")
	}

	if c == nil {
		return nil
	}
	if d == nil {
		d = new(DScreen)
		if d == nil {
			return nil
		}
		s := new(memdraw.Screen)
		if s == nil {
			return nil
		}
		s.Frontmost = nil
		s.Rearmost = nil
		d.dimage = dimage
		if dimage != nil {
			s.Image = dimage.image
			dimage.ref++
		}
		d.dfill = dfill
		if dfill != nil {
			s.Fill = dfill.image
			dfill.ref++
		}
		d.ref = 0
		d.id = id
		d.screen = s
		d.public = public
		d.next = client.dscreen
		d.owner = client
		client.dscreen = d
	}
	c.dscreen = d
	d.ref++
	c.next = client.cscreen
	client.cscreen = c
	return d.screen
}

func drawdelname(client *Client, name *DName) {
	i := 0
	for &client.name[i] != name {
		i++
	}
	copy(client.name[i:], client.name[i+1:])
	client.name = client.name[:len(client.name)-1]
}

func drawfreedscreen(client *Client, this *DScreen) {
	this.ref--
	if this.ref < 0 {
		fmt.Fprintf(os.Stderr, "negative ref in drawfreedscreen\n")
	}
	if this.ref > 0 {
		return
	}
	ds := client.dscreen
	if ds == this {
		client.dscreen = this.next
		goto Found
	}
	for {
		next := ds.next
		if next == nil {
			break
		} /* assign = */
		if next == this {
			ds.next = this.next
			goto Found
		}
		ds = next
	}
	/*
	 * Should signal Enodrawimage, but too hard.
	 */
	return

Found:
	if this.dimage != nil {
		drawfreedimage(client, this.dimage)
	}
	if this.dfill != nil {
		drawfreedimage(client, this.dfill)
	}
}

func drawfreedimage(client *Client, dimage *DImage) {
	dimage.ref--
	if dimage.ref < 0 {
		fmt.Fprintf(os.Stderr, "negative ref in drawfreedimage\n")
	}
	if dimage.ref > 0 {
		return
	}

	/* any names? */
	for i := 0; i < len(client.name); {
		if client.name[i].dimage == dimage {
			drawdelname(client, &client.name[i])
		} else {
			i++
		}
	}
	if dimage.fromname != nil { /* acquired by name; owned by someone else*/
		drawfreedimage(client, dimage.fromname)
		return
	}
	ds := dimage.dscreen
	l := dimage.image
	dimage.dscreen = nil /* paranoia */
	dimage.image = nil
	if ds != nil {
		if l.Data == client.screenimage.Data {
			addflush(client, l.Layer.Screenr)
		}
		l.Layer.Refreshptr = nil
		if drawgoodname(client, dimage) != 0 {
			memdraw.LDelete(l)
		} else {
			memdraw.LFree(l)
		}
		drawfreedscreen(client, ds)
	} else {
		if l.ScreenRef == 0 {
			memdraw.Free(l)
		} else {
			l.ScreenRef--
			if l.ScreenRef == 0 {
				memdraw.Free(l)
			}
		}
	}
}

func drawuninstallscreen(client *Client, this *CScreen) {
	cs := client.cscreen
	if cs == this {
		client.cscreen = this.next
		drawfreedscreen(client, this.dscreen)
		return
	}
	for {
		next := cs.next
		if next == nil {
			break
		} /* assign = */
		if next == this {
			cs.next = this.next
			drawfreedscreen(client, this.dscreen)
			return
		}
		cs = next
	}
}

func drawuninstall(client *Client, id int) int {
	var d *DImage
	for l := &client.dimage[id&HASHMASK]; ; l = &d.next {
		d = *l
		if d == nil {
			break
		}
		if d.id == id {
			*l = d.next
			drawfreedimage(client, d)
			return 0
		}
	}
	return -1
}

func drawaddname(client *Client, di *DImage, str string) error {
	for i := range client.name {
		name := &client.name[i]
		if name.name == str {
			return fmt.Errorf("image name in use")
		}
	}
	client.name = append(client.name, DName{})
	new := &client.name[len(client.name)-1]
	new.name = str
	new.dimage = di
	new.client = client
	client.namevers++
	new.vers = client.namevers
	return nil
}

func drawclientop(cl *Client) draw.Op {
	op := cl.op
	cl.op = draw.SoverD
	return op
}

func drawimage(client *Client, a []uint8) *memdraw.Image {
	d := drawlookup(client, rd4(a), 1)
	if d == nil {
		return nil /* caller must check! */
	}
	return d.image
}

func rd4(b []byte) int {
	return int(int32(binary.LittleEndian.Uint32(b)))
}

func drawrectangle(r *draw.Rectangle, a []uint8) {
	r.Min.X = rd4(a[0*4:])
	r.Min.Y = rd4(a[1*4:])
	r.Max.X = rd4(a[2*4:])
	r.Max.Y = rd4(a[3*4:])
}

func drawpoint(p *draw.Point, a []uint8) {
	p.X = rd4(a[0*4:])
	p.Y = rd4(a[1*4:])
}

func drawchar(dst *memdraw.Image, p draw.Point, src *memdraw.Image, sp *draw.Point, font *DImage, index int, op draw.Op) draw.Point {
	fc := &font.fchar[index]
	var r draw.Rectangle
	r.Min.X = p.X + int(fc.left)
	r.Min.Y = p.Y - (font.ascent - int(fc.miny))
	r.Max.X = r.Min.X + (int(fc.maxx) - int(fc.minx))
	r.Max.Y = r.Min.Y + (int(fc.maxy) - int(fc.miny))
	var sp1 draw.Point
	sp1.X = sp.X + int(fc.left)
	sp1.Y = sp.Y + int(fc.miny)
	memdraw.Draw(dst, r, src, sp1, font.image, draw.Pt(fc.minx, int(fc.miny)), op)
	p.X += int(fc.width)
	sp.X += int(fc.width)
	return p
}

func drawcoord(p []uint8, oldx int, newx *int) []uint8 {
	if len(p) == 0 {
		return nil
	}
	b := p[0]
	p = p[1:]
	x := int(b & 0x7F)
	if b&0x80 != 0 {
		if len(p) < 1 {
			return nil
		}
		x |= int(p[0]) << 7
		x |= int(p[1]) << 15
		p = p[2:]
		if x&(1<<22) != 0 {
			x |= ^0 << 23
		}
	} else {
		if b&0x40 != 0 {
			x |= ^0 << 7
		}
		x += oldx
	}
	*newx = x
	return p
}

func draw_dataread(cl *Client, a []byte) (int, error) {
	drawlk.Lock()
	defer drawlk.Unlock()

	if cl.readdata == nil {
		return 0, fmt.Errorf("no draw data")
	}
	if len(a) < len(cl.readdata) {
		return 0, fmt.Errorf("short read")
	}

	// TODO(rsc) reuse cl.readdata
	n := copy(a, cl.readdata)
	cl.readdata = nil
	return n, nil
}

func draw_datawrite(client *Client, v []byte) (int, error) {
	drawlk.Lock()
	rpc_gfxdrawlock()
	a := v
	m := 0
	oldn := len(v)
	var err error

	for {
		a = a[m:]
		if len(a) == 0 {
			break
		}
		fmt.Fprintf(os.Stderr, "msgwrite %d(%c)...", len(a), a[0])
		var refx *Refx
		var reffn memdraw.Refreshfn
		var r draw.Rectangle
		var clipr draw.Rectangle
		var sp draw.Point
		var q draw.Point
		var pp []draw.Point
		var p draw.Point
		var scrn *memdraw.Screen
		var src *memdraw.Image
		var mask *memdraw.Image
		var lp []*memdraw.Image
		var l *memdraw.Image
		var i *memdraw.Image
		var dst *memdraw.Image
		var fc *FChar
		var dscrn *DScreen
		var dn *DName
		var ll *DImage
		var font *DImage
		var dsrc *DImage
		var ddst *DImage
		var di *DImage
		var cs *CScreen
		var value draw.Color
		var chan_ draw.Pix
		var refresh uint8
		var y int
		var scrnid int
		var repl int
		var oy int
		var ox int
		var oesize int
		var nw int
		var ni int
		var j int
		var esize int
		var dstid int
		var doflush int
		var ci int
		var c int
		switch a[0] {
		/*fmt.Fprintf(os.Stderr, "bad command %d\n", a[0]); */
		default:
			err = fmt.Errorf("bad draw command")
			goto error

		/* allocate: 'b' id[4] screenid[4] refresh[1] chan[4] repl[1]
		R[4*4] clipR[4*4] rrggbbaa[4]
		*/
		case 'b':
			m = 1 + 4 + 4 + 1 + 4 + 1 + 4*4 + 4*4 + 4
			if len(a) < m {
				goto Eshortdraw
			}
			dstid = rd4(a[1:])
			scrnid = int(binary.LittleEndian.Uint16(a[5:]))
			refresh = a[9]
			chan_ = draw.Pix(binary.LittleEndian.Uint32(a[10:]))
			repl = int(a[14])
			drawrectangle(&r, a[15:])
			drawrectangle(&clipr, a[31:])
			value = draw.Color(binary.LittleEndian.Uint32(a[47:]))
			if drawlookup(client, dstid, 0) != nil {
				goto Eimageexists
			}
			if scrnid != 0 {
				dscrn = drawlookupscreen(client, scrnid, &cs)
				if dscrn == nil {
					goto Enodrawscreen
				}
				scrn = dscrn.screen
				if repl != 0 || chan_ != scrn.Image.Pix {
					err = fmt.Errorf("image parameters incompatibile with screen")
					goto error
				}
				reffn = nil
				switch refresh {
				case draw.RefBackup:
					break
				case draw.RefNone:
					reffn = memdraw.LNoRefresh
				case draw.RefMesg:
					reffn = drawrefresh
				default:
					err = fmt.Errorf("unknown refresh method")
					goto error
				}
				l, err = memdraw.LAlloc(scrn, r, reffn, 0, value)
				if err != nil {
					goto Edrawmem
				}
				addflush(client, l.Layer.Screenr)
				l.Clipr = clipr
				draw.RectClip(&l.Clipr, r)
				if drawinstall(client, dstid, l, dscrn) == nil {
					memdraw.LDelete(l)
					goto Edrawmem
				}
				dscrn.ref++
				if reffn != nil {
					refx = nil
					if funcPC(reffn) == funcPC(drawrefresh) {
						refx = new(Refx)
						if refx == nil {
							if drawuninstall(client, dstid) < 0 {
								goto Enodrawimage
							}
							goto Edrawmem
						}
						refx.client = client
						refx.dimage = drawlookup(client, dstid, 1)
					}
					memdraw.LSetRefresh(l, reffn, refx)
				}
				continue
			}
			i, err = memdraw.AllocImage(r, chan_)
			if err != nil {
				goto Edrawmem
			}
			if repl != 0 {
				i.Flags |= memdraw.Frepl
			}
			i.Clipr = clipr
			if repl == 0 {
				draw.RectClip(&i.Clipr, r)
			}
			if drawinstall(client, dstid, i, nil) == nil {
				memdraw.Free(i)
				goto Edrawmem
			}
			fmt.Fprintf(os.Stderr, "ALLOC %p %v %v %x\n", i, r, i.Clipr, value)
			memdraw.FillColor(i, value)
			continue

		/* allocate screen: 'A' id[4] imageid[4] fillid[4] public[1] */
		case 'A':
			m = 1 + 4 + 4 + 4 + 1
			if len(a) < m {
				goto Eshortdraw
			}
			dstid = rd4(a[1:])
			if dstid == 0 {
				goto Ebadarg
			}
			if drawlookupdscreen(client, dstid) != nil {
				goto Escreenexists
			}
			ddst = drawlookup(client, rd4(a[5:]), 1)
			dsrc = drawlookup(client, rd4(a[9:]), 1)
			if ddst == nil || dsrc == nil {
				goto Enodrawimage
			}
			if drawinstallscreen(client, nil, dstid, ddst, dsrc, int(a[13])) == nil {
				goto Edrawmem
			}
			continue

		/* set repl and clip: 'c' dstid[4] repl[1] clipR[4*4] */
		case 'c':
			m = 1 + 4 + 1 + 4*4
			if len(a) < m {
				goto Eshortdraw
			}
			ddst = drawlookup(client, rd4(a[1:]), 1)
			if ddst == nil {
				goto Enodrawimage
			}
			if ddst.name != "" {
				err = fmt.Errorf("can't change repl/clipr of shared image")
				goto error
			}
			dst = ddst.image
			if a[5] != 0 {
				dst.Flags |= memdraw.Frepl
			}
			drawrectangle(&dst.Clipr, a[6:])
			continue

		/* draw: 'd' dstid[4] srcid[4] maskid[4] R[4*4] P[2*4] P[2*4] */
		case 'd':
			m = 1 + 4 + 4 + 4 + 4*4 + 2*4 + 2*4
			if len(a) < m {
				goto Eshortdraw
			}
			dst = drawimage(client, a[1:])
			dstid = rd4(a[1:])
			src = drawimage(client, a[5:])
			mask = drawimage(client, a[9:])
			if dst == nil || src == nil || mask == nil {
				goto Enodrawimage
			}
			drawrectangle(&r, a[13:])
			drawpoint(&p, a[29:])
			drawpoint(&q, a[37:])
			op := drawclientop(client)
			fmt.Fprintf(os.Stderr, "DRAW %p %v %p %v %p %v %v\n", dst, r, src, p, mask, q, op)
			memdraw.Draw(dst, r, src, p, mask, q, op)
			dstflush(client, dstid, dst, r)
			continue

		/* toggle debugging: 'D' val[1] */
		case 'D':
			m = 1 + 1
			if len(a) < m {
				goto Eshortdraw
			}
			drawdebug = int(a[1])
			continue

		/* ellipse: 'e' dstid[4] srcid[4] center[2*4] a[4] b[4] thick[4] sp[2*4] alpha[4] phi[4]*/
		case 'e',
			'E':
			m = 1 + 4 + 4 + 2*4 + 4 + 4 + 4 + 2*4 + 2*4
			if len(a) < m {
				goto Eshortdraw
			}
			dst := drawimage(client, a[1:])
			dstid := rd4(a[1:])
			src := drawimage(client, a[5:])
			if dst == nil || src == nil {
				goto Enodrawimage
			}
			drawpoint(&p, a[9:])
			e0 := rd4(a[17:])
			e1 := rd4(a[21:])
			if e0 < 0 || e1 < 0 {
				err = fmt.Errorf("invalid ellipse semidiameter")
				goto error
			}
			j := rd4(a[25:])
			if j < 0 {
				err = fmt.Errorf("negative ellipse thickness")
				goto error
			}

			drawpoint(&sp, a[29:])
			c = j
			if a[0] == 'E' {
				c = -1
			}
			ox := rd4(a[37:])
			oy := rd4(a[41:])
			op := drawclientop(client)
			/* high bit indicates arc angles are present */
			if ox&(1<<31) != 0 {
				if ox&(1<<30) == 0 {
					ox &= ^(1 << 31)
				}
				memdraw.Arc(dst, p, e0, e1, c, src, sp, ox, oy, op)
			} else {
				memdraw.Ellipse(dst, p, e0, e1, c, src, sp, op)
			}
			dstflush(client, dstid, dst, draw.Rect(p.X-e0-j, p.Y-e1-j, p.X+e0+j+1, p.Y+e1+j+1))
			continue

		/* free: 'f' id[4] */
		case 'f':
			m = 1 + 4
			if len(a) < m {
				goto Eshortdraw
			}
			ll = drawlookup(client, rd4(a[1:]), 0)
			if ll != nil && ll.dscreen != nil && ll.dscreen.owner != client {
				ll.dscreen.owner.refreshme = 1
			}
			if drawuninstall(client, rd4(a[1:])) < 0 {
				goto Enodrawimage
			}
			continue

		/* free screen: 'F' id[4] */
		case 'F':
			m = 1 + 4
			if len(a) < m {
				goto Eshortdraw
			}
			if drawlookupscreen(client, rd4(a[1:]), &cs) == nil {
				goto Enodrawscreen
			}
			drawuninstallscreen(client, cs)
			continue

		/* initialize font: 'i' fontid[4] nchars[4] ascent[1] */
		case 'i':
			m = 1 + 4 + 4 + 1
			if len(a) < m {
				goto Eshortdraw
			}
			dstid = rd4(a[1:])
			if dstid == 0 {
				err = fmt.Errorf("can't use display as font")
				goto error
			}
			font = drawlookup(client, dstid, 1)
			if font == nil {
				goto Enodrawimage
			}
			if font.image.Layer != nil {
				err = fmt.Errorf("can't use window as font")
				goto error
			}
			ni = rd4(a[5:])
			if ni <= 0 || ni > 4096 {
				err = fmt.Errorf("bad font size (4096 chars max)")
				goto error
			}
			font.fchar = make([]FChar, ni)
			font.ascent = int(a[9])
			continue

		/* set image 0 to screen image */
		case 'J':
			m = 1
			if len(a) < m {
				goto Eshortdraw
			}
			if drawlookup(client, 0, 0) != nil {
				goto Eimageexists
			}
			drawinstall(client, 0, client.screenimage, nil)
			client.infoid = 0
			continue

		/* get image info: 'I' */
		case 'I':
			m = 1
			if len(a) < m {
				goto Eshortdraw
			}
			if client.infoid < 0 {
				goto Enodrawimage
			}
			if client.infoid == 0 {
				i = client.screenimage
				if i == nil {
					goto Enodrawimage
				}
			} else {
				di = drawlookup(client, client.infoid, 1)
				if di == nil {
					goto Enodrawimage
				}
				i = di.image
			}
			repl := 0
			if i.Flags&memdraw.Frepl != 0 {
				repl = 1
			}
			client.readdata = []byte(fmt.Sprintf("%11d %11d %11s %11d %11d %11d %11d %11d %11d %11d %11d %11d ",
				client.clientid, client.infoid, i.Pix.String(), repl,
				i.R.Min.X, i.R.Min.Y, i.R.Max.X, i.R.Max.Y,
				i.Clipr.Min.X, i.Clipr.Min.Y, i.Clipr.Max.X, i.Clipr.Max.Y))
			client.infoid = -1
			continue

		/* query: 'Q' n[1] queryspec[n] */
		case 'q':
			if len(a) < 2 {
				goto Eshortdraw
			}
			m = 1 + 1 + int(a[1])
			if len(a) < m {
				goto Eshortdraw
			}
			var buf bytes.Buffer
			for c = 0; c < int(a[1]); c++ {
				switch a[2+c] {
				default:
					err = fmt.Errorf("unknown query")
					goto error
				case 'd': /* dpi */
					if client.forcedpi != 0 {
						fmt.Fprintf(&buf, "%11d ", client.forcedpi)
					} else {
						fmt.Fprintf(&buf, "%11d ", client.displaydpi)
					}
				}
			}
			client.readdata = buf.Bytes()
			continue

		/* load character: 'l' fontid[4] srcid[4] index[2] R[4*4] P[2*4] left[1] width[1] */
		case 'l':
			m = 1 + 4 + 4 + 2 + 4*4 + 2*4 + 1 + 1
			if len(a) < m {
				goto Eshortdraw
			}
			font = drawlookup(client, rd4(a[1:]), 1)
			if font == nil {
				goto Enodrawimage
			}
			if len(font.fchar) == 0 {
				goto Enotfont
			}
			src = drawimage(client, a[5:])
			if src == nil {
				goto Enodrawimage
			}
			ci = int(binary.LittleEndian.Uint16(a[9:]))
			if ci >= len(font.fchar) {
				goto Eindex
			}
			drawrectangle(&r, a[11:])
			drawpoint(&p, a[27:])
			memdraw.Draw(font.image, r, src, p, memdraw.Opaque, p, draw.S)
			fc = &font.fchar[ci]
			fc.minx = r.Min.X
			fc.maxx = r.Max.X
			fc.miny = uint8(r.Min.Y)
			fc.maxy = uint8(r.Max.Y)
			fc.left = int8(a[35])
			fc.width = a[36]
			continue

		/* draw line: 'L' dstid[4] p0[2*4] p1[2*4] end0[4] end1[4] radius[4] srcid[4] sp[2*4] */
		case 'L':
			m = 1 + 4 + 2*4 + 2*4 + 4 + 4 + 4 + 4 + 2*4
			if len(a) < m {
				goto Eshortdraw
			}
			dst = drawimage(client, a[1:])
			dstid = rd4(a[1:])
			drawpoint(&p, a[5:])
			drawpoint(&q, a[13:])
			e0 := draw.End(rd4(a[21:]))
			e1 := draw.End(rd4(a[25:]))
			j = rd4(a[29:])
			if j < 0 {
				err = fmt.Errorf("negative line width")
				goto error
			}
			src = drawimage(client, a[33:])
			if dst == nil || src == nil {
				goto Enodrawimage
			}
			drawpoint(&sp, a[37:])
			op := drawclientop(client)
			memdraw.Line(dst, p, q, e0, e1, j, src, sp, op)
			/* avoid memlinebbox if possible */
			if dstid == 0 || dst.Layer != nil {
				/* BUG: this is terribly inefficient: update maximal containing rect*/
				r = memdraw.LineBBox(p, q, e0, e1, j)
				dstflush(client, dstid, dst, r.Inset(-(1 + 1 + j)))
			}
			continue

		/* create image mask: 'm' newid[4] id[4] */
		/*
			 *
					case 'm':
						m = 4+4;
						if(len(a) < m)
							goto Eshortdraw;
						break;
			 *
		*/

		/* attach to a named image: 'n' dstid[4] j[1] name[j] */
		case 'n':
			m = 1 + 4 + 1
			if len(a) < m {
				goto Eshortdraw
			}
			j = int(a[5])
			if j == 0 { /* give me a non-empty name please */
				goto Eshortdraw
			}
			m += j
			if len(a) < m {
				goto Eshortdraw
			}
			dstid = rd4(a[1:])
			if drawlookup(client, dstid, 0) != nil {
				goto Eimageexists
			}
			s := string(a[6 : 6+j])
			dn = drawlookupname(client, s)
			if dn == nil {
				goto Enoname
			}
			if drawinstall(client, dstid, dn.dimage.image, nil) == nil {
				goto Edrawmem
			}
			di = drawlookup(client, dstid, 0)
			if di == nil {
				goto Eoldname
			}
			di.vers = dn.vers
			di.name = s
			di.fromname = dn.dimage
			di.fromname.ref++
			client.infoid = dstid
			continue

		/* name an image: 'N' dstid[4] in[1] j[1] name[j] */
		case 'N':
			m = 1 + 4 + 1 + 1
			if len(a) < m {
				goto Eshortdraw
			}
			c = int(a[5])
			j = int(a[6])
			if j == 0 { /* give me a non-empty name please */
				goto Eshortdraw
			}
			m += j
			if len(a) < m {
				goto Eshortdraw
			}
			di = drawlookup(client, rd4(a[1:]), 0)
			if di == nil {
				goto Enodrawimage
			}
			if di.name != "" {
				goto Enamed
			}
			if c != 0 {
				s := string(a[7 : 7+j])
				if err = drawaddname(client, di, s); err != nil {
					goto error
				}
				dn = drawlookupname(client, s)
				if dn == nil {
					goto Enoname
				}
				if dn.dimage != di {
					goto Ewrongname
				}
				drawdelname(client, dn)
			}
			continue

		/* position window: 'o' id[4] r.min [2*4] screenr.min [2*4] */
		case 'o':
			m = 1 + 4 + 2*4 + 2*4
			if len(a) < m {
				goto Eshortdraw
			}
			dst = drawimage(client, a[1:])
			if dst == nil {
				goto Enodrawimage
			}
			if dst.Layer != nil {
				drawpoint(&p, a[5:])
				drawpoint(&q, a[13:])
				r = dst.Layer.Screenr
				ni, err = memdraw.LOrigin(dst, p, q)
				if err != nil {
					goto error
				}
				if ni > 0 {
					addflush(client, r)
					addflush(client, dst.Layer.Screenr)
					ll = drawlookup(client, rd4(a[1:]), 1)
					drawrefreshscreen(ll, client)
				}
			}
			continue

		/* set compositing operator for next draw operation: 'O' op */
		case 'O':
			m = 1 + 1
			if len(a) < m {
				goto Eshortdraw
			}
			client.op = draw.Op(a[1])
			continue

		/* filled polygon: 'P' dstid[4] n[2] wind[4] ignore[2*4] srcid[4] sp[2*4] p0[2*4] dp[2*2*n] */
		/* polygon: 'p' dstid[4] n[2] end0[4] end1[4] radius[4] srcid[4] sp[2*4] p0[2*4] dp[2*2*n] */
		case 'p',
			'P':
			m = 1 + 4 + 2 + 4 + 4 + 4 + 4 + 2*4
			if len(a) < m {
				goto Eshortdraw
			}
			dstid = rd4(a[1:])
			dst = drawimage(client, a[1:])
			ni = int(binary.LittleEndian.Uint16(a[5:]))
			if ni < 0 {
				err = fmt.Errorf("negative cout in polygon")
				goto error
			}
			e0 := draw.End(rd4(a[7:]))
			e1 := draw.End(rd4(a[11:]))
			j = 0
			if a[0] == 'p' {
				j = rd4(a[15:])
				if j < 0 {
					err = fmt.Errorf("negative polygon line width")
					goto error
				}
			}
			src = drawimage(client, a[19:])
			if dst == nil || src == nil {
				goto Enodrawimage
			}
			drawpoint(&sp, a[23:])
			drawpoint(&p, a[31:])
			ni++
			pp = make([]draw.Point, ni)
			doflush = 0
			if dstid == 0 || (dst.Layer != nil && dst.Layer.Screen.Image.Data == client.screenimage.Data) {
				doflush = 1 /* simplify test in loop */
			}
			oy = 0
			ox = oy
			esize = 0
			u := a[m:]
			for y = 0; y < ni; y++ {
				q = p
				oesize = esize
				u = drawcoord(u, ox, &p.X)
				if u == nil {
					goto Eshortdraw
				}
				u = drawcoord(u, oy, &p.Y)
				if u == nil {
					goto Eshortdraw
				}
				ox = p.X
				oy = p.Y
				if doflush != 0 {
					esize = j
					if a[0] == 'p' {
						if y == 0 {
							c = memdraw.LineEndSize(e0)
							if c > esize {
								esize = c
							}
						}
						if y == ni-1 {
							c = memdraw.LineEndSize(e1)
							if c > esize {
								esize = c
							}
						}
					}
					if a[0] == 'P' && e0 != 1 && e0 != ^0 {
						r = dst.Clipr
					} else if y > 0 {
						r = draw.Rect(q.X-oesize, q.Y-oesize, q.X+oesize+1, q.Y+oesize+1)
						draw.CombineRect(&r, draw.Rect(p.X-esize, p.Y-esize, p.X+esize+1, p.Y+esize+1))
					}
					if draw.RectClip(&r, dst.Clipr) { /* should perhaps be an arg to dstflush */
						dstflush(client, dstid, dst, r)
					}
				}
				pp[y] = p
			}
			if y == 1 {
				dstflush(client, dstid, dst, draw.Rect(p.X-esize, p.Y-esize, p.X+esize+1, p.Y+esize+1))
			}
			op := drawclientop(client)
			if a[0] == 'p' {
				memdraw.Poly(dst, pp, e0, e1, j, src, sp, op)
			} else {
				memdraw.FillPoly(dst, pp, int(e0), src, sp, op)
			}
			m = len(a) - len(u)
			continue

		/* read: 'r' id[4] R[4*4] */
		case 'r':
			m = 1 + 4 + 4*4
			if len(a) < m {
				goto Eshortdraw
			}
			i = drawimage(client, a[1:])
			if i == nil {
				goto Enodrawimage
			}
			drawrectangle(&r, a[5:])
			if !draw.RectInRect(r, i.R) {
				goto Ereadoutside
			}
			c = draw.BytesPerLine(r, i.Depth)
			c *= r.Dy()
			client.readdata = make([]byte, c)
			n, e := memdraw.Unload(i, r, client.readdata)
			if e != nil {
				client.readdata = nil
				err = fmt.Errorf("bad readimage call")
				goto error
			}
			client.readdata = client.readdata[:n]
			continue

		/* string: 's' dstid[4] srcid[4] fontid[4] P[2*4] clipr[4*4] sp[2*4] ni[2] ni*(index[2]) */
		/* stringbg: 'x' dstid[4] srcid[4] fontid[4] P[2*4] clipr[4*4] sp[2*4] ni[2] bgid[4] bgpt[2*4] ni*(index[2]) */
		case 's',
			'x':
			m = 1 + 4 + 4 + 4 + 2*4 + 4*4 + 2*4 + 2
			if a[0] == 'x' {
				m += 4 + 2*4
			}
			if len(a) < m {
				goto Eshortdraw
			}

			dst = drawimage(client, a[1:])
			dstid = rd4(a[1:])
			src = drawimage(client, a[5:])
			if dst == nil || src == nil {
				goto Enodrawimage
			}
			font = drawlookup(client, rd4(a[9:]), 1)
			if font == nil {
				goto Enodrawimage
			}
			if len(font.fchar) == 0 {
				goto Enotfont
			}
			drawpoint(&p, a[13:])
			drawrectangle(&r, a[21:])
			drawpoint(&sp, a[37:])
			ni = int(binary.LittleEndian.Uint16(a[45:]))
			u := a[m:]
			m += ni * 2
			if len(a) < m {
				goto Eshortdraw
			}
			clipr = dst.Clipr
			dst.Clipr = r
			op := drawclientop(client)
			if a[0] == 'x' {
				/* paint background */
				l = drawimage(client, a[47:])
				if l == nil {
					goto Enodrawimage
				}
				drawpoint(&q, a[51:])
				r.Min.X = p.X
				r.Min.Y = p.Y - font.ascent
				r.Max.X = p.X
				r.Max.Y = r.Min.Y + font.image.R.Dy()
				u := u // local copy
				j = ni
				for {
					j--
					if j < 0 {
						break
					}
					ci = int(binary.LittleEndian.Uint16(u))
					if ci < 0 || ci >= len(font.fchar) {
						dst.Clipr = clipr
						goto Eindex
					}
					r.Max.X += int(font.fchar[ci].width)
					u = u[2:]
				}
				memdraw.Draw(dst, r, l, q, memdraw.Opaque, draw.ZP, op)
			}
			q = p
			for {
				ni--
				if ni < 0 {
					break
				}
				ci = int(binary.LittleEndian.Uint16(u))
				if ci < 0 || ci >= len(font.fchar) {
					dst.Clipr = clipr
					goto Eindex
				}
				q = drawchar(dst, q, src, &sp, font, ci, op)
				u = u[2:]
			}
			dst.Clipr = clipr
			p.Y -= font.ascent
			dstflush(client, dstid, dst, draw.Rect(p.X, p.Y, q.X, p.Y+font.image.R.Dy()))
			continue

		/* use public screen: 'S' id[4] chan[4] */
		case 'S':
			m = 1 + 4 + 4
			if len(a) < m {
				goto Eshortdraw
			}
			dstid = rd4(a[1:])
			if dstid == 0 {
				goto Ebadarg
			}
			dscrn = drawlookupdscreen(client, dstid)
			if dscrn == nil || (dscrn.public == 0 && dscrn.owner != client) {
				goto Enodrawscreen
			}
			if dscrn.screen.Image.Pix != draw.Pix(binary.LittleEndian.Uint32(a[5:])) {
				err = fmt.Errorf("inconsistent chan")
				goto error
			}
			if drawinstallscreen(client, dscrn, 0, nil, nil, 0) == nil {
				goto Edrawmem
			}
			continue

		/* top or bottom windows: 't' top[1] nw[2] n*id[4] */
		case 't':
			m = 1 + 1 + 2
			if len(a) < m {
				goto Eshortdraw
			}
			nw = int(binary.LittleEndian.Uint16(a[2:]))
			if nw < 0 {
				goto Ebadarg
			}
			if nw == 0 {
				continue
			}
			m += nw * 4
			if len(a) < m {
				goto Eshortdraw
			}
			lp = make([]*memdraw.Image, nw)
			for j = 0; j < nw; j++ {
				lp[j] = drawimage(client, a[1+1+2+j*4:])
				if lp[j] == nil {
					goto Enodrawimage
				}
			}
			if lp[0].Layer == nil {
				err = fmt.Errorf("images are not windows")
				goto error
			}
			for j = 1; j < nw; j++ {
				if lp[j].Layer.Screen != lp[0].Layer.Screen {
					err = fmt.Errorf("images not on same screen")
					goto error
				}
			}
			if a[1] != 0 {
				memdraw.LToFrontN(lp, nw)
			} else {
				memdraw.LToRearN(lp, nw)
			}
			if lp[0].Layer.Screen.Image.Data == client.screenimage.Data {
				for j = 0; j < nw; j++ {
					addflush(client, lp[j].Layer.Screenr)
				}
			}
			ll = drawlookup(client, rd4(a[1+1+2:]), 1)
			drawrefreshscreen(ll, client)
			continue

		/* visible: 'v' */
		case 'v':
			m = 1
			drawflush(client)
			continue

		/* write: 'y' id[4] R[4*4] data[x*1] */
		/* write from compressed data: 'Y' id[4] R[4*4] data[x*1] */
		case 'y',
			'Y':
			m = 1 + 4 + 4*4
			if len(a) < m {
				goto Eshortdraw
			}
			dstid = rd4(a[1:])
			dst = drawimage(client, a[1:])
			if dst == nil {
				goto Enodrawimage
			}
			drawrectangle(&r, a[5:])
			if !draw.RectInRect(r, dst.R) {
				goto Ewriteoutside
			}
			y, err = memdraw.Load(dst, r, a[m:], a[0] == 'Y')
			if err != nil {
				err = fmt.Errorf("bad writeimage call")
				goto error
			}
			dstflush(client, dstid, dst, r)
			m += y
			continue
		}
	}
	rpc_gfxdrawunlock()
	drawlk.Unlock()
	return oldn - len(a), nil

Enodrawimage:
	err = fmt.Errorf("unknown id for draw image")
	goto error
Enodrawscreen:
	err = fmt.Errorf("unknown id for draw screen")
	goto error
Eshortdraw:
	err = fmt.Errorf("short draw message")
	goto error
	/*
	   Eshortread:
	   	err = fmt.Errorf("draw read too short");
	   	goto error;
	*/
Eimageexists:
	err = fmt.Errorf("image id in use")
	goto error
Escreenexists:
	err = fmt.Errorf("screen id in use")
	goto error
Edrawmem:
	err = fmt.Errorf("image memory allocation failed")
	goto error
Ereadoutside:
	err = fmt.Errorf("readimage outside image")
	goto error
Ewriteoutside:
	err = fmt.Errorf("writeimage outside image")
	goto error
Enotfont:
	err = fmt.Errorf("image not a font")
	goto error
Eindex:
	err = fmt.Errorf("character index out of range")
	goto error
	/*
	   Enoclient:
	   	err = fmt.Errorf("no such draw client");
	   	goto error;
	   Edepth:
	   	err = fmt.Errorf("image has bad depth");
	   	goto error;
	   Enameused:
	   	err = fmt.Errorf("image name in use");
	   	goto error;
	*/
Enoname:
	err = fmt.Errorf("no image with that name")
	goto error
Eoldname:
	err = fmt.Errorf("named image no longer valid")
	goto error
Enamed:
	err = fmt.Errorf("image already has name")
	goto error
Ewrongname:
	err = fmt.Errorf("wrong name for image")
	goto error
Ebadarg:
	err = fmt.Errorf("bad argument in draw message")
	goto error

error:
	rpc_gfxdrawunlock()
	drawlk.Unlock()
	return 0, err
}

type eface struct {
	_type unsafe.Pointer
	data  unsafe.Pointer
}

func funcPC(f interface{}) uintptr {
	return *(*uintptr)(efaceOf(&f).data)
}
func efaceOf(ep *interface{}) *eface {
	return (*eface)(unsafe.Pointer(ep))
}
