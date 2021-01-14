package memdraw

import (
	"fmt"
	"os"

	"9fans.net/go/draw"
)

const (
	_CHUNK  = 16000
	_HSHIFT = 3 /* HSHIFT==5 runs slightly faster, but hash table is 64x bigger */
	_NHASH  = 1 << (_HSHIFT * _NMATCH)
	_HMASK  = _NHASH - 1
)

func hupdate(h uint32, c uint8) uint32 {
	return ((h << _HSHIFT) ^ uint32(c)) & _HMASK
}

type hlist struct {
	s    int // pointer into data
	next *hlist
	prev *hlist
}

func writememimage(fd *os.File, i *Image) error {
	r := i.R
	bpl := draw.BytesPerLine(r, i.Depth)
	n := r.Dy() * bpl
	data := make([]byte, n)
	ncblock := compblocksize(r, i.Depth)
	outbuf := make([]byte, ncblock)
	hash := make([]hlist, _NHASH)
	chain := make([]hlist, _NMEM)
	var dy int
	for miny := r.Min.Y; miny != r.Max.Y; miny += dy {
		dy = r.Max.Y - miny
		if dy*bpl > _CHUNK {
			dy = _CHUNK / bpl
		}
		nb, err := unloadmemimage(i, draw.Rect(r.Min.X, miny, r.Max.X, miny+dy), data[(miny-r.Min.Y)*bpl:])
		if err != nil {
			return err
		}
		if nb != dy*bpl {
			return fmt.Errorf("unloadmemimage phase error")
		}
	}
	hdr := []byte(fmt.Sprintf("compressed\n%11s %11d %11d %11d %11d ",
		i.Pix.String(), r.Min.X, r.Min.Y, r.Max.X, r.Max.Y))
	if _, err := fd.Write(hdr); err != nil {
		return err
	}

	edata := n
	eout := ncblock
	line := 0 // index into data
	r.Max.Y = r.Min.Y
	for line != edata {
		for i := range hash {
			hash[i] = hlist{}
		}
		for i := range chain {
			chain[i] = hlist{}
		}
		cp := 0 // index into chain
		h := uint32(0)
		outp := 0 // index into outbuf
		for n = 0; n != _NMATCH; n++ {
			h = hupdate(h, data[line+n])
		}
		loutp := 0 // index into outbuf
		for line != edata {
			ndump := 0
			eline := line + bpl
			var dumpbuf [_NDUMP]uint8 /* dump accumulator */
			for p := line; p != eline; {
				var es int
				if eline-p < _NRUN {
					es = eline
				} else {
					es = p + _NRUN
				}
				var q int
				runlen := 0
				for hp := hash[h].next; hp != nil; hp = hp.next {
					s := p + runlen
					if s >= es {
						continue
					}
					t := hp.s + runlen
					for ; s >= p; s-- {
						t0 := t
						t--
						if data[s] != data[t0] {
							goto matchloop
						}
					}
					t += runlen + 2
					s += runlen + 2
					for ; s < es; s++ {
						t0 := t
						t++
						if data[s] != data[t0] {
							break
						}
					}
					n = s - p
					if n > runlen {
						runlen = n
						q = hp.s
						if n == _NRUN {
							break
						}
					}
				matchloop:
				}
				if runlen < _NMATCH {
					if ndump == _NDUMP {
						if eout-outp < ndump+1 {
							goto Bfull
						}
						outbuf[outp] = uint8(ndump - 1 + 128)
						outp++
						copy(outbuf[outp:outp+ndump], dumpbuf[:ndump])
						outp += ndump
						ndump = 0
					}
					dumpbuf[ndump] = data[p]
					ndump++
					runlen = 1
				} else {
					if ndump != 0 {
						if eout-outp < ndump+1 {
							goto Bfull
						}
						outbuf[outp] = uint8(ndump - 1 + 128)
						outp++
						copy(outbuf[outp:outp+ndump], dumpbuf[:ndump])
						outp += ndump
						ndump = 0
					}
					offs := p - q - 1
					if eout-outp < 2 {
						goto Bfull
					}
					outbuf[outp] = byte(((runlen - _NMATCH) << 2) + (offs >> 8))
					outp++
					outbuf[outp] = uint8(offs & 255)
					outp++
				}
				for q = p + runlen; p != q; p++ {
					if chain[cp].prev != nil {
						chain[cp].prev.next = nil
					}
					chain[cp].next = hash[h].next
					chain[cp].prev = &hash[h]
					if chain[cp].next != nil {
						chain[cp].next.prev = &chain[cp]
					}
					chain[cp].prev.next = &chain[cp]
					chain[cp].s = p
					cp++
					if cp == _NMEM {
						cp = 0
					}
					if edata-p > _NMATCH {
						h = hupdate(h, data[p+_NMATCH])
					}
				}
			}
			if ndump != 0 {
				if eout-outp < ndump+1 {
					goto Bfull
				}
				outbuf[outp] = uint8(ndump - 1 + 128)
				outp++
				copy(outbuf[outp:outp+ndump], dumpbuf[:ndump])
				outp += ndump
			}
			line = eline
			loutp = outp
			r.Max.Y++
		}
	Bfull:
		if loutp == 0 {
			return fmt.Errorf("no data")
		}
		n = loutp
		hdr := []byte(fmt.Sprintf("%11d %11ld ", r.Max.Y, n))
		fd.Write(hdr)
		fd.Write(outbuf[:n])
		r.Min.Y = r.Max.Y
	}
	return nil
}
