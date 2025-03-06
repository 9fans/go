package plan9

import (
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	IOHDRSIZE = 24
)

type Fcall struct {
	Type    uint8
	Fid     uint32
	Tag     uint16
	Msize   uint32
	Version string   // Tversion, Rversion
	Oldtag  uint16   // Tflush
	Ename   string   // Rerror
	Qid     Qid      // Rattach, Ropen, Rcreate
	Iounit  uint32   // Ropen, Rcreate
	Aqid    Qid      // Rauth
	Afid    uint32   // Tauth, Tattach
	Uname   string   // Tauth, Tattach
	Aname   string   // Tauth, Tattach
	Perm    Perm     // Tcreate
	Name    string   // Tcreate
	Mode    uint8    // Tcreate, Topen
	Newfid  uint32   // Twalk
	Wname   []string // Twalk
	Wqid    []Qid    // Rwalk
	Offset  uint64   // Tread, Twrite
	Count   uint32   // Tread, Rwrite
	Data    []byte   // Twrite, Rread
	Stat    []byte   // Twstat, Rstat

	// 9P2000.u extensions
	Errno     uint32 // Rerror
	Uid       uint32 // Tattach, Tauth
	Extension string // Tcreate
}

const (
	Tversion = 100 + iota
	Rversion
	Tauth
	Rauth
	Tattach
	Rattach
	Terror // illegal
	Rerror
	Tflush
	Rflush
	Twalk
	Rwalk
	Topen
	Ropen
	Tcreate
	Rcreate
	Tread
	Rread
	Twrite
	Rwrite
	Tclunk
	Rclunk
	Tremove
	Rremove
	Tstat
	Rstat
	Twstat
	Rwstat
	Tmax
)

func (f *Fcall) Bytes() ([]byte, error) {
	b := pbit32(nil, 0) // length: fill in later
	b = pbit8(b, f.Type)
	b = pbit16(b, f.Tag)
	switch f.Type {
	default:
		return nil, ProtocolError("invalid type")

	case Tversion:
		b = pbit32(b, f.Msize)
		b = pstring(b, f.Version)

	case Tflush:
		b = pbit16(b, f.Oldtag)

	case Tauth:
		b = pbit32(b, f.Afid)
		b = pstring(b, f.Uname)
		b = pstring(b, f.Aname)

	case Tattach:
		b = pbit32(b, f.Fid)
		b = pbit32(b, f.Afid)
		b = pstring(b, f.Uname)
		b = pstring(b, f.Aname)

	case Twalk:
		b = pbit32(b, f.Fid)
		b = pbit32(b, f.Newfid)
		if len(f.Wname) > MAXWELEM {
			return nil, ProtocolError("too many names in walk")
		}
		b = pbit16(b, uint16(len(f.Wname)))
		for i := range f.Wname {
			b = pstring(b, f.Wname[i])
		}

	case Topen:
		b = pbit32(b, f.Fid)
		b = pbit8(b, f.Mode)

	case Tcreate:
		b = pbit32(b, f.Fid)
		b = pstring(b, f.Name)
		b = pperm(b, f.Perm)
		b = pbit8(b, f.Mode)

	case Tread:
		b = pbit32(b, f.Fid)
		b = pbit64(b, f.Offset)
		b = pbit32(b, f.Count)

	case Twrite:
		b = pbit32(b, f.Fid)
		b = pbit64(b, f.Offset)
		b = pbit32(b, uint32(len(f.Data)))
		b = append(b, f.Data...)

	case Tclunk, Tremove, Tstat:
		b = pbit32(b, f.Fid)

	case Twstat:
		b = pbit32(b, f.Fid)
		b = pbit16(b, uint16(len(f.Stat)))
		b = append(b, f.Stat...)

	case Rversion:
		b = pbit32(b, f.Msize)
		b = pstring(b, f.Version)

	case Rerror:
		b = pstring(b, f.Ename)

	case Rflush, Rclunk, Rremove, Rwstat:
		// nothing

	case Rauth:
		b = pqid(b, f.Aqid)

	case Rattach:
		b = pqid(b, f.Qid)

	case Rwalk:
		if len(f.Wqid) > MAXWELEM {
			return nil, ProtocolError("too many qid in walk")
		}
		b = pbit16(b, uint16(len(f.Wqid)))
		for i := range f.Wqid {
			b = pqid(b, f.Wqid[i])
		}

	case Ropen, Rcreate:
		b = pqid(b, f.Qid)
		b = pbit32(b, f.Iounit)

	case Rread:
		b = pbit32(b, uint32(len(f.Data)))
		b = append(b, f.Data...)

	case Rwrite:
		b = pbit32(b, f.Count)

	case Rstat:
		b = pbit16(b, uint16(len(f.Stat)))
		b = append(b, f.Stat...)
	}

	pbit32(b[0:0], uint32(len(b)))
	return b, nil
}

