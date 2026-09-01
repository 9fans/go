package client_test

import (
	"net"
	"sync"
	"testing"
	"time"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

// proxyServer simulates a 9P proxy (like 9pserve/acme): fids are entered
// into the server's table immediately on Twalk, but are only removed once
// the simulated back-end responds to the Tclunk.  This matches the real
// behaviour of 9pserve, which forwards requests to a back-end process and
// tracks fids on behalf of that back-end.
//
// Reading and writing run in separate goroutines so that the server is
// never blocked writing when it also needs to read — both sides writing
// simultaneously on an unbuffered net.Pipe would otherwise deadlock.
type proxyServer struct {
	conn       net.Conn
	out        chan *plan9.Fcall
	fmu        sync.Mutex
	fids       map[uint32]bool
	clunkDelay time.Duration

	// clunkSeen is closed the moment the server reads a Tclunk.
	clunkSeen chan struct{}
}

func (s *proxyServer) serve() {
	go s.sender()

	// Tversion handshake.
	f := s.read()
	if f == nil {
		return
	}
	s.send(&plan9.Fcall{Type: plan9.Rversion, Tag: plan9.NOTAG,
		Msize: f.Msize, Version: "9P2000"})

	// Tattach — give the client a root fid.
	f = s.read()
	if f == nil {
		return
	}
	s.fmu.Lock()
	s.fids[f.Fid] = true
	s.fmu.Unlock()
	s.send(&plan9.Fcall{Type: plan9.Rattach, Tag: f.Tag,
		Qid: plan9.Qid{Type: plan9.QTDIR}})

	for {
		f = s.read()
		if f == nil {
			return
		}
		switch f.Type {
		case plan9.Twalk:
			s.fmu.Lock()
			dup := s.fids[f.Newfid] && f.Newfid != f.Fid
			if !dup {
				s.fids[f.Newfid] = true
			}
			s.fmu.Unlock()
			if dup {
				s.send(&plan9.Fcall{Type: plan9.Rerror, Tag: f.Tag,
					Ename: "duplicate fid"})
				continue
			}
			wqid := make([]plan9.Qid, len(f.Wname))
			for i := range wqid {
				wqid[i] = plan9.Qid{Type: plan9.QTFILE, Path: 1}
			}
			s.send(&plan9.Fcall{Type: plan9.Rwalk, Tag: f.Tag, Wqid: wqid})

		case plan9.Topen:
			s.send(&plan9.Fcall{Type: plan9.Ropen, Tag: f.Tag,
				Qid: plan9.Qid{Type: plan9.QTFILE, Path: 1}})

		case plan9.Tclunk:
			tag, fid := f.Tag, f.Fid
			// Signal the test that the Tclunk has been received.
			select {
			case <-s.clunkSeen:
			default:
				close(s.clunkSeen)
			}
			// Simulate back-end latency: keep fid in the table until
			// the back-end would have replied.
			go func() {
				time.Sleep(s.clunkDelay)
				s.fmu.Lock()
				delete(s.fids, fid)
				s.fmu.Unlock()
				s.send(&plan9.Fcall{Type: plan9.Rclunk, Tag: tag})
			}()

		default:
			s.send(&plan9.Fcall{Type: plan9.Rerror, Tag: f.Tag,
				Ename: "not supported"})
		}
	}
}

func (s *proxyServer) sender() {
	for f := range s.out {
		if err := plan9.WriteFcall(s.conn, f); err != nil {
			return
		}
	}
}

func (s *proxyServer) read() *plan9.Fcall {
	f, err := plan9.ReadFcall(s.conn)
	if err != nil {
		return nil
	}
	return f
}

func (s *proxyServer) send(f *plan9.Fcall) { s.out <- f }

// TestFidRecycle is a regression test for a bug where rpc() recycled a fid
// number into freefid as soon as Tclunk was written to the wire, before
// the server's Rclunk was received.  Proxy servers such as 9pserve keep
// fids in their own table until the back-end process responds to the
// Tclunk; if the client reused the fid number for a concurrent Twalk
// during that window, the proxy detected the collision and returned
// "duplicate fid".
func TestFidRecycle(t *testing.T) {
	c1, c2 := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })

	srv := &proxyServer{
		conn:       c2,
		out:        make(chan *plan9.Fcall, 64),
		fids:       make(map[uint32]bool),
		clunkDelay: 5 * time.Millisecond,
	}
	go srv.serve()

	conn, err := client.NewConn(c1)
	if err != nil {
		t.Fatal(err)
	}
	fs, err := conn.Attach(nil, "nobody", "")
	if err != nil {
		t.Fatal(err)
	}

	// Each round:
	//   1. Open a file (allocating fid F).
	//   2. Close it in a goroutine, which sends Tclunk(F).
	//   3. Wait until the proxy has received the Tclunk, then immediately
	//      open another file.  The proxy still has F in its table at this
	//      point (the back-end hasn't replied yet).  If the client reuses
	//      F as the Newfid for the new Walk, the proxy returns "duplicate
	//      fid".
	const rounds = 20
	var dupFids int
	for i := 0; i < rounds; i++ {
		srv.clunkSeen = make(chan struct{})

		fid, err := fs.Open("file", plan9.OWRITE)
		if err != nil {
			t.Fatalf("round %d open: %v", i, err)
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			fid.Close()
		}()

		<-srv.clunkSeen // proxy has seen Tclunk; back-end has not yet replied

		fid2, err := fs.Open("file", plan9.OREAD)
		if err != nil {
			if err.Error() == "duplicate fid" {
				dupFids++
			} else {
				t.Logf("round %d open2: %v", i, err)
			}
		} else {
			fid2.Close()
		}

		wg.Wait()
	}

	if dupFids > 0 {
		t.Errorf("got %d 'duplicate fid' errors in %d rounds: "+
			"fid number recycled before server Rclunk", dupFids, rounds)
	}
}
