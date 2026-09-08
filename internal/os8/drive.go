// Package os8 provides a read-only OS/8 filesystem on SIMH PDP-8 DECtape.
package os8

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"time"
)

type Drive struct {
	source  string
	raw     []byte
	entries map[string]entry
}

func Open(source string) (*Drive, error) {
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// Standard SIMH PDP-8 DECtape geometry; smaller complete test volumes allowed.
	if !st.Mode().IsRegular() || st.Size() == 0 || st.Size() > 1474*physicalBytes {
		return nil, fmt.Errorf("invalid SIMH TU56 image size or type")
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	blocks, err := logicalBlocks(raw)
	if err != nil {
		return nil, err
	}
	d := &Drive{source: source, raw: raw, entries: map[string]entry{}}
	seen := map[int]bool{}
	used := map[int]bool{}
	for n := 1; n != 0; {
		if seen[n] {
			return nil, fmt.Errorf("directory cycle at block %d", n)
		}
		seen[n] = true
		s, err := parseSegment(blocks, n)
		if err != nil {
			return nil, err
		}
		for _, e := range s.Entries {
			for b := e.Start; b < e.Start+e.Length; b++ {
				if used[b] {
					return nil, fmt.Errorf("overlapping allocation at block %d", b)
				}
				used[b] = true
			}
			if e.Name == "<FREE>" {
				continue
			}
			if _, ok := d.entries[e.Name]; ok {
				return nil, fmt.Errorf("duplicate filename %s", e.Name)
			}
			d.entries[e.Name] = e
		}
		n = s.Next
	}
	return d, nil
}
func (d *Drive) Close() error        { d.raw = nil; return nil }
func (d *Drive) Description() string { return fmt.Sprintf("tu56:%s (ro, SIMH OS/8)", d.source) }
func (d *Drive) lookup(name string) (entry, error) {
	if d.raw == nil {
		return entry{}, fs.ErrClosed
	}
	if !fs.ValidPath(name) {
		return entry{}, &fs.PathError{Op: "lookup", Path: name, Err: fs.ErrInvalid}
	}
	e, ok := d.entries[name]
	if !ok {
		return entry{}, &fs.PathError{Op: "lookup", Path: name, Err: fs.ErrNotExist}
	}
	return e, nil
}
func (d *Drive) Stat(name string) (fs.FileInfo, error) {
	if d.raw == nil {
		return nil, fs.ErrClosed
	}
	if name == "." {
		return info{".", 0, true}, nil
	}
	e, err := d.lookup(name)
	if err != nil {
		return nil, err
	}
	return info{name, int64(e.Length) * 512, false}, nil
}
func (d *Drive) ReadDir(name string) ([]fs.DirEntry, error) {
	st, err := d.Stat(name)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", name)
	}
	out := make([]fs.DirEntry, 0, len(d.entries))
	for name, e := range d.entries {
		out = append(out, info{name, int64(e.Length) * 512, false})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}
func (d *Drive) Open(name string) (fs.File, error) {
	st, err := d.Stat(name)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		entries, err := d.ReadDir(name)
		if err != nil {
			return nil, err
		}
		return &dir{info: info{".", 0, true}, entries: entries}, nil
	}
	e, _ := d.lookup(name)
	data := make([]byte, 0, e.Length*512)
	for n := e.Start * 2; n < (e.Start+e.Length)*2; n++ {
		data = append(data, d.raw[n*physicalBytes:n*physicalBytes+256]...)
	}
	return newFile(info{name, int64(len(data)), false}, data), nil
}

// Sector preserves a full physical DECtape block, including its 129th word.
func (d *Drive) Sector(n int64) ([]byte, error) {
	if d.raw == nil {
		return nil, fs.ErrClosed
	}
	if n < 0 || n >= int64(len(d.raw)/physicalBytes) {
		return nil, fmt.Errorf("physical block %d out of range", n)
	}
	off := int(n) * physicalBytes
	return append([]byte(nil), d.raw[off:off+physicalBytes]...), nil
}

// Sectors returns physical block provenance, matching Sector's numbering.
func (d *Drive) Sectors(name string) ([]int64, error) {
	e, err := d.lookup(name)
	if err != nil {
		return nil, err
	}
	out := make([]int64, e.Length*2)
	for i := range out {
		out[i] = int64(e.Start*2 + i)
	}
	return out, nil
}

type info struct {
	name string
	size int64
	dir  bool
}

func (i info) Name() string { return i.name }
func (i info) Size() int64  { return i.size }
func (i info) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0555
	}
	return 0444
}
func (i info) ModTime() time.Time         { return time.Time{} }
func (i info) IsDir() bool                { return i.dir }
func (i info) Sys() any                   { return nil }
func (i info) Type() fs.FileMode          { return i.Mode().Type() }
func (i info) Info() (fs.FileInfo, error) { return i, nil }

type file struct {
	info   info
	r      *bytes.Reader
	closed bool
}

func newFile(i info, b []byte) *file { return &file{info: i, r: bytes.NewReader(b)} }
func (f *file) Stat() (fs.FileInfo, error) {
	if f.closed {
		return nil, fs.ErrClosed
	}
	return f.info, nil
}
func (f *file) Read(b []byte) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.r.Read(b)
}
func (f *file) Close() error { f.closed = true; return nil }

type dir struct {
	info    info
	entries []fs.DirEntry
	pos     int
	closed  bool
}

func (d *dir) Stat() (fs.FileInfo, error) {
	if d.closed {
		return nil, fs.ErrClosed
	}
	return d.info, nil
}
func (d *dir) Read([]byte) (int, error) {
	if d.closed {
		return 0, fs.ErrClosed
	}
	return 0, fmt.Errorf("cannot read directory bytes")
}
func (d *dir) Close() error { d.closed = true; return nil }
func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.closed {
		return nil, fs.ErrClosed
	}
	if d.pos >= len(d.entries) && n > 0 {
		return nil, io.EOF
	}
	end := len(d.entries)
	if n > 0 && d.pos+n < end {
		end = d.pos + n
	}
	out := append([]fs.DirEntry(nil), d.entries[d.pos:end]...)
	d.pos = end
	return out, nil
}
