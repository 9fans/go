// +build !plan9

package acme

import "9fans.net/go/plan9/client"

func mountAcme() {
	fs, err := client.MountService("acme")
	fsys = fs
	fsysErr = err
}
