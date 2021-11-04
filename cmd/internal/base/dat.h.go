package base

type Mntdir struct {
	ID   int
	Ref  int
	Dir  []rune
	Next *Mntdir
	// Incl holds include directories as added by the Incl command.
	// This is present so that warning messages issued by
	// commands can create windows that have the correct
	// include directories (see ../../acme/acme.go:/<-exec.Ccommand)
	Incl [][]rune
}
