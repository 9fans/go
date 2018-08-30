package acme

type acmeFsys interface {
	Open(name string, mode uint8) (acmeFid, error)
}

type acmeFid interface {
	Close() error
	Read(b []byte) (n int, err error)
	ReadAt(b []byte, offset int64) (n int, err error)
	Seek(n int64, whence int) (int64, error)
	Write(b []byte) (n int, err error)
}