func UnmarshalFcall(b []byte) (f *Fcall, err error) {
	defer func() {
		if recover() != nil {
			println("bad fcall at ", b)
			f = nil
			err = ProtocolError("malformed Fcall")
		}
	}()

	n, b := gbit32(b)
	if len(b) != int(n)-4 {
		panic(1)
	}

	f = new(Fcall)
	f.Type, b = gbit8(b)
	f.Tag, b = gbit16(b)

	switch f.Type {
	default:
		panic(1)

	case Tversion:
		f.Msize, b = gbit32(b)
		f.Version, b = gstring(b)

	case Tflush:
		f.Oldtag, b = gbit16(b)

	case Tauth:
		f.Afid, b = gbit32(b)
		f.Uname, b = gstring(b)
		f.Aname, b = gstring(b)

	case Tattach:
		f.Fid, b = gbit32(b)
		f.Afid, b = gbit32(b)
		f.Uname, b = gstring(b)
		f.Aname, b = gstring(b)

	case Twalk:
		f.Fid, b = gbit32(b)
		f.Newfid, b = gbit32(b)
		var n uint16
		n, b = gbit16(b)
		if n > MAXWELEM {
			panic(1)
		}
		f.Wname = make([]string, n)
		for i := range f.Wname {
			f.Wname[i], b = gstring(b)
		}

	case Topen:
		f.Fid, b = gbit32(b)
		f.Mode, b = gbit8(b)

	case Tcreate:
		f.Fid, b = gbit32(b)
		f.Name, b = gstring(b)
		f.Perm, b = gperm(b)
		f.Mode, b = gbit8(b)

	case Tread:
		f.Fid, b = gbit32(b)
		f.Offset, b = gbit64(b)
		f.Count, b = gbit32(b)

	case Twrite:
		f.Fid, b = gbit32(b)
		f.Offset, b = gbit64(b)
		n, b = gbit32(b)
		if len(b) != int(n) {
			panic(1)
		}
		f.Data = b
		b = nil

	case Tclunk, Tremove, Tstat:
		f.Fid, b = gbit32(b)

	case Twstat:
		f.Fid, b = gbit32(b)
		var n uint16
		n, b = gbit16(b)
		if len(b) != int(n) {
			panic(1)
		}
		f.Stat = b
		b = nil

	case Rversion:
		f.Msize, b = gbit32(b)
		f.Version, b = gstring(b)

	case Rerror:
		f.Ename, b = gstring(b)

	case Rflush, Rclunk, Rremove, Rwstat:
		// nothing

	case Rauth:
		f.Aqid, b = gqid(b)

	case Rattach:
		f.Qid, b = gqid(b)

	case Rwalk:
		var n uint16
		n, b = gbit16(b)
		if n > MAXWELEM {
			panic(1)
		}
		if n > 0 {
			f.Wqid = make([]Qid, n)
		}
		for i := range f.Wqid {
			f.Wqid[i], b = gqid(b)
		}

	case Ropen, Rcreate:
		f.Qid, b = gqid(b)
		f.Iounit, b = gbit32(b)

	case Rread:
		n, b = gbit32(b)
		if len(b) != int(n) {
			panic(1)
		}
		f.Data = b
		b = nil

	case Rwrite:
		f.Count, b = gbit32(b)

	case Rstat:
		var n uint16
		n, b = gbit16(b)
		if len(b) != int(n) {
			panic(1)
		}
		f.Stat = b
		b = nil
	}

	if len(b) != 0 {
		panic(1)
	}

	return f, nil
}

