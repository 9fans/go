// #include <u.h>
// #include <libc.h>
// #include <auth.h>
// #include <fcall.h>
// #include <thread.h>
// #include <9p.h>

package srv9p

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"9fans.net/go/plan9"
)

// A Server is a 9P server handling a single 9P conversation
// (typically a connection in /srv mounted by the kernel,
// which multiplexes all clients onto the single connection).
//
// Clients are expected to allocate a Server and initialize the
// exported func fields appropriately. See the comments on
// each field for details.
//
// The callback funcs that take a context correspond to handling
// a specific 9P request. If the context is cancelled, the request has
// been flushed, and the handler should return as soon as possible
// so that the flush response can be sent.
type Server struct {
	// For client use.
	Aux any

	// Tree is an optional file tree served by the server.
	// If a tree is provided, many of the callback functions become optional,
	// as described in the doc comments for the individual functions.
	Tree *Tree

	// Auth handles a Tauth request, initializing afid as a new authentication fid
	// for use authenticating as user to attach with the attach name aname.
	// On success, Auth returns the qid of the authentication fid,
	// which must have Type set to plan9.QTAUTH.
	// If no authentication is required, Auth should return an error.
	//
	// Auth is optional.
	// If Auth is nil, the server responds with an error of the form
	// “prog: authentication not required” where prog is
	// the base name of os.Args[0].
	Auth func(ctx context.Context, afid *Fid, user, aname string) (plan9.Qid, error)

	// Attach handles a Tattach request, initializing fid to represent the file system root
	// and using afid (if non-nil) as an authentication credential.
	// If afid is non-nil, the server has already checked that user and aname
	// match the call to Auth that initialized it.
	// On success, Attach returns the qid of the file system root.
	//
	// If Tree is non-nil, then Attach is optional.
	// When omitted, the qid from the file tree root is used.
	Attach func(ctx context.Context, fid, afid *Fid, user, aname string) (plan9.Qid, error)

	// Walk handles a Twalk request, initializing newfid to represent
	// the same file or directory as fid (except when newfid == fid),
	// and then walking newfid along the successive path elements listed in names.
	// On success, Walk returns a slice of qids corresponding to each path element.
	// If the qid slice has fewer elements than the names slice,
	// that partial result means the next name was not found.
	// Walk should only return an error with an empty qid slice,
	// indicating that the first name in the list was not found
	// or some other error occured (like permission denied).
	//
	// If len(names) > 0, the server has checked that fid is a directory.
	//
	// The top-level [Walk] function helps implement Walk in terms of
	// simpler operations to clone the fid and step one name at a time.
	//
	// If Tree is non-nil, the server handles walks using the file tree,
	// and Walk is ignored. Otherwise Walk is required.
	Walk func(ctx context.Context, fid, newfid *Fid, names []string) ([]plan9.Qid, error)

	// Open handles a Topen request, opening fid according to mode
	// (plan9.OREAD, plan9.ORDWR, and so on).
	// If the fid's qid has changed, Open should call fid.SetQid.
	// If the fid needs to report a non-zero I/O unit, Open should call fid.SetIOUnit.
	//
	// The server has checked that the fid is not already open.
	// If the fid is a directory, the server has checked that the open mode is plan9.OREAD.
	//
	// If Tree is non-nil, the server has already checked that the
	// file metadata corresponding to fid permits the operation.
	// (If not, the server sends back an error without calling Open.)
	Open func(ctx context.Context, fid *Fid, mode uint8) error

	// Create handles a Tcreate request, which updates fid to refer to a
	// new file or directory created in the directory currently represented by fid.
	// The server has checked that fid corresponds to a directory.
	//
	// If Tree is non-nil, the server has already checked that the
	// file metadata corresponding to fid permits the operation.
	// (If not, the server sends back an error without calling Create.)
	//
	// Create is optional. If Create is non-nil, the server responds with a
	// “create prohibited” error.
	Create func(ctx context.Context, fid *Fid, name string, perm plan9.Perm, mode uint8) (plan9.Qid, error)

	// Remove handles a Tremove request, removing the file or directory
	// represented by fid.
	//
	// If Tree is non-nil, the server has already checked that the
	// file metadata corresponding to fid permits the operation.
	// (If not, the server sends back an error without calling Remove.)
	// The server invokes fid.File().Remove() when Remove returns
	// successfully (or is nil).
	//
	// Remove is optional. If Remove is nil and Tree is non-nil,
	// the Remove function is considered a no-op, causing the
	// server to check the file metadata for permission and then
	// carry out the operation itself.
	// If Remove is nil and Tree is also nil, the server responds
	// with a “remove prohibited” error.
	//
	// Remove makes the fid no longer accessible to future requests,
	// but other active requests may still have references to it.
	// The server calls Clunk when there are no more pending requests
	// referring to fid. Cleanup of resources associated with fid should
	// usually be delegated to Clunk.
	Remove func(ctx context.Context, fid *Fid) error

	// Read handles a Tread request, reading from an open file or directory
	// in the manner of an [io.ReaderAt].
	// The server has checked that fid is open for read.
	// The [ReadBytes], [ReadString], and [ReadDir] functions
	// may help implement specific calls.
	//
	// If Tree is non-nil, the server handles reads of directories itself,
	// using the file tree's metadata, not using Read.
	//
	// Read is optional. If Read is nil, then the server responds with
	// a “read prohibited” error.
	Read func(ctx context.Context, fid *Fid, data []byte, offset int64) (int, error)

	// Write handles a Twrite request, writing to an open file
	// in the manner of an [io.WriterAt].
	// The server has checked that fid is open for write.
	//
	// If Tree is non-nil, the server increments fid.File's qid version
	// on each successful return from Write.
	//
	// Write is optional. If Write is nil, then the server responds with
	// a “write prohibited” error.
	Write func(ctx context.Context, fid *Fid, data []byte, offset int64) (int, error)

	// Stat handles a Tstat request, returning the file system
	// metadata for the fid.
	//
	// Stat is optional. If Stat is nil and Tree is non-nil, then
	// the server responds with metadata from the file tree.
	// Otherwise, the server responds with a “stat prohibited” error.
	Stat func(ctx context.Context, fid *Fid) (*plan9.Dir, error)

	// Wstat handles a Twstat request, updating the file system
	// metadata for fid according to d.
	// Fields that are to be left unchanged are ^0 or empty strings.
	// The server has checked that immutable fields are not being changed.
	// If no fields are being changed, the server calls Sync instead.
	//
	// If Tree is non-nil, the server has already checked that the
	// file metadata corresponding to fid permits the operation.
	// (If not, the server sends back an error without calling Wstat.)
	//
	// Wstat is optional. If Wstat is nil,
	// the server responds with a “wstat prohibited” error,
	Wstat func(ctx context.Context, fid *Fid, d *plan9.Dir) error

	// Sync handles the special case of a Twstat request
	// that changes no fields, meaning it is a request to
	// guarantee that the contents of the associated file
	// are committed to stable storage.
	//
	// Sync is optional. If Sync is nil, the request succeeds unconditionally.
	Sync func(ctx context.Context, fid *Fid) error

	// Clunk is called when fid is no longer defined on its 9P connection
	// and no longer referred to by any pending requests.
	// A typical use is to clean up resources associated with a fid.
	//
	// Clunk is not called until all requests referring to fid
	// have been completed. In particular, Clunk is called
	// only after the server sends an Rclunk, Rremove, or Rerror
	// response to the Tclunk or Tremove that undefined the fid.
	// It must simply clean up; it cannot return an error and
	// cannot be cancelled.
	//
	// Clunk is optional. A nil Clunk is equivalent to a no-op.
	Clunk func(fid *Fid)

	// Msize is the maximum 9P message size to support.
	// If zero, Msize uses a sensible default: plan9.IOHDRSZ + 8192.
	Msize uint32

	// If Trace is non-nil, the server prints a trace of all protocol messages to it.
	Trace io.Writer
}

