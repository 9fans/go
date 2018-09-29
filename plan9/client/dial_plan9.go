package client

import (
	"fmt"
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
	fd, err := openSrv(service)
	if err != nil {
		return nil, err
	}
	// TODO(fhs): what if something else is already using this mount point?
	mtpt := fmt.Sprintf("/n/9fans.%s", service)
	err = syscall.Mount(fd, -1, mtpt, 0, "")
	if err != nil {
		return nil, err
	}
	return &Fsys{Mtpt: mtpt}, nil
}

// Namespace returns the path to the name space directory.
func Namespace() string {
	return "/srv"
}
