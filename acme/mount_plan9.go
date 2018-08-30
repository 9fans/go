package acme

import (
	"os"
	"path/filepath"
)

func mountAcme() {
	// Already mounted at /mnt/acme
	fsys = fsysDir("/mnt/acme")
	fsysErr = nil
}

type fsysDir string

func (fs fsysDir) Open(name string, mode uint8) (acmeFid, error) {
	return os.OpenFile(filepath.Join(string(fs), name), int(mode), 0)
}