// A conn is a single connection to a server.
type conn struct {
	srv *Server

	addr string // identifier for remote address (never set yet)

	fids  *refMap[uint32, *Fid]     // active fids
	reqs  *refMap[uint16, *request] // pending requests
	msize atomic.Uint32             // max message size

	in    io.ReadCloser  // read 9P requests from here
	outMu sync.Mutex     // protects reply logic and out
	out   io.WriteCloser // write 9P replies here
}

var (
	Ebadattach        = errors.New("unknown specifier in attach")
	errAttachMismatch = errors.New("attach does not match auth fid")
	ErrBadOffset      = errors.New("bad offset")
	errBadCount       = errors.New("bad count")
	errDuplicateOpen  = errors.New("duplicate open")
	errCreateNonDir   = errors.New("create in non-directory")
	errDuplicateTag   = errors.New("duplicate tag")
	errDuplicateFid   = errors.New("duplicate fid")
	errIsDir          = errors.New("is a directory")
	errNoCreate       = errors.New("create prohibited")
	errNoRead         = errors.New("read prohibited")
	errNoWalk         = errors.New("walk prohibited")
	errNoWrite        = errors.New("write prohibited")
	errNoRemove       = errors.New("remove prohibited")
	errNoStat         = errors.New("stat prohibited")
	errNoWstat        = errors.New("wstat prohibited")
	errNotOpenRead    = errors.New("not open for read")
	errNotOpenWrite   = errors.New("not open for write")
	errWstatType      = errors.New("wstat cannot change type")
	errWstatDev       = errors.New("wstat cannot change dev")
	errWstatQid       = errors.New("wstat cannot change qid")
	errWstatMuid      = errors.New("wstat cannot change muid")
	errWstatDMDIR     = errors.New("wstat cannot change DMDIR bit")
	Enomem            = errors.New("out of memory")
	Enoremove         = errors.New("remove prohibited")
	Enostat           = errors.New("stat prohibited")
	errNotFound       = errors.New("file not found")
	Enowrite          = errors.New("write prohibited")
	Enowstat          = errors.New("wstat prohibited")
	errPerm           = errors.New("permission denied")
	errUnknownFid     = errors.New("unknown fid")
	Ebaddir           = errors.New("bad directory in wstat")
	errNotDir         = errors.New("not a directory")
	errWalkNonDir     = errors.New("walk in non-directory")
	errCloneOpenFid   = errors.New("clone of open fid")
)

