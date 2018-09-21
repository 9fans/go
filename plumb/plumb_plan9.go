package plumb

import (
	"fmt"
	"os"
	"strings"

	"9fans.net/go/plan9/client"
)

func mountPlumb() {
	name := os.Getenv("plumbsrv")
	if name == "" {
		fsysErr = fmt.Errorf("$plumbsrv not set")
		return
	}
	p := "/srv/"
	if strings.HasPrefix(name, p) {
		name = name[len(p):]
	}
	fsys, fsysErr = client.MountService(name)
}
