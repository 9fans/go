package base

type Mntdir struct {
	ID   int
	Ref  int
	Dir  []rune
	Next *Mntdir
	Incl [][]rune
}
