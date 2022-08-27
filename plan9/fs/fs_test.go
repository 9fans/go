//go:build u9fs && linux

package fs

import (
	"flag"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"

	"9fans.net/go/plan9/client"
	"golang.org/x/sys/unix"
)

// tests an io.FS backedup by a 9p server. Needs u9fs https://github.com/unofficial-mirror/u9fs
// Use the root flag to test any folder, for example
// go test -tags u9fs -root /home/user/images -exp 'gopher.png,glenda.png' -timeout 0

var root = flag.String("root", "./testdata", "the root tree to check")
var exp = flag.String("exp", "fortunes.txt", "a comma separated list of files expected to find in root tree")

func TestFS(t *testing.T) {
	execPath, err := exec.LookPath("u9fs")
	check(t, err, "u9fs is not in PATH")

	// create a socket pair to connect the 9p client with a u9fs serving root
	fds, err := unix.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	check(t, err, "failed to create socket pair")
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	user := os.Getenv("USER")

	// first init the server
	srv := os.NewFile(uintptr(fds[1]), "srv")
	cmd := exec.Cmd{
		Path: execPath,
		Args: []string{"u9fs", "-u", user, "-n", "-a", "none",
			"-l", path.Join(t.TempDir(), "u9fs.log"), *root},
		Stdin:  srv,
		Stdout: srv,
	}
	check(t, cmd.Start(), "failed to start u9fs")
	defer cmd.Process.Kill()

	// init the client last because server must be up to read Tversion
	cli := os.NewFile(uintptr(fds[0]), "cli")
	conn, err := client.NewConn(cli)
	check(t, err, "failed to create client")
	fsys, err := conn.Attach(nil, user, "")
	check(t, err, "failed to attach client")

	// create and check the filesystem
	err = fstest.TestFS(NewFS(fsys), strings.Split(*exp, ",")...)
	check(t, err, "FS test failed")
}

func check(t *testing.T, err error, msg string) {
	if err != nil {
		t.Fatalf("%s: %s", msg, err)
	}
}