func (f *Fcall) String() string {
	if f == nil {
		return "<nil>"
	}
	switch f.Type {
	case Tversion:
		return fmt.Sprintf("Tversion tag %d msize %d version '%s'",
			f.Tag, f.Msize, f.Version)
	case Rversion:
		return fmt.Sprintf("Rversion tag %d msize %d version '%s'",
			f.Tag, f.Msize, f.Version)
	case Tauth:
		return fmt.Sprintf("Tauth tag %d afid %d uname %s aname %s",
			f.Tag, f.Afid, f.Uname, f.Aname)
	case Rauth:
		return fmt.Sprintf("Rauth tag %d qid %v", f.Tag, f.Qid)
	case Tattach:
		return fmt.Sprintf("Tattach tag %d fid %d afid %d uname %s aname %s",
			f.Tag, f.Fid, f.Afid, f.Uname, f.Aname)
	case Rattach:
		return fmt.Sprintf("Rattach tag %d qid %v", f.Tag, f.Qid)
	case Rerror:
		return fmt.Sprintf("Rerror tag %d ename %s", f.Tag, f.Ename)
	case Tflush:
		return fmt.Sprintf("Tflush tag %d oldtag %d", f.Tag, f.Oldtag)
	case Rflush:
		return fmt.Sprintf("Rflush tag %d", f.Tag)
	case Twalk:
		return fmt.Sprintf("Twalk tag %d fid %d newfid %d wname %v",
			f.Tag, f.Fid, f.Newfid, f.Wname)
	case Rwalk:
		return fmt.Sprintf("Rwalk tag %d wqid %v", f.Tag, f.Wqid)
	case Topen:
		return fmt.Sprintf("Topen tag %d fid %d mode %d", f.Tag, f.Fid, f.Mode)
	case Ropen:
		return fmt.Sprintf("Ropen tag %d qid %v iounit %d", f.Tag, f.Qid, f.Iounit)
	case Tcreate:
		return fmt.Sprintf("Tcreate tag %d fid %d name %s perm %v mode %d",
			f.Tag, f.Fid, f.Name, f.Perm, f.Mode)
	case Rcreate:
		return fmt.Sprintf("Rcreate tag %d qid %v iounit %d", f.Tag, f.Qid, f.Iounit)
	case Tread:
		return fmt.Sprintf("Tread tag %d fid %d offset %d count %d",
			f.Tag, f.Fid, f.Offset, f.Count)
	case Rread:
		return fmt.Sprintf("Rread tag %d count %d %s",
			f.Tag, len(f.Data), dumpsome(f.Data))
	case Twrite:
		return fmt.Sprintf("Twrite tag %d fid %d offset %d count %d %s",
			f.Tag, f.Fid, f.Offset, len(f.Data), dumpsome(f.Data))
	case Rwrite:
		return fmt.Sprintf("Rwrite tag %d count %d", f.Tag, f.Count)
	case Tclunk:
		return fmt.Sprintf("Tclunk tag %d fid %d", f.Tag, f.Fid)
	case Rclunk:
		return fmt.Sprintf("Rclunk tag %d", f.Tag)
	case Tremove:
		return fmt.Sprintf("Tremove tag %d fid %d", f.Tag, f.Fid)
	case Rremove:
		return fmt.Sprintf("Rremove tag %d", f.Tag)
	case Tstat:
		return fmt.Sprintf("Tstat tag %d fid %d", f.Tag, f.Fid)
	case Rstat:
		d, err := UnmarshalDir(f.Stat)
		if err != nil {
			return fmt.Sprintf("Rstat tag %d stat (%d bytes; %v)",
				f.Tag, len(f.Stat), err)
		}
		return fmt.Sprintf("Rstat tag %d stat %v", f.Tag, d)
	case Twstat:
		d, err := UnmarshalDir(f.Stat)
		if err == nil {
			return fmt.Sprintf("Twstat tag %d fid %d stat(%d bytes)",
				f.Tag, f.Fid, len(f.Stat))
		}
		return fmt.Sprintf("Twstat tag %d fid %d stat %v", f.Tag, f.Fid, d)
	case Rwstat:
		return fmt.Sprintf("Rwstat tag %d", f.Tag)
	}
	return fmt.Sprintf("unknown type %d", f.Type)
}

