// +build !plan9

package plumb

import (
	"sync"

	"9fans.net/go/plan9/client"
)

var fsys *client.Fsys
var fsysErr error
var fsysOnce sync.Once

func mountPlumb() {
	fsys, fsysErr = client.MountService("plumb")
}

// Open opens the plumbing file with the given name and open mode.
func Open(name string, mode int) (*client.Fid, error) {
	fsysOnce.Do(mountPlumb)
	if fsysErr != nil {
		return nil, fsysErr
	}
	fid, err := fsys.Open(name, uint8(mode))
	if err != nil {
		return nil, err
	}
	return fid, nil
}
