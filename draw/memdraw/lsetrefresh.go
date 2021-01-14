package memdraw

func LSetRefresh(i *Image, fn Refreshfn, ptr interface{}) bool {
	l := i.Layer
	if l.Refreshfn != nil && fn != nil { /* just change functions */
		l.Refreshfn = fn
		l.Refreshptr = ptr
		return true
	}

	if l.Refreshfn == nil { /* is using backup image; just free it */
		Free(l.save)
		l.save = nil
		l.Refreshfn = fn
		l.Refreshptr = ptr
		return true
	}

	var err error
	l.save, err = AllocImage(i.R, i.Pix)
	if err != nil {
		return false
	}
	/* easiest way is just to update the entire save area */
	l.Refreshfn(i, i.R, l.Refreshptr)
	l.Refreshfn = nil
	l.Refreshptr = nil
	return true
}
