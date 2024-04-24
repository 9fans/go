//go:build !plan9
// +build !plan9

package client

import (
	"io"
	"os"
	"strings"
	"sync"

	"9fans.net/go/plan9"
)

func getuser() string { return os.Getenv("USER") }

type Fid struct {
	qid  plan9.Qid
	fid  uint32
	mode uint8
	// f guards offset and c.
	f sync.Mutex
	// c holds the underlying connection.
	// It's nil after the Fid has been closed.
	_c     *conn
	offset int64
}

func (fid *Fid) conn() (*conn, error) {
	fid.f.Lock()
	c := fid._c
	fid.f.Unlock()
	if c == nil {
		return nil, errClosed
	}
	return c, nil
}

func (fid *Fid) Close() error {
	if fid == nil {
		// TODO why is Close allowed on a nil fid but no other operations?
		return nil
	}
	conn, err := fid.conn()
	if err != nil {
		return err
	}
	tx := &plan9.Fcall{Type: plan9.Tclunk, Fid: fid.fid}
	_, err = conn.rpc(tx, fid)
	return err
}

// clunked marks the fid as clunked and closes it. This is called
// just before sending a message that will clunk it.
func (fid *Fid) clunked() error {
	fid.f.Lock()
	defer fid.f.Unlock()
	if fid._c == nil {
		return errClosed
	}
	fid._c.putfidnum(fid.fid)
	fid._c.release()
	fid._c = nil
	return nil
}

func (fid *Fid) Create(name string, mode uint8, perm plan9.Perm) error {
	conn, err := fid.conn()
	if err != nil {
		return err
	}
	tx := &plan9.Fcall{Type: plan9.Tcreate, Fid: fid.fid, Name: name, Mode: mode, Perm: perm}
	rx, err := conn.rpc(tx, nil)
	if err != nil {
		return err
	}
	fid.mode = mode
	fid.qid = rx.Qid
	return nil
}

func (fid *Fid) Open(mode uint8) error {
	conn, err := fid.conn()
	if err != nil {
		return err
	}
	tx := &plan9.Fcall{Type: plan9.Topen, Fid: fid.fid, Mode: mode}
	if _, err := conn.rpc(tx, nil); err != nil {
		return err
	}
	fid.mode = mode
	return nil
}

func (fid *Fid) Qid() plan9.Qid {
	return fid.qid
}

func (fid *Fid) Read(b []byte) (n int, err error) {
	return fid.readAt(b, -1)
}

func (fid *Fid) ReadAt(b []byte, offset int64) (n int, err error) {
	for len(b) > 0 {
		m, err := fid.readAt(b, offset)
		if err != nil {
			return n, err
		}
		n += m
		b = b[m:]
		if offset != -1 {
			offset += int64(m)
		}
	}
	return n, nil
}

func (fid *Fid) readAt(b []byte, offset int64) (n int, err error) {
	conn, err := fid.conn()
	if err != nil {
		return 0, err
	}
	msize := conn.msize - plan9.IOHDRSZ
	n = len(b)
	if uint32(n) > msize {
		n = int(msize)
	}
	o := offset
	if o == -1 {
		fid.f.Lock()
		o = fid.offset
		fid.f.Unlock()
	}
	tx := &plan9.Fcall{Type: plan9.Tread, Fid: fid.fid, Offset: uint64(o), Count: uint32(n)}
	rx, err := conn.rpc(tx, nil)
	if err != nil {
		return 0, err
	}
	if len(rx.Data) == 0 {
		return 0, io.EOF
	}
	copy(b, rx.Data)
	if offset == -1 {
		fid.f.Lock()
		fid.offset += int64(len(rx.Data))
		fid.f.Unlock()
	}
	return len(rx.Data), nil
}

func (fid *Fid) Remove() error {
	conn, err := fid.conn()
	if err != nil {
		return err
	}
	tx := &plan9.Fcall{Type: plan9.Tremove, Fid: fid.fid}
	_, err = conn.rpc(tx, fid)
	return err
}

