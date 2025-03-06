package srv9p

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"9fans.net/go/plan9"
)

const timeout = 1 * time.Second

func TestTrace(t *testing.T) {
	files, err := filepath.Glob("testdata/*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			lineno := 0
			var rc io.ReadCloser
			var wc io.WriteCloser
			lines := strings.Split(string(data), "\n")
			for lineno := 0; lineno < len(lines); lineno++ {
				pos := fmt.Sprintf("%s:%d", file, lineno+1)
				line := strings.TrimSpace(lines[lineno])
				if strings.HasPrefix(line, "#") || line == "" {
					continue
				}
				verb, rest, _ := strings.Cut(line, " ")
				switch {
				default:
					t.Fatalf("%s: unknown verb: %s", pos, verb)
				case verb == "serve":
					if rc != nil {
						t.Fatalf("%s: already serving", pos)
					}
					newServer, ok := servers[strings.TrimSpace(rest)]
					if !ok {
						t.Fatalf("%s: unknown server", pos)
					}
					rc, wc = runServer(t, newServer(t))
				//case verb == "log":
				//	expect log line
				case verb[0] == 'T':
					if rc == nil {
						t.Fatalf("%s: not serving", pos)
					}
					f, err := plan9.ParseFcall(line)
					if err != nil {
						t.Fatalf("%s: parsing request: %v", pos, err)
					}
					msg, err := f.Bytes()
					if err != nil {
						t.Fatalf("%s: marshaling fcall: %v", pos, err)
					}
					if _, err := wc.Write(msg); err != nil {
						t.Fatalf("%s: writing fcall: %v", pos, err)
					}
				case verb[0] == 'R':
					// Collect list of responses we want.
					var want []*plan9.Fcall
					for ; lineno < len(lines) && strings.HasPrefix(lines[lineno], "R"); lineno++ {
						pos := fmt.Sprintf("%s:%d", file, lineno+1)
						line := strings.TrimSpace(lines[lineno])
						f, err := plan9.ParseFcall(line)
						if err != nil {
							t.Fatalf("%s: parsing response: %v", pos, err)
						}
						want = append(want, f)
					}
					lineno-- // for loop will lineno++

					// Read responses we hope for.
					var extra []*plan9.Fcall
				Read:
					for range want {
						rf, err := testReadFcall(t, rc)
						if err != nil {
							t.Fatalf("%s: %v", pos, err)
						}
						for i, f := range want {
							if reflect.DeepEqual(rf, f) {
								want[i] = nil
								continue Read
							}
						}
						extra = append(extra, rf)
					}
					if len(extra) == 0 {
						continue
					}
					if len(want) == 1 && len(extra) == 1 && want[0] != nil {
						t.Fatalf("%s: reply mismatch:\nhave %s\nwant %s", pos, extra[0], want[0])
					}
					var buf bytes.Buffer
					fmt.Fprintf(&buf, "%s: reply mismatch:\nhave:", pos)
					for _, f := range extra {
						if f != nil {
							fmt.Fprintf(&buf, "\n%s", f)
						}
					}
					fmt.Fprintf(&buf, "\nwant:")
					for _, f := range want {
						if f != nil {
							fmt.Fprintf(&buf, "\n%s", f)
						}
					}
					t.Fatal(buf.String())
				}
			}
			lineno++
			pos := fmt.Sprintf("%s:%d", file, lineno)
			if rc == nil {
				return
			}
			wc.Close()
			f, err := testReadFcall(t, rc)
			if err == nil {
				t.Fatalf("%s: unexpected response after script end:\n%s", pos, f)
			}
			if err != io.EOF {
				t.Fatalf("%s: unexpected error after script end (want EOF): %v", pos, err)
			}
		})
	}
}

func testReadFcall(t *testing.T, r io.Reader) (*plan9.Fcall, error) {
	const timeout = 5 * time.Second
	done := make(chan bool, 1)
	var f *plan9.Fcall
	var err error
	go func() {
		f, err = plan9.ReadFcall(r)
		done <- true
	}()
	select {
	case <-done:
		return f, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timed out reading reply")
	}
}

func runServer(t *testing.T, srv *Server) (io.ReadCloser, io.WriteCloser) {
	r1, w1, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		srv.Serve(r2, w1)
		r2.Close()
		w1.Close()
	}()
	return r1, w2
}

var servers = map[string]func(*testing.T) *Server{
	"tree":  treeServer,
	"pipe":  pipeServer,
	"ramfs": ramfsServer,
}

