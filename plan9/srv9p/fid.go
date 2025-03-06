package srv9p

import (
	"errors"
	"sync/atomic"

	"9fans.net/go/plan9"
)

// A Fid represents a file reference in a 9P connection.
type Fid struct {
	// immutable fields
	uid   string // uid used in auth, attach
	aname string // aname used in auth
	conn  *conn

	// ref counts references to this fid.
	// When the last reference to a fid is dropped,
	// [Fid.decRef] calls [Srv.Clunk].
	// The fids refMap holds a reference,
	// and each active request referring to fid also holds a reference
	// (in a local variable in the function running the request).
	ref atomic.Int32 // references

	// mutable fields
	// These are all atomic to avoid data races.
	// In normal protocol usage there should only be one request
	// modifying most of these at a time, but a misbehaved client
	// might do something like send parallel Topen requests.
	// Using atomics avoids memory races in that case.
	file      atomic.Pointer[File]      // file in srv.Tree
	iounit    atomic.Uint32             // i/o unit
	qid       atomicValue[plan9.Qid]    // current qid
	aux       atomicValue[any]          // for client use
	omode     atomic.Int32              // open mode (-1 if unopened)
	dirReader atomic.Pointer[dirReader] // directory reader (for file trees)
	dirOffset atomic.Int64              // required offset of next directory read (0 also okay)
	dirIndex  atomic.Int64              // directory read index, for use by [Fid.ReadDir]
}

func (f *Fid) File() *File        { return f.file.Load() }
func (f *Fid) SetFile(file *File) { f.file.Store(file) }

func (f *Fid) Aux() any       { return f.aux.Load() }
func (f *Fid) SetAux(aux any) { f.aux.Store(aux) }

func (f *Fid) Qid() plan9.Qid       { return f.qid.Load() }
func (f *Fid) SetQid(qid plan9.Qid) { f.qid.Store(qid) }

func (f *Fid) Iounit() uint32          { return f.iounit.Load() }
func (f *Fid) SetIounit(iounit uint32) { f.iounit.Store(iounit) }

// newFid allocates a new fid with the given id and returns a new reference.
func (c *conn) newFid(id uint32) (*Fid, error) {
	f := &Fid{conn: c}
	f.omode.Store(-1)
	f.incRef() // for caller
	f.incRef() // for pool
	if !c.fids.tryInsert(id, f) {
		// no decRef+CleanupFid because no one else knows about f
		return nil, errors.New("duplicate fid")
	}
	return f, nil
}

func (f *Fid) incRef() {
	if f != nil {
		f.ref.Add(1)
	}
}

func (f *Fid) decRef() {
	if f != nil && f.ref.Add(-1) == 0 {
		f.dirReader.Load().Close()
		if clunk := f.conn.srv.Clunk; clunk != nil {
			clunk(f)
		}
	}
}

// An atomicValue can be atomically read and written.
type atomicValue[T any] struct {
	ptr atomic.Pointer[T]
}

func (v *atomicValue[T]) Load() T {
	p := v.ptr.Load()
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func (v *atomicValue[T]) Store(t T) {
	v.ptr.Store(&t)
}
