package srv9p

// hasPerm does simplistic permission checking.
// It assumes that each user is the leader of her own group.
func hasPerm(f *File, uid string, perm int) bool {
	m := int(f.Stat.Mode) // other
	if perm&m == perm {
		return true
	}

	if f.Stat.Uid == uid {
		m |= m >> 6
		if perm&m == perm {
			return true
		}
	}

	if f.Stat.Gid == uid {
		m |= m >> 3
		if perm&m == perm {
			return true
		}
	}

	return false
}
