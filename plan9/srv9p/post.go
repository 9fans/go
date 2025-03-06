//go:build plan9

package srv9p

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Post posts one end of a pipe at /srv/name and returns the other end
// for use as both directions in a call to srv.Serve.
// The returned file is close-on-exec.
func Post(name string) (*os.File, error) {
	var fd [2]int
	err := syscall.Pipe(fd[:])
	if err != nil {
		return nil, fmt.Errorf("pipe: %v", err)
	}

	// Dup fd[1] close-on-exec.
	cfd, err := syscall.Open(fmt.Sprintf("#d/%d", fd[1]), syscall.O_RDWR|syscall.O_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("reopen close-on-write: %v", err)
	}
	syscall.Close(fd[1])
	fd[1] = cfd

	const O_RCLOSE = 64
	sfd, err := syscall.Create("/srv/"+name, syscall.O_WRONLY|O_RCLOSE|syscall.O_CLOEXEC, 0600)
	if err != nil {
		syscall.Close(fd[0])
		syscall.Close(fd[1])
		return nil, fmt.Errorf("create /srv/%s: %v", name, err)
	}
	_, err = syscall.Write(sfd, []byte(fmt.Sprint(fd[0])))
	syscall.Close(fd[0])
	if err != nil {
		syscall.Close(fd[1])
		return nil, fmt.Errorf("write /srv/%s: %v", name, err)
	}

	return os.NewFile(uintptr(fd[1]), "/srv/"+name), nil
}

func PostMountServe(srvname, mtpt string, flags int, args []string, server func() *Server) {
	// In Plan 9 C, it was easy to use fork with the right flags
	// to fork a server into the background and exit.
	// The Go runtime makes this too hard, so instead we fork a child process
	// to post the service and run the server.
	// and wait for it to start up.
	if name := os.Getenv("_SRV9P_CHILD_PROC_"); name != "" {
		// We are the child.
		rw, err := Post(name)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("ok\n")
		server().Serve(rw, rw)
		log.Fatal("exiting")
		return
	}

	// Start the child process.
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	name := srvname
	if name == "" {
		// Make up a temporary name for the child process to use,
		// so we can open the service file descriptor.
		name = fmt.Sprintf("srv9p.%d.%d", os.Getpid(), time.Now().UnixNano())
	}
	cmd := exec.Command(exe, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	null, err := os.Open("/dev/null")
	if err != nil {
		log.Fatal(err)
	}
	cmd.Stdin = null
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "_SRV9P_CHILD_PROC_="+name)
	cmd.SysProcAttr = &syscall.SysProcAttr{Rfork: syscall.RFNOTEG | syscall.RFNAMEG}
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	buf := make([]byte, 10)
	n, _ := stdout.Read(buf)
	if n != 3 || string(buf[:3]) != "ok\n" {
		// Assume child printed problem to standard error.
		log.Fatal("child process failure")
	}

	if mtpt != "" {
		fd, err := syscall.Open("/srv/"+name, syscall.O_RDWR)
		if err != nil {
			log.Fatalf("open /srv/%s: %v", name, err)
		}
		if srvname == "" {
			syscall.Remove("/srv/" + name)
		}
		if err := syscall.Mount(fd, -1, mtpt, flags, ""); err != nil {
			log.Fatalf("mount %s: %v", mtpt, err)
		}
		syscall.Close(fd)
	}
}
