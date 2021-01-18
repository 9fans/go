package main

func rinit(r *Rasp) {
	r.nrunes = 0
	r.sect = nil
}

func free(interface{}) {}

func rclear(r *Rasp) {
	var ns *Section
	for s := r.sect; s != nil; s = ns {
		ns = s.next
		free(s.text)
		free(s)
	}
	r.sect = nil
}

func rsinsert(r *Rasp, s *Section) *Section {
	t := new(Section)
	if r.sect == s { /* includes empty list case: r->sect==s==0 */
		r.sect = t
		t.next = s
	} else {
		u := r.sect
		if u == nil {
			panic("rsinsert 1")
		}
		for {
			if u.next == s {
				t.next = s
				u.next = t
				goto Return
			}
			u = u.next
			if u == nil {
				break
			}
		}
		panic("rsinsert 2")
	}
Return:
	return t
}

func rsdelete(r *Rasp, s *Section) {
	if s == nil {
		panic("rsdelete")
	}
	if r.sect == s {
		r.sect = s.next
		goto Free
	}
	for t := r.sect; t != nil; t = t.next {
		if t.next == s {
			t.next = s.next
			goto Free
		}
	}
	panic("rsdelete 2")

Free:
	if s.text != nil {
		free(s.text)
	}
	free(s)

}

func splitsect(r *Rasp, s *Section, n0 int) {
	if s == nil {
		panic("splitsect")
	}
	rsinsert(r, s.next)
	if s.text == nil {
		s.next.text = nil
	} else {
		s.next.text = make([]rune, TBLOCKSIZE)
		copy(s.next.text, s.text[n0:s.nrunes])
	}
	s.next.nrunes = s.nrunes - n0
	s.nrunes = n0
}

func findsect(r *Rasp, s *Section, p int, q int) *Section {
	if s == nil && p != q {
		panic("findsect")
	}
	for ; s != nil && p+s.nrunes <= q; s = s.next {
		p += s.nrunes
	}
	if p != q {
		splitsect(r, s, q-p)
		s = s.next
	}
	return s
}

func rresize(r *Rasp, a int, old int, new int) {
	s := findsect(r, r.sect, 0, a)
	t := findsect(r, s, a, a+old)
	var ns *Section
	for ; s != t; s = ns {
		ns = s.next
		rsdelete(r, s)
	}
	/* now insert the new piece before t */
	if new > 0 {
		ns = rsinsert(r, t)
		ns.nrunes = new
		ns.text = nil
	}
	r.nrunes += new - old
}

func rdata(r *Rasp, p0 int, p1 int, cp []rune) {
	s := findsect(r, r.sect, 0, p0)
	t := findsect(r, s, p0, p1)
	var ns *Section
	for ; s != t; s = ns {
		ns = s.next
		if s.text != nil {
			panic("rdata")
		}
		rsdelete(r, s)
	}
	p1 -= p0
	s = rsinsert(r, t)
	s.text = make([]rune, TBLOCKSIZE)
	copy(s.text, cp[:p1])
	s.nrunes = p1
}

func rclean(r *Rasp) {
	for s := r.sect; s != nil; s = s.next {
		for s.next != nil && (s.text != nil) == (s.next.text != nil) {
			if s.text != nil {
				if s.nrunes+s.next.nrunes > TBLOCKSIZE {
					break
				}
				copy(s.text[s.nrunes:s.nrunes+s.next.nrunes], s.next.text[:s.next.nrunes])
			}
			s.nrunes += s.next.nrunes
			rsdelete(r, s.next)
		}
	}
}

var raspbuf []rune

func rload(r *Rasp, p0 int, p1 int) []rune {
	for cap(raspbuf) < p1-p0 {
		raspbuf = append(raspbuf[:cap(raspbuf)], 0)
	}

	p := 0
	s := r.sect
	for ; s != nil && p+s.nrunes <= p0; s = s.next {
		p += s.nrunes
	}
	nb := 0
	for p < p1 && s != nil {
		/*
		 * Subtle and important.  If we are preparing to handle an 'rdata'
		 * call, it's because we have an 'rresize' hole here, so the
		 * screen doesn't have data for that space anyway (it got cut
		 * first).  So pretend it isn't there.
		 */
		if s.text != nil {
			n := s.nrunes - (p0 - p)
			if n > p1-p0 { /* all in this section */
				n = p1 - p0
			}
			copy(raspbuf[nb:nb+n], s.text[p0-p:])
			nb += n
		}
		p += s.nrunes
		p0 = p
		s = s.next
	}
	return raspbuf[:nb]
}

func rmissing(r *Rasp, p0 int, p1 int) int {
	p := 0
	s := r.sect
	for ; s != nil && p+s.nrunes <= p0; s = s.next {
		p += s.nrunes
	}
	nm := 0
	for p < p1 && s != nil {
		if s.text == nil {
			n := s.nrunes - (p0 - p)
			if n > p1-p0 { /* all in this section */
				n = p1 - p0
			}
			nm += n
		}
		p += s.nrunes
		p0 = p
		s = s.next
	}
	return nm
}

func rcontig(r *Rasp, p0 int, p1 int, text bool) int {
	p := 0
	s := r.sect
	for ; s != nil && p+s.nrunes <= p0; s = s.next {
		p += s.nrunes
	}
	np := 0
	for p < p1 && s != nil && text == (s.text != nil) {
		n := s.nrunes - (p0 - p)
		if n > p1-p0 { /* all in this section */
			n = p1 - p0
		}
		np += n
		p += s.nrunes
		p0 = p
		s = s.next
	}
	return np
}