func (srv *Server) Serve(in io.ReadCloser, out io.WriteCloser) {
	c := &conn{
		srv:  srv,
		fids: newRefMap[uint32, *Fid](),
		reqs: newRefMap[uint16, *request](),
		in:   in,
		out:  out,
	}
	msize := srv.Msize
	if msize == 0 {
		msize = 8*1024 + plan9.IOHDRSZ
	}
	c.msize.Store(msize)

	for {
		r, err := c.readReq()
		if err != nil {
			break
		}
		if r.err != nil {
			go r.respond()
			continue
		}
		go c.serveRequest(r)
	}

	c.msize.Store(0) // stop future writes
	c.fids.clear()
}

func (c *conn) serveRequest(r *request) {
	switch r.ifcall.Type {
	default:
		r.err = errors.New("unknown message")
	case plan9.Tversion:
		c.version(r)
	case plan9.Tauth:
		c.auth(r)
	case plan9.Tattach:
		c.attach(r)
	case plan9.Tflush:
		if !c.flush(r) {
			return
		}
	case plan9.Twalk:
		c.walk(r)
	case plan9.Topen:
		c.open(r)
	case plan9.Tcreate:
		c.create(r)
	case plan9.Tread:
		c.read(r)
	case plan9.Twrite:
		c.write(r)
	case plan9.Tclunk:
		c.clunk(r)
	case plan9.Tremove:
		c.remove(r)
	case plan9.Tstat:
		c.stat(r)
	case plan9.Twstat:
		c.wstat(r)
	}
	r.respond()
}

