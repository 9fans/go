package client

import (
	"path/filepath"
	"syscall"
)

func openSrv(service string) (fd int, err error) {
	p := filepath.Join(Namespace(), service)
	return syscall.Open(p, syscall.O_RDWR)
}

func DialService(service string) (*Conn, error) {
	fd, err := openSrv(service)
	if err != nil {
		return nil, err
	}
	return &Conn{fd: fd, name: service}, nil
}

func Mount(network, addr string) (*Fsys, error) {
	panic("TODO")
}

func MountService(service string) (*Fsys, error) {
	panic("TODO")
}

// Namespace returns the path to the name space directory.
func Namespace() string {
	return "/srv"
}