func (fid *Fid) Seek(n int64, whence int) (int64, error) {
	switch whence {
	case 0:
		fid.f.Lock()
		fid.offset = n
		fid.f.Unlock()

	case 1:
		fid.f.Lock()
		n += fid.offset
		if n < 0 {
			fid.f.Unlock()
			return 0, Error("negative offset")
		}
		fid.offset = n
		fid.f.Unlock()

	case 2:
		d, err := fid.Stat()
		if err != nil {
			return 0, err
		}
		n += int64(d.Length)
		if n < 0 {
			return 0, Error("negative offset")
		}
		fid.f.Lock()
		fid.offset = n
		fid.f.Unlock()

	default:
		return 0, Error("bad whence in seek")
	}

	return n, nil
}

func (fid *Fid) Stat() (*plan9.Dir, error) {
	conn, err := fid.conn()
	if err != nil {
		return nil, err
	}
	tx := &plan9.Fcall{Type: plan9.Tstat, Fid: fid.fid}
	rx, err := conn.rpc(tx, nil)
	if err != nil {
		return nil, err
	}
	return plan9.UnmarshalDir(rx.Stat)
}

// TODO(rsc): Could use ...string instead?
func (fid *Fid) Walk(name string) (*Fid, error) {
	conn, err := fid.conn()
	if err != nil {
		return nil, err
	}
	wfidnum, err := conn.newfidnum()
	if err != nil {
		return nil, err
	}

	// Split, delete empty strings and dot.
	elem := strings.Split(name, "/")
	j := 0
	for _, e := range elem {
		if e != "" && e != "." {
			elem[j] = e
			j++
		}
	}
	elem = elem[0:j]

	var wfid *Fid
	fromfidnum := fid.fid
	for nwalk := 0; ; nwalk++ {
		n := len(elem)
		if n > plan9.MAXWELEM {
			n = plan9.MAXWELEM
		}
		tx := &plan9.Fcall{Type: plan9.Twalk, Fid: fromfidnum, Newfid: wfidnum, Wname: elem[0:n]}
		rx, err := conn.rpc(tx, nil)
		if err == nil && len(rx.Wqid) != n {
			err = Error("file '" + name + "' not found")
		}
		if err != nil {
			if wfid != nil {
				wfid.Close()
			}
			return nil, err
		}
		if n == 0 {
			wfid = conn.newFid(wfidnum, fid.qid)
		} else {
			wfid = conn.newFid(wfidnum, rx.Wqid[n-1])
		}
		elem = elem[n:]
		if len(elem) == 0 {
			break
		}
		fromfidnum = wfid.fid
	}
	return wfid, nil
}

func (fid *Fid) Write(b []byte) (n int, err error) {
	return fid.WriteAt(b, -1)
}

func (fid *Fid) WriteAt(b []byte, offset int64) (n int, err error) {
	conn, err := fid.conn()
	if err != nil {
		return 0, err
	}
	msize := conn.msize - plan9.IOHDRSIZE
	tot := 0
	n = len(b)
	first := true
	for tot < n || first {
		want := n - tot
		if uint32(want) > msize {
			want = int(msize)
		}
		got, err := fid.writeAt(b[tot:tot+want], offset)
		tot += got
		if err != nil {
			return tot, err
		}
		if offset != -1 {
			offset += int64(got)
		}
		first = false
	}
	return tot, nil
}

func (fid *Fid) writeAt(b []byte, offset int64) (n int, err error) {
	conn, err := fid.conn()
	if err != nil {
		return 0, err
	}
	o := offset
	if o == -1 {
		fid.f.Lock()
		o = fid.offset
		fid.f.Unlock()
	}
	tx := &plan9.Fcall{Type: plan9.Twrite, Fid: fid.fid, Offset: uint64(o), Data: b}
	rx, err := conn.rpc(tx, nil)
	if err != nil {
		return 0, err
	}
	if offset == -1 && rx.Count > 0 {
		fid.f.Lock()
		fid.offset += int64(rx.Count)
		fid.f.Unlock()
	}
	return int(rx.Count), nil
}

func (fid *Fid) Wstat(d *plan9.Dir) error {
	conn, err := fid.conn()
	if err != nil {
		return err
	}
	b, err := d.Bytes()
	if err != nil {
		return err
	}
	tx := &plan9.Fcall{Type: plan9.Twstat, Fid: fid.fid, Stat: b}
	_, err = conn.rpc(tx, nil)
	return err
}