func (r *request) respond() {
	conn := r.conn

	if conn == nil {
		panic("srv9p: invalid Req: missing conn")
	}
	if r.responded.Load() {
		panic("srv9p: already responded to Req: " + r.ifcall.String())
	}

	r.ofcall.Tag = r.ifcall.Tag
	r.ofcall.Type = r.ifcall.Type + 1
	if r.err != nil {
		r.ofcall.Ename = r.err.Error()
		r.ofcall.Type = plan9.Rerror
	}

	if conn.srv.Trace != nil {
		fmt.Fprintf(conn.srv.Trace, "-%s-> %v\n", conn.addr, r.ofcall)
	}

	msg, err := r.ofcall.Bytes()
	conn.outMu.Lock()
	if !r.duplicate {
		// Remove from the pool while holding srv.write
		// so that any flush that arrives and doesn't find
		// r in the request pool will not be able to respond
		// before we send this response, avoiding a protocol violation.
		// We run the decRef after unlocking.
		r.conn.reqs.drop(r.ifcall.Tag)
	}
	if err != nil {
		log.Printf("fcall marshal: %v", err)
	} else if msize := int64(conn.msize.Load()); int64(len(msg)) > msize {
		if msize > 0 {
			log.Printf("fcall too big")
		}
	} else {
		if _, err := conn.out.Write(msg); err != nil {
			log.Printf("srv write: %v", err)
		}
	}
	conn.outMu.Unlock()

	r.mu.Lock()
	r.responded.Store(true) // r.flush no longer accessed by others
	r.mu.Unlock()
	for _, f := range r.flush {
		f.respond()
	}
	r.flush = nil
}

// readReq reads the next 9P request message from the connection
// and returns a Req for that request.
func (c *conn) readReq() (*request, error) {
	f, err := c.readFcall()
	if err != nil {
		return nil, err
	}

	r := c.newReq(f)
	if c.srv.Trace != nil {
		if r.err != nil {
			fmt.Fprintf(c.srv.Trace, "<-%s- %v: %v\n", c.addr, r.ifcall, r.err)
		} else {
			fmt.Fprintf(c.srv.Trace, "<-%s- %v\n", c.addr, r.ifcall)
		}
	}
	return r, nil
}

// readFcall reads a single 9P request message from the connection.
func (c *conn) readFcall() (*plan9.Fcall, error) {
	// 128 bytes should be enough for most messages
	buf := make([]byte, 128)
	_, err := io.ReadFull(c.in, buf[0:4])
	if err != nil {
		return nil, err
	}

	// read 4-byte header, make room for remainder
	n := binary.LittleEndian.Uint32(buf)
	if n < 4 {
		return nil, plan9.ProtocolError("invalid length")
	}
	if n > c.msize.Load() {
		return nil, plan9.ProtocolError("message too long")
	}

	if int(n) <= len(buf) {
		buf = buf[0:n]
	} else {
		buf = make([]byte, n)
		binary.LittleEndian.PutUint32(buf[0:4], uint32(n))
	}

	// read remainder and unpack
	_, err = io.ReadFull(c.in, buf[4:])
	if err != nil {
		return nil, err
	}
	return plan9.UnmarshalFcall(buf)
}

func (c *conn) version(r *request) {
	if !strings.HasPrefix(r.ifcall.Version, "9P") {
		r.ofcall.Version = "unknown"
	} else {
		r.ofcall.Version = "9P2000"
	}
	r.ofcall.Msize = min(r.ifcall.Msize, c.msize.Load())
	c.msize.Store(r.ofcall.Msize)
}

func (c *conn) auth(r *request) {
	var afid *Fid
	if afid, r.err = c.newFid(r.ifcall.Afid); r.err != nil {
		return
	}
	defer afid.decRef()

	if c.srv.Auth == nil {
		r.err = errors.New(filepath.Base(os.Args[0]) + ": authentication not required")
		c.fids.drop(r.ifcall.Afid)
		return
	}

	afid.uid = r.ifcall.Uname
	afid.aname = r.ifcall.Aname
	var qid plan9.Qid
	qid, r.err = c.srv.Auth(r.ctx, afid, r.ifcall.Uname, r.ifcall.Aname)
	if r.err != nil {
		c.fids.drop(r.ifcall.Afid)
		return
	}
	afid.SetQid(qid)
	r.ofcall.Qid = qid
}

