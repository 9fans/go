package srv9p

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"9fans.net/go/plan9"
)

// A request represents a pending request on a 9P connection
type request struct {
	// immutable fields
	conn      *conn
	ctx       context.Context
	cancel    func()
	ifcall    *plan9.Fcall
	ofcall    *plan9.Fcall
	duplicate bool
	err       error

	// mutable fields
	mu        sync.Mutex
	responded atomic.Bool
	flush     []*request
}

// newReq allocates a new request in the conn c for the 9P request f.
// If there is already a request with that tag, the returned request has
// r.duplicate and r.err set, and the caller is expected to call r.respond rather
// than process it.
func (c *conn) newReq(f *plan9.Fcall) *request {
	ctx, cancel := context.WithCancel(context.Background())
	r := &request{
		conn:   c,
		ctx:    ctx,
		cancel: cancel,
		ifcall: f,
		ofcall: new(plan9.Fcall),
	}
	if !c.reqs.tryInsert(f.Tag, r) {
		r.duplicate = true
		r.err = errors.New("duplicate tag")
		return r
	}
	return r
}

// incRef and decRef are no-ops.
// They are implemented only so that request can be used with refMap.
func (r *request) incRef() {}
func (r *request) decRef() {}
