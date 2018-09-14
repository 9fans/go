package plumb

import (
	"os"
	"path/filepath"
)

// Open opens the plumbing file with the given name and open mode.
func Open(name string, mode int) (*os.File, error) {
	return os.OpenFile(filepath.Join("/mnt/plumb", name), mode, 0)
}
