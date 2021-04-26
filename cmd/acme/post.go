package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"9fans.net/go/plan9/client"
)

var chattyfuse int

func post9pservice(rfd, wfd *os.File, name, mtpt string) error {
	if name == "" && mtpt == "" {
		rfd.Close()
		wfd.Close()
		return fmt.Errorf("nothing to do")
	}

	if name != "" {
		var addr string
		if strings.Contains(addr, "!") { // assume is already network address
			addr = name
		} else {
			addr = "unix!" + client.Namespace() + "/" + name
		}
		cmd := exec.Command("9pserve", "-u", addr)
		cmd.Stdin = rfd
		cmd.Stdout = wfd
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return err
		}
		if mtpt != "" {
			// reopen
			log.Fatalf("post9pservice mount not implemented")
		}
	}
	if mtpt != "" {
		log.Fatalf("post9pservice mount not implemented")
	}
	return nil
}
