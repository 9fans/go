package disk

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"unsafe"

	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
)

const blockincr = 256

const maxblock = 8 * 1024

var disk *Disk

type Disk struct {
	fd   *os.File
	addr int64
	free [maxblock/blockincr + 1]*block
}

type block struct {
	addr int64
	u    struct {
		n    int
		next *block
	}
}

func Init() {
	disk = newDisk()
}

func newDisk() *Disk {
	d := new(Disk)
	d.fd = TempFile()
	return d
}

func roundSize(n int, ip *int) int {
	if n > maxblock {
		util.Fatal("internal error: ntosize")
	}
	size := n
	if size&(blockincr-1) != 0 {
		size += blockincr - (size & (blockincr - 1))
	}
	/* last bucket holds blocks of exactly Maxblock */
	if ip != nil {
		*ip = size / blockincr
	}
	return size * runes.RuneSize
}

func (d *Disk) allocBlock(n int) *block {
	var i int
	size := roundSize(n, &i)
	b := d.free[i]
	if b != nil {
		d.free[i] = b.u.next
	} else {
		/* allocate in chunks to reduce malloc overhead */
		if blist == nil {
			bb := make([]block, 100)
			for j := 0; j < 100-1; j++ {
				bb[j].u.next = &bb[j+1]
			}
			blist = &bb[0]
		}
		b = blist
		blist = b.u.next
		b.addr = d.addr
		if d.addr+int64(size) < d.addr {
			util.Fatal("temp file overflow")
		}
		d.addr += int64(size)
	}
	b.u.n = n
	return b
}

func (d *Disk) freeBlock(b *block) {
	var i int
	roundSize(b.u.n, &i)
	b.u.next = d.free[i]
	d.free[i] = b
}

func runedata(r []rune) []byte {
	var b []byte
	h := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	h.Data = uintptr(unsafe.Pointer(&r[0]))
	h.Len = runes.RuneSize * len(r)
	h.Cap = runes.RuneSize * cap(r)
	return b
}

func (d *Disk) write(bp **block, r []rune) {
	n := len(r)
	b := *bp
	size := roundSize(b.u.n, nil)
	nsize := roundSize(n, nil)
	if size != nsize {
		d.freeBlock(b)
		b = d.allocBlock(n)
		*bp = b
	}
	if nw, err := d.fd.WriteAt(runedata(r), b.addr); nw != n*runes.RuneSize || err != nil {
		if err == nil {
			err = io.ErrShortWrite
		}
		util.Fatal(fmt.Sprintf("writing temp file: %v", err))
	}
	b.u.n = n
}

func (d *Disk) read(b *block, r []rune) {
	n := len(r)
	if n > b.u.n {
		util.Fatal("internal error: diskread")
	}

	roundSize(b.u.n, nil) /* called only for sanity check on Maxblock */
	if nr, err := d.fd.ReadAt(runedata(r), b.addr); nr != n*runes.RuneSize || err != nil {
		util.Fatal("read error from temp file")
	}
}

func TempFile() *os.File {
	f, err := ioutil.TempFile("", fmt.Sprintf("acme.%d.*", os.Getpid()))
	if err != nil {
		// TODO rescue()
		log.Fatalf("creating temp file: %v", err)
	}
	return f
}
