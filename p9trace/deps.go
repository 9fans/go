// This file is not ignored by 'go mod tidy',
// to keep leveldb as a required module.

//go:build never

package p9trace

import _ "github.com/golang/leveldb/table"