func (c *conn) attach(r *request) {
	var fid, afid *Fid
	if r.ifcall.Afid != plan9.NOFID {
		if afid = c.fids.lookup(r.ifcall.Afid); afid == nil {
			r.err = errUnknownFid
			return
		}
		if afid.uid != r.ifcall.Uname || afid.aname != r.ifcall.Aname {
			r.err = errAttachMismatch
			return
		}
		defer afid.decRef()
	}
	if fid, r.err = c.newFid(r.ifcall.Fid); r.err != nil {
		return
	}
	defer fid.decRef()

	fid.uid = r.ifcall.Uname
	qid := plan9.Qid{Type: plan9.QTDIR}
	if c.srv.Tree != nil {
		fid.SetFile(c.srv.Tree.Root)
		qid = fid.File().Stat.Qid
		fid.SetQid(qid)
	}
	if c.srv.Attach != nil {
		qid, r.err = c.srv.Attach(r.ctx, fid, afid, r.ifcall.Uname, r.ifcall.Aname)
		if r.err != nil {
			c.fids.drop(r.ifcall.Fid)
			return
		}
		fid.SetQid(qid)
	}
	r.ofcall.Qid = qid
}

func (c *conn) flush(r *request) bool {
	old := c.reqs.lookup(r.ifcall.Oldtag)
	defer old.decRef()

	if old == nil || old == r {
		return true
	}

	old.cancel()

	old.mu.Lock()
	if !old.responded.Load() {
		old.flush = append(old.flush, r)
		old.mu.Unlock()
		return false
	}
	old.mu.Unlock()

	return true
}

func (c *conn) clunk(r *request) {
	fid := c.fids.delete(r.ifcall.Fid)
	if fid == nil {
		r.err = errUnknownFid
	}
	fid.decRef()
}

func (c *conn) walk(r *request) {
	fid := c.fids.lookup(r.ifcall.Fid)
	if fid == nil {
		r.err = errUnknownFid
		return
	}
	defer fid.decRef()

	if fid.omode.Load() != -1 {
		r.err = errCloneOpenFid
		return
	}
	if len(r.ifcall.Wname) > 0 && fid.Qid().Type&plan9.QTDIR == 0 {
		r.err = errWalkNonDir
		return
	}

	var newfid *Fid
	if r.ifcall.Fid == r.ifcall.Newfid {
		newfid = fid
		newfid.incRef()
	} else {
		newfid, r.err = c.newFid(r.ifcall.Newfid)
		if r.err != nil {
			return
		}
		newfid.uid = fid.uid
	}
	defer newfid.decRef()

	switch {
	default:
		r.err = errNoWalk
	case fid.File() != nil:
		r.ofcall.Wqid, r.err = fid.File().Walk(r, fid, newfid, r.ifcall.Wname)
	case c.srv.Walk != nil:
		r.ofcall.Wqid, r.err = c.srv.Walk(r.ctx, fid, newfid, r.ifcall.Wname)
	}

	if r.err != nil || len(r.ofcall.Wqid) < len(r.ifcall.Wname) {
		if r.ifcall.Fid != r.ifcall.Newfid && newfid != nil {
			c.fids.drop(r.ifcall.Newfid)
		}
		if len(r.ofcall.Wqid) == 0 {
			if r.err == nil && len(r.ifcall.Wname) > 0 {
				r.err = errNotFound
			}
		} else {
			r.err = nil // no error on partial walks
		}
		return
	}

	var qid plan9.Qid
	if len(r.ofcall.Wqid) == 0 { // clone
		qid = fid.Qid()
	} else {
		qid = r.ofcall.Wqid[len(r.ofcall.Wqid)-1]
	}
	newfid.SetQid(qid)
}

func Walk(fid, newfid *Fid, names []string, clone func(old, new *Fid) error, walk1 func(f *Fid, elem string) (plan9.Qid, error)) ([]plan9.Qid, error) {
	if fid == newfid && len(names) > 1 {
		// The problem here is how to rewind the fid on error.
		// The Plan 9 kernel does not send such a request,
		// so we don't bother implementing it.
		// We could instead make a temporary fid and then replace the original in the pool.
		return nil, errors.New("srv9p: multiwalk without clone not implemented")
	}

	if fid != newfid {
		newfid.qid = fid.qid
		if clone != nil {
			if err := clone(fid, newfid); err != nil {
				return nil, err
			}
		}
	}

	var qids []plan9.Qid
	for _, name := range names {
		q, err := walk1(newfid, name)
		if err != nil {
			return qids, err
		}
		newfid.SetQid(q)
		qids = append(qids, q)
		if q.Type&plan9.QTDIR == 0 {
			break
		}
	}
	return qids, nil
}

