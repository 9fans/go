// Package fs implements io/fs.FS for a 9p filesystem.
// An example is to run a local http.FileServer with a remote 9p system.
package fs

import (
	"io"
	"io/fs"
	"strings"
	"time"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

// NewFS returns an io/fs.FS filesystem that wraps a 9p filesystem.
func NewFS(fsys *client.Fsys) fs.FS {
	return fs9p{fsys: fsys}
}

// MountWithNames attaches to the 9p filesystem at network!addr with the
// provided user and attach names and wraps an io/fs.FS filesystem around it.
// Assumes the 9p server does not require Auth.
func MountWithNames(network, addr, uname, aname string) (fs.FS, error) {
	c, err := client.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	fsys, err := c.Attach(nil, uname, aname)
	if err != nil {
		c.Close()
	}
	return fs9p{fsys: fsys}, nil
}

// fs9p implements io/fs.FS.
type fs9p struct {
	fsys *client.Fsys
}

func (f fs9p) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}

	fid, err := f.fsys.Open(name, plan9.OREAD)
	if err != nil {
		// error detection is based on freebsd/u9fs
		werr := err
		if strings.Contains(werr.Error(), "Permission denied") {
			werr = fs.ErrPermission
		} else if strings.Contains(werr.Error(), "No such file or directory") {
			werr = fs.ErrNotExist
		}
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  werr,
		}
	}

	if fid.Qid().Type&plan9.QTDIR > 0 {
		return &dir9p{file9p: file9p{fid: fid}}, nil
	}
	return file9p{fid: fid}, nil
}

// file9p implements io/fs.File.
type file9p struct {
	fid *client.Fid
}

func (f file9p) Stat() (fs.FileInfo, error) {
	dir, err := f.fid.Stat()
	if err != nil {
		return nil, err
	}
	return fileInfo9p{sys: dir}, nil

}

func (f file9p) Read(b []byte) (int, error) {
	return f.fid.Read(b)
}

func (f file9p) Close() error {
	return f.fid.Close()
}

func (f file9p) ReadAt(p []byte, off int64) (n int, err error) {
	return f.fid.ReadAt(p, off)
}

func (f file9p) Seek(offset int64, whence int) (int64, error) {
	return f.fid.Seek(offset, whence)
}

// dir9p implements io/fs.ReadDirFile.
type dir9p struct {
	file9p

	dirsRead []*plan9.Dir // directories read ahead by dirread
}

func (d *dir9p) ReadDir(n int) ([]fs.DirEntry, error) {
	var dirs []*plan9.Dir
	var err error

	if n <= 0 {
		dirs, err = d.fid.Dirreadall()
		dirs = append(d.dirsRead, dirs...) // preserve directory order
		d.dirsRead = d.dirsRead[:0]
	} else {
		for err == nil && len(d.dirsRead) < n {
			dirs, err = d.fid.Dirread()
			d.dirsRead = append(d.dirsRead, dirs...) // preserve directory order
		}
		if len(d.dirsRead) >= n {
			dirs = d.dirsRead[0:n]
			d.dirsRead = d.dirsRead[n:]
		} else {
			dirs = d.dirsRead
			d.dirsRead = d.dirsRead[:0]
		}
	}

	if len(d.dirsRead) > 0 && err == io.EOF {
		err = nil
	}

	entries := make([]fs.DirEntry, len(dirs))
	for i, dir := range dirs {
		entries[i] = dirEntry9p{sys: dir}
	}
	return entries, err
}

// fileInfo9p implements io/fs.FileInfo.
type fileInfo9p struct {
	sys *plan9.Dir
}

func (f fileInfo9p) Name() string {
	return f.sys.Name
}

func (f fileInfo9p) Size() int64 {
	// for directories size is implementation defined. Use 0 for portability.
	if f.sys.Mode&plan9.DMDIR > 0 {
		return 0
	}

	// 9p uses uint64 but FileInfo uses int64. For most practical cases,
	// the conversion it OK.
	return int64(f.sys.Length)
}

func (f fileInfo9p) Mode() fs.FileMode {
	// init mode to the permission bits and then set the others
	mode := fs.FileMode(f.sys.Mode & 0777)
	if f.sys.Mode&plan9.DMDIR > 0 {
		mode |= fs.ModeDir
	}
	if f.sys.Mode&plan9.DMAPPEND > 0 {
		mode |= fs.ModeAppend
	}
	if f.sys.Mode&plan9.DMEXCL > 0 {
		mode |= fs.ModeExclusive
	}
	if f.sys.Mode&plan9.DMTMP > 0 {
		mode |= fs.ModeTemporary
	}

	// The following are not defined by 9p (http://9p.io/sys/man/5/INDEX.html)
	// but are defined by the plan9 client
	if f.sys.Mode&plan9.DMSYMLINK > 0 {
		mode |= fs.ModeSymlink
	}
	if f.sys.Mode&plan9.DMDEVICE > 0 {
		mode |= fs.ModeDevice
	}
	if f.sys.Mode&plan9.DMNAMEDPIPE > 0 {
		mode |= fs.ModeNamedPipe
	}
	if f.sys.Mode&plan9.DMSOCKET > 0 {
		mode |= fs.ModeSocket
	}
	if f.sys.Mode&plan9.DMSETUID > 0 {
		mode |= fs.ModeSetuid
	}
	if f.sys.Mode&plan9.DMSETGID > 0 {
		mode |= fs.ModeSetgid
	}

	return mode
}

func (f fileInfo9p) ModTime() time.Time {
	return time.Unix(int64(f.sys.Mtime), 0)
}

func (f fileInfo9p) IsDir() bool {
	return f.sys.Mode&plan9.DMDIR > 0
}

func (f fileInfo9p) Sys() interface{} {
	return f.sys
}

// dirEntry9p implements io/fs.DirEntry.
type dirEntry9p struct {
	sys *plan9.Dir
}

func (d dirEntry9p) Name() string {
	return d.sys.Name
}

func (d dirEntry9p) IsDir() bool {
	return d.sys.Mode&plan9.DMDIR > 0
}

func (d dirEntry9p) Info() (fs.FileInfo, error) {
	return fileInfo9p{sys: d.sys}, nil
}

func (d dirEntry9p) Type() fs.FileMode {
	var mode fs.FileMode
	if d.sys.Mode&plan9.DMDIR > 0 {
		mode |= fs.ModeDir
	}

	// The following are not defined by 9p (http://9p.io/sys/man/5/INDEX.html)
	// but are defined by the plan9 client
	if d.sys.Mode&plan9.DMSYMLINK > 0 {
		mode |= fs.ModeSymlink
	}
	if d.sys.Mode&plan9.DMDEVICE > 0 {
		mode |= fs.ModeDevice
	}
	if d.sys.Mode&plan9.DMNAMEDPIPE > 0 {
		mode |= fs.ModeNamedPipe
	}
	if d.sys.Mode&plan9.DMSOCKET > 0 {
		mode |= fs.ModeSocket
	}

	return mode
}
