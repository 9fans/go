package plan9

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type ProtocolError string

func (e ProtocolError) Error() string {
	return string(e)
}

const (
	STATMAX = 65535
)

type Dir struct {
	Type   uint16
	Dev    uint32
	Qid    Qid
	Mode   Perm
	Atime  uint32
	Mtime  uint32
	Length uint64
	Name   string
	Uid    string
	Gid    string
	Muid   string
}

var nullDir = Dir{
	^uint16(0),
	^uint32(0),
	Qid{^uint64(0), ^uint32(0), ^uint8(0)},
	^Perm(0),
	^uint32(0),
	^uint32(0),
	^uint64(0),
	"",
	"",
	"",
	"",
}

func (d *Dir) IsNull() bool {
	return *d == nullDir
}

func (d *Dir) Null() {
	*d = nullDir
}

func pdir(b []byte, d *Dir) []byte {
	n := len(b)
	b = pbit16(b, 0) // length, filled in later
	b = pbit16(b, d.Type)
	b = pbit32(b, d.Dev)
	b = pqid(b, d.Qid)
	b = pperm(b, d.Mode)
	b = pbit32(b, d.Atime)
	b = pbit32(b, d.Mtime)
	b = pbit64(b, d.Length)
	b = pstring(b, d.Name)
	b = pstring(b, d.Uid)
	b = pstring(b, d.Gid)
	b = pstring(b, d.Muid)
	pbit16(b[0:n], uint16(len(b)-(n+2)))
	return b
}

func (d *Dir) Bytes() ([]byte, error) {
	return pdir(nil, d), nil
}

func UnmarshalDir(b []byte) (d *Dir, err error) {
	defer func() {
		if v := recover(); v != nil {
			d = nil
			err = ProtocolError("malformed Dir")
		}
	}()

	n, b := gbit16(b)
	if int(n) != len(b) {
		panic(1)
	}

	d = new(Dir)
	d.Type, b = gbit16(b)
	d.Dev, b = gbit32(b)
	d.Qid, b = gqid(b)
	d.Mode, b = gperm(b)
	d.Atime, b = gbit32(b)
	d.Mtime, b = gbit32(b)
	d.Length, b = gbit64(b)
	d.Name, b = gstring(b)
	d.Uid, b = gstring(b)
	d.Gid, b = gstring(b)
	d.Muid, b = gstring(b)

	if len(b) != 0 {
		panic(1)
	}
	return d, nil
}

func (d *Dir) String() string {
	return fmt.Sprintf("name '%s' uid '%s' gid '%s' muid '%s' qid %v mode %v atime %d mtime %d length %d type %d dev %d",
		d.Name, d.Uid, d.Gid, d.Muid, d.Qid, d.Mode,
		d.Atime, d.Mtime, d.Length, d.Type, d.Dev)
}

func parseDir(s string) (*Dir, error) {
	d := new(Dir)
	d.Null()
	for s != "" {
		s = strings.TrimSpace(s)
		if s == "" {
			break
		}
		var name string
		name, s, _ = strings.Cut(s, " ")
		s = strings.TrimSpace(s)
		var arg string
		if strings.HasPrefix(s, "'") {
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
			return nil, fmt.Errorf("unknown field %q", name)
		case "type":
			n, err := strconv.ParseUint(arg, 0, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid type: %v", err)
			}
			d.Type = uint16(n)
		case "dev":
			n, err := strconv.ParseUint(arg, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid dev: %v", err)
			}
			d.Dev = uint32(n)
		case "qid":
			q, err := parseQid(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid qid: %v", err)
			}
			d.Qid = q
		case "mode":
			m, err := parsePerm(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid mode: %v", err)
			}
			d.Mode = m
		case "atime":
			n, err := strconv.ParseUint(arg, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid atime: %v", err)
			}
			d.Atime = uint32(n)
		case "mtime":
			n, err := strconv.ParseUint(arg, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid mtime: %v", err)
			}
			d.Mtime = uint32(n)
		case "length":
			n, err := strconv.ParseUint(arg, 0, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid length: %v", err)
			}
			d.Length = n
		case "name":
			d.Name = arg
		case "uid":
			d.Uid = arg
		case "gid":
			d.Gid = arg
		case "muid":
			d.Muid = arg
		}
	}
	return d, nil
}

func dumpsome(b []byte) string {
	// Is this all directories?
	if s, ok := dumpDirs(b); ok {
		return s
	}

	if len(b) > 64 {
		b = b[0:64]
	}

	printable := true
	for _, c := range b {
		if (c != 0 && c != '\n' && c != '\t' && c < ' ') || c > 127 {
			printable = false
			break
		}
	}

	if printable {
		return strconv.Quote(string(b))
	}
	return fmt.Sprintf("%x", b)
}