func (c *conn) open(r *request) {
	fid := c.fids.lookup(r.ifcall.Fid)
	if fid == nil {
		r.err = errUnknownFid
		return
	}
	defer fid.decRef()

	if fid.omode.Load() != -1 {
		r.err = errDuplicateOpen
		return
	}
	qid := fid.Qid()
	if qid.Type&plan9.QTDIR != 0 && r.ifcall.Mode&^plan9.ORCLOSE != plan9.OREAD {
		r.err = errIsDir
		return
	}
	r.ofcall.Qid = qid
	var perm int
	switch r.ifcall.Mode & 3 {
	case plan9.OREAD:
		perm = plan9.AREAD
	case plan9.OWRITE:
		perm = plan9.AWRITE
	case plan9.ORDWR:
		perm = plan9.AREAD | plan9.AWRITE
	case plan9.OEXEC:
		perm = plan9.AEXEC
	}
	if r.ifcall.Mode&plan9.OTRUNC != 0 {
		perm |= plan9.AWRITE
	}
	if qid.Type&plan9.QTDIR != 0 && perm != plan9.AREAD {
		r.err = errPerm
		return
	}
	if file := fid.File(); file != nil {
		if !hasPerm(file, fid.uid, perm) {
			r.err = errPerm
			return
		}
		if r.ifcall.Mode&plan9.ORCLOSE != 0 && !hasPerm(file.parent, fid.uid, plan9.AWRITE) {
			r.err = errPerm
			return
		}
		r.ofcall.Qid = file.Stat.Qid
		if r.ofcall.Qid.Type&plan9.QTDIR != 0 {
			var dr *dirReader
			dr, r.err = file.openDir()
			if r.err != nil {
				return
			}
			fid.dirReader.Store(dr)
		}
	}

	if c.srv.Open != nil {
		r.err = c.srv.Open(r.ctx, fid, r.ifcall.Mode)
		if r.err != nil {
			return
		}
	}

	fid.omode.Store(int32(r.ifcall.Mode))
	r.ofcall.Qid = fid.Qid()
	r.ofcall.Iounit = fid.Iounit()
}

func (c *conn) create(r *request) {
	fid := c.fids.lookup(r.ifcall.Fid)
	if fid == nil {
		r.err = errUnknownFid
		return
	}
	defer fid.decRef()

	switch {
	case fid.omode.Load() != -1:
		r.err = errDuplicateOpen
	case fid.Qid().Type&plan9.QTDIR == 0:
		r.err = errCreateNonDir
	case fid.File() != nil && !hasPerm(fid.File(), fid.uid, plan9.AWRITE):
		r.err = errPerm
	case c.srv.Create == nil:
		r.err = errNoCreate
	default:
		r.ofcall.Qid, r.err = c.srv.Create(r.ctx, fid, r.ifcall.Name, r.ifcall.Perm, r.ifcall.Mode)
		if r.err == nil {
			fid.omode.Store(int32(r.ifcall.Mode))
			fid.SetQid(r.ofcall.Qid)
		}
	}
}

func (c *conn) read(r *request) {
	fid := c.fids.lookup(r.ifcall.Fid)
	if fid == nil {
		r.err = errUnknownFid
		return
	}
	defer fid.decRef()

	if int32(r.ifcall.Count) < 0 {
		r.err = errBadCount
		return
	}
	if int64(r.ifcall.Offset) < 0 || (fid.Qid().Type&plan9.QTDIR != 0 && r.ifcall.Offset != 0 && int64(r.ifcall.Offset) != fid.dirOffset.Load()) {
		r.err = ErrBadOffset
		return
	}

	if msize := c.msize.Load(); r.ifcall.Count > msize-plan9.IOHDRSZ {
		r.ifcall.Count = msize - plan9.IOHDRSZ
	}
	r.ofcall.Data = make([]byte, r.ifcall.Count)
	if o := fid.omode.Load() & 3; o != plan9.OREAD && o != plan9.ORDWR && o != plan9.OEXEC {
		r.err = errNotOpenRead
		return
	}
	var n int
	if dr := fid.dirReader.Load(); dr != nil {
		n, r.err = dr.Read(r.ofcall.Data)
	} else if c.srv.Read == nil {
		r.err = errNoRead
	} else {
		n, r.err = c.srv.Read(r.ctx, fid, r.ofcall.Data, int64(r.ifcall.Offset))
	}
	if r.err != nil {
		return
	}
	r.ofcall.Data = r.ofcall.Data[:n]
	if fid.Qid().Type&plan9.QTDIR != 0 {
		fid.dirOffset.Store(int64(r.ifcall.Offset) + int64(n))
	}
}

