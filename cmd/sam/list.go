// #include "sam.h"

// +build ignore

package main

/*
 * Check that list has room for one more element.
 */
func growlist(l *List, esize int) {
	if l.listptr == nil || l.nalloc == 0 {
		l.nalloc = INCR
		l.listptr = emalloc(INCR * esize)
		l.nused = 0
	} else if l.nused == l.nalloc {
		p := erealloc(l.listptr, (l.nalloc+INCR)*esize)
		l.listptr = p
		memset(p+l.nalloc*esize, 0, INCR*esize)
		l.nalloc += INCR
	}
}

/*
 * Remove the ith element from the list
 */
func dellist(l *List, i int) {
	l.nused--
	var pp *Posn
	var vpp **[0]byte

	switch l.type_ {
	case 'P':
		pp = l.posnptr + i
		memmove(pp, pp+1, (l.nused-i)*sizeof(*pp))
	case 'p':
		vpp = l.voidpptr + i
		memmove(vpp, vpp+1, (l.nused-i)*sizeof(*vpp))
	}
}

/*
 * Add a new element, whose position is i, to the list
 */
func inslist(l *List, i int, args ...interface{}) {
	var list va_list

	va_start(list, i)
	var pp *Posn
	var vpp **[0]byte
	switch l.type_ {
	case 'P':
		growlist(l, sizeof(*pp))
		pp = l.posnptr + i
		memmove(pp+1, pp, (l.nused-i)*sizeof(*pp))
		*pp = va_arg(list, Posn)
	case 'p':
		growlist(l, sizeof(*vpp))
		vpp = l.voidpptr + i
		memmove(vpp+1, vpp, (l.nused-i)*sizeof(*vpp))
		*vpp = va_arg(list, *[0]byte)
	}
	va_end(list)

	l.nused++
}

func listfree(l *List) {
	free(l.listptr)
	free(l)
}

func listalloc(type_ int) *List {
	l := emalloc(sizeof(List))
	l.type_ = type_
	l.nalloc = 0
	l.nused = 0

	return l
}
