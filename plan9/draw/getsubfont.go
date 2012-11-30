package draw

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func getsubfont(d *Display, name string) (*Subfont, error) {
	data, err := ioutil.ReadFile(name)
	if err != nil && strings.HasPrefix(name, "/mnt/font/") {
		data1, err1 := fontPipe(name[len("/mnt/font/"):])
		if err1 == nil {
			data, err = data1, err1
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "getsubfont: %v\n", err)
		return nil, err
	}
	/*
	 * unlock display so i/o happens with display released, unless
	 * user is doing his own locking, in which case this could break things.
	 * _getsubfont is called only from string.c and stringwidth.c,
	 * which are known to be safe to have this done.
	 */
	dolock := d != nil && d.locking
	if dolock {
		//unlockdisplay(d)
	}
	f, err := d.ReadSubfont(name, bytes.NewReader(data), dolock)
	if dolock {
		//lockdisplay(d);
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "getsubfont: can't read %s: %v\n", name, err)
	}
	return f, err
}
