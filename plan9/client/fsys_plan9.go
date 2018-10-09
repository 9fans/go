package client

import (
	"os"
	"path/filepath"

	"9fans.net/go/plan9"
)

type Fsys struct {
	Mtpt string
}

func (c *Conn) Attach(afid *Fid, user, aname string) (*Fsys, error) {
	panic("unimplemented")
}

func (fs *Fsys) Access(name string, mode int) error {
	panic("unimplemented")
}

func (fs *Fsys) Create(name string, mode uint8, perm plan9.Perm) (*Fid, error) {
	panic("unimplemented")
}

func (fs *Fsys) Open(name string, mode uint8) (*Fid, error) {
	f, err := os.OpenFile(filepath.Join(fs.Mtpt, name), int(mode), 0)
	return &Fid{File: f}, err
}

func (fs *Fsys) Remove(name string) error {
	panic("unimplemented")
}

func (fs *Fsys) Stat(name string) (*plan9.Dir, error) {
	panic("unimplemented")
}

func (fs *Fsys) Wstat(name string, d *plan9.Dir) error {
	panic("unimplemented")
}