func (c *conn) write(r *request) {
	fid := c.fids.lookup(r.ifcall.Fid)
	if fid == nil {
		r.err = errUnknownFid
		return
	}
	defer fid.decRef()

	if int64(r.ifcall.Offset) < 0 {
		r.err = ErrBadOffset
		return
	}
	if o := fid.omode.Load() & 3; o != plan9.OWRITE && o != plan9.ORDWR {
		r.err = errNotOpenWrite
		return
	}
	if c.srv.Write == nil {
		r.err = errNoWrite
		return
	}
	n, err := c.srv.Write(r.ctx, fid, r.ifcall.Data, int64(r.ifcall.Offset))
	if err != nil {
		r.err = err
		return
	}
	r.ofcall.Count = uint32(n)
	if file := fid.File(); file != nil {
		file.Stat.Qid.Vers++
	}
}

func (c *conn) remove(r *request) {
	fid := c.fids.delete(r.ifcall.Fid)
	if fid == nil {
		r.err = errUnknownFid
		return
	}
	defer fid.decRef()

	if file := fid.File(); file != nil && !hasPerm(file.parent, fid.uid, plan9.AWRITE) {
		r.err = errPerm
		return
	}
	if c.srv.Remove != nil {
		r.err = c.srv.Remove(r.ctx, fid)
		if r.err != nil {
			return
		}
	} else if c.srv.Tree == nil {
		r.err = errNoRemove
		return
	}
	if file := fid.File(); file != nil {
		// Permission check above succeeded, and
		// remove succeeded or is nil; carry out operation.
		r.err = file.remove()
		fid.SetFile(nil)
	}
}

func (c *conn) stat(r *request) {
	fid := c.fids.lookup(r.ifcall.Fid)
	if fid == nil {
		r.err = errUnknownFid
		return
	}
	defer fid.decRef()

	var d *plan9.Dir
	if c.srv.Stat != nil {
		d, r.err = c.srv.Stat(r.ctx, fid)
	} else if file := fid.File(); file != nil {
		// TODO lock and copy?
		d = &file.Stat
	} else {
		r.err = errNoStat
	}
	if r.err != nil {
		return
	}
	stat, err := d.Bytes()
	if err != nil {
		r.err = err
		return
	}
	r.ofcall.Stat = stat
}

func (c *conn) wstat(r *request) {
	fid := c.fids.lookup(r.ifcall.Fid)
	if fid == nil {
		r.err = errUnknownFid
		return
	}
	defer fid.decRef()

	d, err := plan9.UnmarshalDir(r.ifcall.Stat)
	if err != nil {
		r.err = err
		return
	}
	if d.IsNull() {
		if c.srv.Sync != nil {
			r.err = c.srv.Sync(r.ctx, fid)
		}
		return
	}
	if c.srv.Wstat == nil {
		r.err = errNoWstat
		return
	}
	switch {
	case ^d.Type != 0:
		r.err = errWstatType
	case ^d.Dev != 0:
		r.err = errWstatDev
		return
	case ^d.Qid.Type != 0 || ^d.Qid.Vers != 0 || ^d.Qid.Path != 0:
		r.err = errWstatQid
		return
	case d.Muid != "":
		r.err = errWstatMuid
		return
	case ^d.Mode != 0 && (d.Mode&plan9.DMDIR != 0) != (fid.Qid().Type&plan9.QTDIR != 0):
		r.err = errWstatDMDIR
	default:
		r.err = c.srv.Wstat(r.ctx, fid, d)
	}
}
