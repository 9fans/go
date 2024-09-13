// Package p9trace implements a parser for the
// [Plan 9 File System Traces].
//
// [Plan 9 File System Traces]: https://github.com/9fans/p9trace
package p9trace

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"io"
	"iter"
	"log"
	"os"
)

const (
	TagNull  = 0
	TagSuper = 1
	TagDir   = 2
	TagInd1  = 3
	TagInd2  = 4
	TagFile  = 5
	TagMax   = 6
)

const (
	HeaderSize = 35
	SuperSize  = 16
	DirSize    = 62
)

type Record struct {
	Tag   uint8    // type of block (Tag* constants)
	Path  uint32   // unique file id
	Addr  uint32   // address of block
	ZSize uint16   // zero-truncated size of data in block
	WSize uint16   // fast lempel-ziv (wide deflate) compressed
	DSize uint16   // deflate compressed
	Score [20]byte // keyed hash of data

	Super *Super
	Dir   []*Dir
	Ind   []uint32
}

type Super struct {
	CWRAddr uint32 // cached-worm root: dir block for root of snapshot
	RORAddr uint32 // read-only root: dir block for snapshot tree
	Last    uint32 // addr of previous super block
	Next    uint32 // addr of next super block
}

type Dir struct {
	Slot    uint16
	Path    uint32
	Version uint32
	Mode    uint16
	Size    uint32
	DBlock  [6]uint32
	IBlock  uint32
	DIBlock uint32
	MTime   uint32
	ATime   uint32
	UID     uint16
	GID     uint16
	WID     uint16
}

var errCorrupt = errors.New("corrupt data")

func (r *Record) Pointers() iter.Seq[*uint32] {
	return func(yield func(*uint32) bool) {
		if s := r.Super; s != nil {
			if !yield(&s.CWRAddr) || !yield(&s.RORAddr) || !yield(&s.Last) || !yield(&s.Next) {
				return
			}
		}
		for _, d := range r.Dir {
			for i := range d.DBlock {
				if !yield(&d.DBlock[i]) {
					return
				}
			}
			if !yield(&d.IBlock) || !yield(&d.DIBlock) {
				return
			}
		}
		for i := range r.Ind {
			if !yield(&r.Ind[i]) {
				return
			}
		}
	}
}

func (r *Record) MarshalBinary() ([]byte, error) {
	var out []byte
	u8 := func(x uint8) { out = append(out, x) }
	u16 := func(x uint16) { out = binary.BigEndian.AppendUint16(out, x) }
	u32 := func(x uint32) { out = binary.BigEndian.AppendUint32(out, x) }

	// Header
	u8(r.Tag)
	u32(r.Path)
	u32(r.Addr)
	u16(r.ZSize)
	u16(r.WSize)
	u16(r.DSize)
	out = append(out, r.Score[:]...)

	switch r.Tag {
	case TagSuper:
		s := r.Super
		u32(s.CWRAddr)
		u32(s.RORAddr)
		u32(s.Last)
		u32(s.Next)

	case TagDir:
		u16(uint16(len(r.Dir)))
		for _, d := range r.Dir {
			u16(d.Slot)
			u32(d.Path)
			u32(d.Version)
			u16(d.Mode)
			u32(d.Size)
			for _, b := range &d.DBlock {
				u32(b)
			}
			u32(d.IBlock)
			u32(d.DIBlock)
			u32(d.MTime)
			u32(d.ATime)
			u16(d.UID)
			u16(d.GID)
			u16(d.WID)
		}

	case TagInd1, TagInd2:
		u16(uint16(len(r.Ind)))
		for _, b := range r.Ind {
			u32(b)
		}
	}

	return out, nil
}

func (r *Record) UnmarshalBinary(data []byte) error {
	if len(data) < HeaderSize {
		return errCorrupt
	}
	*r = Record{
		Tag:   data[0],
		Path:  binary.BigEndian.Uint32(data[1:]),
		Addr:  binary.BigEndian.Uint32(data[5:]),
		ZSize: binary.BigEndian.Uint16(data[9:]),
		WSize: binary.BigEndian.Uint16(data[11:]),
		DSize: binary.BigEndian.Uint16(data[13:]),
		Score: [20]byte(data[15:]),
	}

	data = data[HeaderSize:]
	switch r.Tag {
	case TagSuper:
		if len(data) != SuperSize {
			return errCorrupt
		}
		r.Super = &Super{
			CWRAddr: binary.BigEndian.Uint32(data[0:]),
			RORAddr: binary.BigEndian.Uint32(data[4:]),
			Last:    binary.BigEndian.Uint32(data[8:]),
			Next:    binary.BigEndian.Uint32(data[12:]),
		}
		data = data[SuperSize:]

	case TagDir:
		if len(data) < 2 {
			return errCorrupt
		}
		n := int(binary.BigEndian.Uint16(data))
		data = data[2:]
		if len(data) != n*DirSize {
			return errCorrupt
		}
		r.Dir = make([]*Dir, n)
		for i := range n {
			r.Dir[i] = &Dir{
				Slot:    binary.BigEndian.Uint16(data[0:]),
				Path:    binary.BigEndian.Uint32(data[2:]),
				Version: binary.BigEndian.Uint32(data[6:]),
				Mode:    binary.BigEndian.Uint16(data[10:]),
				Size:    binary.BigEndian.Uint32(data[12:]),
				DBlock: [6]uint32{
					binary.BigEndian.Uint32(data[16:]),
					binary.BigEndian.Uint32(data[20:]),
					binary.BigEndian.Uint32(data[24:]),
					binary.BigEndian.Uint32(data[28:]),
					binary.BigEndian.Uint32(data[32:]),
					binary.BigEndian.Uint32(data[36:]),
				},
				IBlock:  binary.BigEndian.Uint32(data[40:]),
				DIBlock: binary.BigEndian.Uint32(data[44:]),
				MTime:   binary.BigEndian.Uint32(data[48:]),
				ATime:   binary.BigEndian.Uint32(data[52:]),
				UID:     binary.BigEndian.Uint16(data[56:]),
				GID:     binary.BigEndian.Uint16(data[58:]),
				WID:     binary.BigEndian.Uint16(data[60:]),
			}
			data = data[DirSize:]
		}

	case TagInd1, TagInd2:
		if len(data) < 2 {
			return errCorrupt
		}
		n := int(binary.BigEndian.Uint16(data))
		data = data[2:]
		if len(data) != n*4 {
			return errCorrupt
		}
		r.Ind = make([]uint32, n)
		for i := range n {
			r.Ind[i] = binary.BigEndian.Uint32(data[4*i:])
		}
		data = nil
	}

	if len(data) != 0 {
		return errCorrupt
	}
	return nil
}

func FileRecords(files ...string) iter.Seq[*Record] {
	return func(yield func(*Record) bool) {
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				log.Fatal(err)
			}
			for len(data) > 0 {
				n := binary.BigEndian.Uint16(data)
				z := n&0x8000 != 0
				n &^= 0x8000
				raw := data[2 : 2+n]
				data = data[2+n:]
				if z {
					rc := flate.NewReader(bytes.NewReader(raw))
					raw2, err := io.ReadAll(rc)
					rc.Close()
					if err != nil {
						log.Fatal(err)
					}
					raw = raw2
				}
				var r Record
				if err := r.UnmarshalBinary(raw); err != nil {
					log.Fatal(err)
				}
				enc, err := r.MarshalBinary()
				if err != nil {
					log.Fatal(err)
				}
				if !bytes.Equal(raw, enc) {
					log.Fatalf("roundtrip")
				}
				if !yield(&r) {
					return
				}
			}
		}
	}
}
