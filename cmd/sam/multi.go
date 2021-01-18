// #include "sam.h"

package main

import "path/filepath"

var file []*File
var tag int

func newfile() *File {
	f := fileopen()
	file = append(file, f)
	f.tag = tag
	tag++
	if downloaded {
		outTs(Hnewname, f.tag)
	}
	/* already sorted; file name is "" */
	return f
}

func whichmenu(f *File) int {
	for i := range file {
		if file[i] == f {
			return i
		}
	}
	return -1
}

func delfile(f *File) {
	w := whichmenu(f)
	if w < 0 { /* e.g. x/./D */
		return
	}
	if downloaded {
		outTs(Hdelname, f.tag)
	}
	delfilelist(w)
	fileclose(f)
}

func delfilelist(w int) {
	copy(file[w:], file[w+1:])
	file = file[:len(file)-1]
}

func fullname(name *String) {
	debug("curwd %v", &curwd)
	if len(name.s) > 0 && name.s[0] != '/' && name.s[0] != 0 {
		Strinsert(name, &curwd, Posn(0))
		debug("post %v", name)
	}
}

func fixname(name *String) {
	debug("fixnmae %s\n", name)
	fullname(name)
	debug("fixfull %s\n", name)
	s := Strtoc(name)
	if len(s) > 0 {
		s = filepath.Clean(s)
	}
	t := tmpcstr(s)
	Strduplstr(name, t)
	// free(s)
	freetmpstr(t)

	if Strispre(&curwd, name) {
		Strdelete(name, 0, len(curwd.s))
	}
}

func sortname(f *File) {
	w := whichmenu(f)
	dupwarned := false
	delfilelist(w)
	var i int
	if f == cmd {
		i = 0
	} else {
		for i = 0; i < len(file); i++ { // NOT range - must end with i = len(file)
			cmp := Strcmp(&f.name, &file[i].name)
			if cmp == 0 && !dupwarned {
				dupwarned = true
				warn_S(Wdupname, &f.name)
			} else if cmp < 0 && (i > 0 || cmd == nil) {
				break
			}
		}
	}
	insfilelist(i, f)
	if downloaded {
		outTsS(Hmovname, f.tag, &f.name)
	}
}

func insfilelist(i int, f *File) {
	file = append(file, nil)
	copy(file[i+1:], file[i:])
	file[i] = f
}

func state(f *File, cleandirty State) {
	if f == cmd {
		return
	}
	f.unread = false
	if downloaded && whichmenu(f) >= 0 { /* else flist or menu */
		if f.mod && cleandirty != Dirty {
			outTs(Hclean, f.tag)
		} else if !f.mod && cleandirty == Dirty {
			outTs(Hdirty, f.tag)
		}
	}
	if cleandirty == Clean {
		f.mod = false
	} else {
		f.mod = true
	}
}

func lookfile(s *String) *File {
	for _, f := range file {
		if Strcmp(&f.name, s) == 0 {
			return f
		}
	}
	return nil
}
