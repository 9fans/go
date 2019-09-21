// Copyright 2012 The go-plan9-auth Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"os/exec"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

// GetKeyFunc takes an attribute-value-list as argument.
// GetKey is an example implementation.
type GetKeyFunc func(string) error

// GetKey prompts the user for missing key information and
// passes the information to factotum. Params is an
// attribute-value-list.
func GetKey(params string) error {
	path := "/boot/factotum"
	if _, err := exec.LookPath(path); err != nil {
		// Probably using Plan 9 Port
		path = "factotum"
	}
	return exec.Command(path, "-g", params).Run()
}

type control struct {
	f *client.Fid
}

func newControl() (*control, error) {
	fsys, err := client.MountService("factotum")
	if err != nil {
		return nil, err
	}
	fid, err := fsys.Open("ctl", plan9.ORDWR)
	if err != nil {
		return nil, err
	}
	return &control{f: fid}, nil
}

func (c *control) Close() error {
	return c.f.Close()
}

func (c *control) AddKey(params string) error {
	_, err := c.f.Write([]byte("key " + params))
	return err
}

func (c *control) DeleteKey(params string) error {
	_, err := c.f.Write([]byte("delkey " + params))
	return err
}