func treeServer(t *testing.T) *Server {
	tree := NewTree("glenda", "bunny", plan9.DMDIR|0755, nil)
	tree.Root.Stat.Mtime = 123456789
	tree.Root.Stat.Atime = 200000000
	f, err := tree.Root.Create("hello", "gopher", 0644, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.Aux = []byte("hello, go nuts")
	f.Stat.Mtime = 123456789
	f.Stat.Atime = 200000000
	f, err = tree.Root.Create("slang", "gopher", plan9.DMDIR|0555, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.Stat.Mtime = 123456789
	f.Stat.Atime = 200000000
	c, err := f.Create("hi", "tor", 0444, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Aux = []byte("hello, 9fans")
	c.Stat.Mtime = 123456789
	c.Stat.Atime = 200000000

	srv := &Server{
		Tree: tree,
	}
	srv.Read = func(ctx context.Context, fid *Fid, b []byte, offset int64) (int, error) {
		data, _ := fid.File().Aux.([]byte)
		return fid.ReadBytes(b, offset, data)
	}
	return srv
}

func pipeServer(t *testing.T) *Server {
	const (
		qidDir = iota
		qidRead
		qidWrite
	)

	dirgen := func(i int, d *plan9.Dir) bool {
		switch i {
		default:
			return false
		case -1:
			d.Name = "/"
			d.Qid = plan9.Qid{Path: qidDir, Type: plan9.QTDIR}
			d.Mode = 0555 | plan9.DMDIR
		case 0:
			d.Name = "read"
			d.Qid = plan9.Qid{Path: qidRead}
			d.Mode = 0444
		case 1:
			d.Name = "write"
			d.Qid = plan9.Qid{Path: qidWrite}
			d.Mode = 0222
		}
		d.Uid = "pipe"
		d.Gid = "band"
		return true
	}

	type reply struct {
		n   int
		err error
	}
	type write struct {
		data  []byte
		reply chan reply
	}
	pipe := make(chan write)

	srv := &Server{
		Attach: func(ctx context.Context, fid, afid *Fid, user, aname string) (plan9.Qid, error) {
			return plan9.Qid{Path: qidDir, Type: plan9.QTDIR}, nil
		},
		Walk: func(ctx context.Context, fid, newfid *Fid, names []string) ([]plan9.Qid, error) {
			return Walk(fid, newfid, names, nil, func(fid *Fid, name string) (plan9.Qid, error) {
				switch name {
				case "read":
					return plan9.Qid{Path: qidRead}, nil
				case "write":
					return plan9.Qid{Path: qidWrite}, nil
				}
				return plan9.Qid{}, errNotFound
			})
		},
		Open: func(ctx context.Context, fid *Fid, mode uint8) error {
			qid := fid.Qid()
			if qid.Path == 1 && mode != plan9.OREAD ||
				qid.Path == 2 && mode != plan9.OWRITE {
				return errPerm
			}
			return nil
		},
		Stat: func(ctx context.Context, fid *Fid) (*plan9.Dir, error) {
			d := new(plan9.Dir)
			dirgen(int(fid.Qid().Path)-1, d)
			return d, nil
		},
		Read: func(ctx context.Context, fid *Fid, data []byte, offset int64) (int, error) {
			if fid.Qid().Path == qidDir {
				println("dir read")
				return fid.ReadDir(data, offset, dirgen)
			}
			// must be qidRead
			select {
			case <-ctx.Done():
				return 0, fmt.Errorf("flushed")
			case w := <-pipe:
				println("matched")
				n := copy(data, w.data)
				var err error
				if n < len(w.data) {
					err = fmt.Errorf("short write")
				}
				w.reply <- reply{n, err}
				return n, err
			}
		},
		Write: func(ctx context.Context, fid *Fid, data []byte, offset int64) (int, error) {
			// must be qidWrite
			w := write{data: data, reply: make(chan reply, 1)}
			select {
			case <-ctx.Done():
				return 0, fmt.Errorf("flushed")
			case pipe <- w:
				println("matched")
				reply := <-w.reply
				return reply.n, reply.err
			}
		},
	}

	return srv
}

func ramfsServer(t *testing.T) *Server {
	type ramFile struct {
		data []byte
	}

	srv := &Server{
		Tree: NewTree("ram", "ram", plan9.DMDIR|0777, nil),
		Open: func(ctx context.Context, fid *Fid, mode uint8) error {
			if mode&plan9.OTRUNC != 0 {
				rf := fid.File().Aux.(*ramFile)
				rf.data = nil
			}
			return nil
		},
		Create: func(ctx context.Context, fid *Fid, name string, perm plan9.Perm, mode uint8) (plan9.Qid, error) {
			f, err := fid.File().Create(name, "ram", perm, new(ramFile))
			if err != nil {
				return plan9.Qid{}, err
			}
			fid.SetFile(f)
			return f.Stat.Qid, nil
		},
		Read: func(ctx context.Context, fid *Fid, data []byte, offset int64) (int, error) {
			rf := fid.File().Aux.(*ramFile)
			return fid.ReadBytes(data, offset, rf.data)
		},
		Write: func(ctx context.Context, fid *Fid, data []byte, offset int64) (int, error) {
			rf := fid.File().Aux.(*ramFile)
			if int64(int(offset)) != offset || int(offset)+len(data) < 0 {
				return 0, ErrBadOffset
			}
			end := int(offset) + len(data)
			if len(rf.data) < end {
				rf.data = slices.Grow(rf.data, end-len(rf.data))
				rf.data = rf.data[:end]
			}
			copy(rf.data[offset:], data)
			return len(data), nil
		},
	}

	return srv
}
