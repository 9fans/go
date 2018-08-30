// +build !plan9

package acme

import "9fans.net/go/plan9/client"

func mountAcme() {
	fs, err := client.MountService("acme")
	fsys = &fsysWrapper{fs}
	fsysErr = err
}

type fsysWrapper struct {
	*client.Fsys
}

func (fs *fsysWrapper) Open(name string, mode uint8) (acmeFid, error) {
	return fs.Fsys.Open(name, mode)
}