func ReadFcall(r io.Reader) (*Fcall, error) {
	// 128 bytes should be enough for most messages
	buf := make([]byte, 128)
	_, err := io.ReadFull(r, buf[0:4])
	if err != nil {
		return nil, err
	}

	// read 4-byte header, make room for remainder
	n, _ := gbit32(buf)
	if n < 4 {
		return nil, ProtocolError("invalid length")
	}
	if int(n) <= len(buf) {
		buf = buf[0:n]
	} else {
		buf = make([]byte, n)
		pbit32(buf[0:0], n)
	}

	// read remainder and unpack
	_, err = io.ReadFull(r, buf[4:])
	if err != nil {
		return nil, err
	}
	return UnmarshalFcall(buf)
}

func WriteFcall(w io.Writer, f *Fcall) error {
	b, err := f.Bytes()
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

var types = map[string]uint8{
	"Tversion": Tversion,
	"Rversion": Rversion,
	"Tauth":    Tauth,
	"Rauth":    Rauth,
	"Tattach":  Tattach,
	"Rattach":  Rattach,
	"Rerror":   Rerror,
	"Tflush":   Tflush,
	"Rflush":   Rflush,
	"Twalk":    Twalk,
	"Rwalk":    Rwalk,
	"Topen":    Topen,
	"Ropen":    Ropen,
	"Tcreate":  Tcreate,
	"Rcreate":  Rcreate,
	"Tread":    Tread,
	"Rread":    Rread,
	"Twrite":   Twrite,
	"Rwrite":   Rwrite,
	"Tclunk":   Tclunk,
	"Rclunk":   Rclunk,
	"Tremove":  Tremove,
	"Rremove":  Rremove,
	"Tstat":    Tstat,
	"Rstat":    Rstat,
	"Twstat":   Twstat,
	"Rwstat":   Rwstat,
}

var modes = map[string]uint8{
	"OREAD":   OREAD,
	"OWRITE":  OWRITE,
	"ORDWR":   ORDWR,
	"OEXEC":   OEXEC,
	"OTRUNC":  OTRUNC,
	"OCEXEC":  OCEXEC,
	"ORCLOSE": ORCLOSE,
}

func ParseFcall(s string) (*Fcall, error) {
	f := new(Fcall)
	s = strings.TrimSpace(s)
	typ, s, _ := strings.Cut(s, " ")
	var ok bool
	if f.Type, ok = types[typ]; !ok {
		return nil, fmt.Errorf("unknown type %q", typ)
	}

	for {
		s = strings.TrimSpace(s)
		if s == "" {
			break
		}
		var name string
		name, s, _ = strings.Cut(s, " ")
		s = strings.TrimSpace(s)
		var arg string
		if strings.HasPrefix(s, "(") {
			i := strings.Index(s, ")")
			if i < 0 {
				return nil, fmt.Errorf("missing closing paren")
			}
			arg, s = s[1:i], s[i+1:]
		} else if strings.HasPrefix(s, "[") {
			i := strings.Index(s, "]")
			if i < 0 {
				return nil, fmt.Errorf("missing closing paren")
			}
			arg, s = s[:i+1], s[i+1:]
		} else if strings.HasPrefix(s, "'") {
			i := strings.Index(s[1:], "'")
			if i < 0 {
				return nil, fmt.Errorf("missing closing quote")
			}
			arg, s = s[1:1+i], s[1+i+1:]
		} else {
			arg, s, _ = strings.Cut(s, " ")
		}
		switch name {
		default:
			return nil, fmt.Errorf("unknown fcall field %q", name)
		case "afid":
			if arg == "NOFID" {
				f.Afid = NOFID
				break
			}
			n, err := strconv.ParseUint(arg, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid afid: %v", err)
			}
			f.Afid = uint32(n)
		case "aname":
			f.Aname = arg
		case "aqid":
			q, err := parseQid(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid qid: %v", err)
			}
			f.Aqid = q
		case "count":
			n, err := strconv.ParseUint(arg, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid count: %v", err)
			}
			f.Count = uint32(n)
		case "data":
			f.Data = parseData(arg)
		case "ename":
			f.Ename = arg
		case "fid":
			n, err := strconv.ParseUint(arg, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid fid: %v", err)
			}
			f.Fid = uint32(n)
		case "iounit":
			n, err := strconv.ParseUint(arg, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid iounit: %v", err)
			}
			f.Iounit = uint32(n)
		case "mode":
			if strings.HasPrefix(arg, "O") {
				f.Mode = 0
				for _, name := range strings.Split(arg, "|") {
					if m, ok := modes[name]; ok {
						f.Mode |= m
					} else {
						return nil, fmt.Errorf("invalid mode: unknown %s", name)
					}
				}
			} else {
				n, err := strconv.ParseUint(arg, 0, 8)
				if err != nil {
					return nil, fmt.Errorf("invalid mode: %v", err)
				}
				f.Mode = uint8(n)
			}
		case "msize":
			n, err := strconv.ParseUint(arg, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid msize: %v", err)
			}
			f.Msize = uint32(n)
		case "name":
			f.Name = arg
		case "newfid":
			n, err := strconv.ParseUint(arg, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid newfid: %v", err)
			}
			f.Newfid = uint32(n)
		case "offset":
			n, err := strconv.ParseUint(arg, 0, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid offset: %v", err)
			}
			f.Offset = n
		case "oldtag":
			n, err := strconv.ParseUint(arg, 0, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid oldtag: %v", err)
			}
			f.Oldtag = uint16(n)
		case "perm":
			p, err := parsePerm(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid perm: %v", err)
			}
			f.Perm = p
		case "qid":
			q, err := parseQid(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid qid: %v", err)
			}
			f.Qid = q
		case "stat":
			d, err := parseDir(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid stat: %v", err)
			}
			b, err := d.Bytes()
			if err != nil {
				return nil, fmt.Errorf("invalid stat: %v", err)
			}
			f.Stat = b
		case "tag":
			n, err := strconv.ParseUint(arg, 0, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid tag: %v", err)
			}
			f.Tag = uint16(n)
		case "uname":
			f.Uname = arg
		case "version":
			f.Version = arg
		case "wname":
			if strings.HasPrefix(arg, "[") {
				f.Wname = strings.Fields(arg[1 : len(arg)-1])
			} else {
				f.Wname = []string{arg}
			}
		case "wqid":
			if strings.HasPrefix(arg, "[") {
				for _, qs := range strings.Fields(arg[1 : len(arg)-1]) {
					q, err := parseQid(qs)
					if err != nil {
						return nil, fmt.Errorf("invalid wqid: %v", err)
					}
					f.Wqid = append(f.Wqid, q)
				}
			} else {
				q, err := parseQid(arg)
				if err != nil {
					return nil, fmt.Errorf("invalid wqid: %v", err)
				}
				f.Wqid = []Qid{q}
			}
		}
	}
	return f, nil
}

func parseData(s string) []byte {
	// Try parsing as directories
	if strings.HasPrefix(s, "[(") && strings.HasSuffix(s, ")]") {
		var data []byte
		for _, ds := range strings.Split(s[2:len(s)-2], ") (") {
			d, err := parseDir(ds)
			if err != nil {
				goto NotDir
			}
			b, err := d.Bytes()
			if err != nil {
				goto NotDir
			}
			data = append(data, b...)
		}
		return data
	}
NotDir:

	// Try parsing as hex.
	if data, err := hex.DecodeString(s); err == nil {
		return data
	}

	// Fall back to literal string.
	return []byte(s)
}
