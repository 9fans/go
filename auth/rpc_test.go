// Copyright 2012 The go-plan9-auth Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"testing"
)

func testUserPass(t *testing.T, getUP func(*control, string, string) (string, string, error)) {
	user, pass := "gopher", "gorocks"
	params := "dom=testing.golang.org proto=pass role=client"
	key := params + fmt.Sprintf(" user=%s !password=%s", user, pass)

	ctl, err := newControl()
	if err != nil {
		t.Fatalf("open factotum/ctl: %v", err)
	}
	defer ctl.Close()

	user1, pass1, err := getUP(ctl, params, key)
	if err != nil {
		t.Errorf("GetUserPassword failed: %v\n", err)
	}
	if user1 != user || pass1 != pass {
		t.Errorf("GetUserPassword gave user=%s !password=%s; want user=%s !password=%s\n", user1, pass1, user, pass)
	}

	if err := ctl.DeleteKey(params); err != nil {
		t.Errorf("DeleteKey failed: %v\n", err)
	}
}

func TestGetUserPassword(t *testing.T) {
	testUserPass(t, func(ctl *control, params, key string) (string, string, error) {
		if err := ctl.AddKey(key); err != nil {
			t.Fatalf("AddKey failed: %v\n", err)
		}
		return GetUserPassword(nil, params)
	})
}

func TestGetUserPassword1(t *testing.T) {
	testUserPass(t, func(ctl *control, params, key string) (string, string, error) {
		return GetUserPassword(func(string) error {
			return ctl.AddKey(key)
		}, params)
	})
}