func dumpDirs(b []byte) (string, bool) {
	var dirs []*Dir
	for len(b) > 0 {
		if len(b) < 2 {
			return "", false
		}
		n, _ := gbit16(b)
		m := int(n) + 2
		if len(b) < m {
			return "", false
		}
		d, err := UnmarshalDir(b[:m])
		if err != nil {
			return "", false
		}
		dirs = append(dirs, d)
		b = b[m:]
	}
	var out bytes.Buffer
	out.WriteString("[")
	for i, d := range dirs {
		if i > 0 {
			out.WriteString(" ")
		}
		out.WriteString("(")
		out.WriteString(d.String())
		out.WriteString(")")
	}
	out.WriteString("]")
	return out.String(), true
}

type Perm uint32

type permChar struct {
	bit Perm
	c   rune
}

var permChars = []permChar{
	permChar{DMDIR, 'd'},
	permChar{DMAPPEND, 'a'},
	permChar{DMAUTH, 'A'},
	permChar{DMDEVICE, 'D'},
	permChar{DMSOCKET, 'S'},
	permChar{DMNAMEDPIPE, 'P'},
	permChar{0, '-'},
	permChar{DMEXCL, 'l'},
	permChar{DMSYMLINK, 'L'},
	permChar{0, '-'},
	permChar{0400, 'r'},
	permChar{0, '-'},
	permChar{0200, 'w'},
	permChar{0, '-'},
	permChar{0100, 'x'},
	permChar{0, '-'},
	permChar{0040, 'r'},
	permChar{0, '-'},
	permChar{0020, 'w'},
	permChar{0, '-'},
	permChar{0010, 'x'},
	permChar{0, '-'},
	permChar{0004, 'r'},
	permChar{0, '-'},
	permChar{0002, 'w'},
	permChar{0, '-'},
	permChar{0001, 'x'},
	permChar{0, '-'},
}

func parsePerm(s string) (Perm, error) {
	orig := s
	did := false
	var p Perm
	for _, pc := range permChars {
		if pc.bit == 0 && did {
			did = false
			continue
		}
		if s == "" {
			return Perm(0), fmt.Errorf("perm too short: %q", orig)
		}
		if s[0] == byte(pc.c) {
			s = s[1:]
			p |= pc.bit
			if pc.bit != 0 {
				did = true
			}
		}
	}
	if s != "" {
		return Perm(0), fmt.Errorf("perm too long: %q", orig)
	}
	return p, nil
}

func (p Perm) String() string {
	s := ""
	did := false
	for _, pc := range permChars {
		if p&pc.bit != 0 {
			did = true
			s += string(pc.c)
		}
		if pc.bit == 0 {
			if !did {
				s += string(pc.c)
			}
			did = false
		}
	}
	return s
}

func gperm(b []byte) (Perm, []byte) {
	p, b := gbit32(b)
	return Perm(p), b
}

func pperm(b []byte, p Perm) []byte {
	return pbit32(b, uint32(p))
}

type Qid struct {
	Path uint64
	Vers uint32
	Type uint8
}

func (q Qid) String() string {
	t := ""
	if q.Type&QTDIR != 0 {
		t += "d"
	}
	if q.Type&QTAPPEND != 0 {
		t += "a"
	}
	if q.Type&QTEXCL != 0 {
		t += "l"
	}
	if q.Type&QTAUTH != 0 {
		t += "A"
	}
	if t != "" {
		t = "." + t
	}
	return fmt.Sprintf("%#x.%d%s", q.Path, q.Vers, t)
}

func parseQid(s string) (Qid, error) {
	orig := s
	var q Qid
	var ok bool
	ps, vs, _ := strings.Cut(s, ".")
	vs, ts, _ := strings.Cut(vs, ".")
	pn, err1 := strconv.ParseUint(ps, 0, 64)
	vn, err2 := strconv.ParseUint(vs, 0, 32)
	if ts, ok = strings.CutPrefix(ts, "d"); ok {
		q.Type |= QTDIR
	}
	if ts, ok = strings.CutPrefix(ts, "a"); ok {
		q.Type |= QTAPPEND
	}
	if ts, ok = strings.CutPrefix(ts, "l"); ok {
		q.Type |= QTEXCL
	}
	if ts, ok = strings.CutPrefix(ts, "A"); ok {
		q.Type |= QTAUTH
	}
	if err1 != nil || err2 != nil || ts != "" {
		return Qid{}, fmt.Errorf("invalid qid %q", orig)
	}
	q.Path = pn
	q.Vers = uint32(vn)

	return q, nil
}

func gqid(b []byte) (Qid, []byte) {
	var q Qid
	q.Type, b = gbit8(b)
	q.Vers, b = gbit32(b)
	q.Path, b = gbit64(b)
	return q, b
}

func pqid(b []byte, q Qid) []byte {
	b = pbit8(b, q.Type)
	b = pbit32(b, q.Vers)
	b = pbit64(b, q.Path)
	return b
}
