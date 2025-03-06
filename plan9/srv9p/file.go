package srv9p

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"9fans.net/go/plan9"
)

/*
 * File trees.
 */
type File struct {
	Aux  any
	Stat plan9.Dir

	tree *Tree

	ref     atomic.Int32
	readers atomic.Int32
	parent  *File
	/*
	 * To avoid deadlock, the following rules must be followed.
	 * Always lock child then parent, never parent then child.
	 */
	mu      sync.RWMutex
	child   []*File
	deleted int
}

type Tree struct {
	Root      *File
	cleanup   func(*File)
	genlock   sync.Mutex
	qidgen    atomic.Uint64
	dirqidgen atomic.Uint64
}

func NewTree(uid, gid string, mode plan9.Perm, cleanup func(*File)) *Tree {
	if uid == "" {
		uid = cmp.Or(os.Getenv("USER"), os.Getenv("user"), "none")
	}
	if gid == "" {
		gid = uid
	}
	if cleanup == nil {
		cleanup = func(*File) {}
	}
	now := time.Now().Unix()

	t := &Tree{
		cleanup: cleanup,
	}
	t.dirqidgen.Add(1)

	f := &File{
		tree: t,
		Stat: plan9.Dir{
			Name:  "/",
			Uid:   uid,
			Gid:   gid,
			Muid:  uid,
			Qid:   plan9.Qid{0, 0, plan9.QTDIR},
			Mtime: uint32(now),
			Atime: uint32(now),
			Mode:  plan9.DMDIR | mode,
		},
	}
	f.parent = f
	t.Root = f

	return t
}

func (f *File) Create(name, uid string, perm plan9.Perm, aux any) (*File, error) {
	if f.Stat.Qid.Type&plan9.QTDIR == 0 {
		return nil, fmt.Errorf("create in non-directory")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	/*
	 * We might encounter blank spots along the
	 * way due to deleted files that have not yet
	 * been flushed from the file list.  Don't reuse
	 * those - some apps (e.g., omero) depend on
	 * the file order reflecting creation order.
	 * Always create at the end of the list.
	 */
	for _, c := range f.child {
		if c != nil && c.Stat.Name == name {
			return nil, fmt.Errorf("file already exists")
		}
	}

	t := f.tree
	qid := plan9.Qid{Path: t.qidgen.Add(1)}
	if perm&plan9.DMDIR != 0 {
		qid.Type |= plan9.QTDIR
	}
	if perm&plan9.DMAPPEND != 0 {
		qid.Type |= plan9.QTAPPEND
	}
	if perm&plan9.DMEXCL != 0 {
		qid.Type |= plan9.QTEXCL
	}
	now := uint32(time.Now().Unix())

	c := &File{
		Aux:    aux,
		tree:   t,
		parent: f,
		Stat: plan9.Dir{
			Name:  name,
			Qid:   qid,
			Uid:   uid,
			Gid:   f.Stat.Gid,
			Muid:  uid,
			Mode:  perm,
			Mtime: now,
			Atime: now,
		},
	}

	f.child = append(f.child, c)

	return c, nil
}

// lookup looks up elem in the directory f and returns it if found.
// lookup consumes a reference to f, and it returns a new reference
// to its result.
func (f *File) lookup(name string) *File {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if name == ".." {
		return f.parent
	}
	for _, c := range f.child {
		if c != nil && c.Stat.Name == name {
			return c
		}
	}
	return nil
}

// walk evaluates the slash-separated path name relative to f.
// walk consumes a reference to f, and it returns a new reference to its result.
func (f *File) walk(name string) *File {
	for f != nil && name != "" {
		var elem string
		elem, name, _ = strings.Cut(name, "/")
		f = f.walk(elem)
	}
	return f
}

// remove removes f from the tree.
// It consumes a reference to f.
func (f *File) remove() (err error) {
	fp := f.parent
	if fp == nil {
		return errors.New("no parent")
	}
	if fp == f {
		return errors.New("cannot remove root")
	}

	f.mu.Lock()
	fp.mu.Lock()
	if len(f.child)-f.deleted != 0 {
		fp.mu.Unlock()
		f.mu.Unlock()
		return errors.New("not empty")
	}

	if f.parent != fp {
		fp.mu.Unlock()
		f.mu.Unlock()
		return errors.New("parent changed underfoot")
	}

	i := slices.Index(fp.child, f)
	if i < 0 {
		fp.mu.Unlock()
		f.mu.Unlock()
		return errors.New("not found in parent")
	}
	f.parent = nil
	fp.child[i] = nil
	fp.deleted++
	fp.trimChildList()
	f.mu.Unlock()
	fp.mu.Unlock()

	return nil
}

func (f *File) trimChildList() {
	/*
	 * can't delete filelist structures while there
	 * are open readers of this directory, because
	 * they might have references to the structures.
	 * instead, just leave the empty refs in the list
	 * until there is no activity and then clean up.
	 */
	if f.readers.Load() != 0 || f.deleted == 0 {
		return
	}

	w := 0
	for _, c := range f.child {
		if c != nil {
			f.child[w] = c
			w++
		}
	}
	clear(f.child[w:])
	f.child = f.child[:w]
	f.deleted = 0
}

// A dirReader is an open directory reader.
type dirReader struct {
	dir *File
	i   int // next child index to read
}

func (f *File) openDir() (*dirReader, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.Stat.Mode&plan9.DMDIR == 0 {
		return nil, errNotDir
	}

	r := &dirReader{dir: f}
	f.readers.Add(1)
	return r, nil
}

func (r *dirReader) Read(b []byte) (int, error) {
	f := r.dir
	if f == nil {
		return 0, nil
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	n := 0
	for ; r.i < len(f.child); r.i++ {
		c := f.child[r.i]
		if c == nil {
			continue
		}
		stat, err := c.Stat.Bytes()
		if err != nil {
			continue
		}
		if len(stat) > len(b) {
			break
		}
		copy(b, stat)
		n += len(stat)
		b = b[len(stat):]
	}
	return n, nil
}

func (r *dirReader) Close() error {
	if r == nil {
		return nil
	}
	f := r.dir
	if f == nil || f.readers.Add(-1) == 0 {
		return nil
	}

	f.mu.Lock()
	f.trimChildList()
	f.mu.Unlock()

	return nil
}

func (f *File) Walk(r *request, fid, newfid *Fid, names []string) ([]plan9.Qid, error) {
	var qids []plan9.Qid
	for _, name := range names {
		f = f.lookup(name)
		if f == nil {
			break
		}
		qids = append(qids, f.Stat.Qid)
	}
	if f != nil {
		newfid.SetFile(f)
		newfid.SetQid(f.Stat.Qid)
	}
	return qids, nil
}
