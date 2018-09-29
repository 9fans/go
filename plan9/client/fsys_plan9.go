package client

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"9fans.net/go/plan9"
)

type Fsys struct {
	Mtpt string
}

func (c *Conn) Attach(afid *Fid, user, aname string) (*Fsys, error) {
	// TODO(fhs): what if something else is already using this mount point?
	mtpt := fmt.Sprintf("/n/9fans.%s", c.name)
	if len(aname) > 0 {
		mtpt += "." + aname
	}
	afd := -1
	if afid != nil {
		afd = int(afid.File.Fd())
	}
	err := syscall.Mount(c.fd, afd, mtpt, plan9.MREPL, aname)
	if err != nil {
		return nil, err
	}
	return &Fsys{Mtpt: mtpt}, nil
}

func (fs *Fsys) Access(name string, mode int) error {
	panic("TODO")
}

func (fs *Fsys) Create(name string, mode uint8, perm plan9.Perm) (*Fid, error) {
	panic("TODO")
}

func (fs *Fsys) Open(name string, mode uint8) (*Fid, error) {
	f, err := os.OpenFile(filepath.Join(fs.Mtpt, name), int(mode), 0)
	return &Fid{File: f}, err
}

func (fs *Fsys) Remove(name string) error {
	panic("TODO")
}

func (fs *Fsys) Stat(name string) (*plan9.Dir, error) {
	panic("TODO")
}

func (fs *Fsys) Wstat(name string, d *plan9.Dir) error {
	panic("TODO")
}
