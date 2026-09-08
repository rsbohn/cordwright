// Package tu56 exposes read-only DECtape/TU56 image containers.
package tu56

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	WordsPerBlock = 129
	BytesPerBlock = WordsPerBlock * 2
)

type Drive struct {
	source string
	data   []byte
	blocks int
}

func Open(source string) (*Drive, error) {
	b, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || len(b)%BytesPerBlock != 0 {
		return nil, fmt.Errorf("invalid TU56 image size %d (want a non-empty multiple of %d)", len(b), BytesPerBlock)
	}
	return &Drive{source: source, data: b, blocks: len(b) / BytesPerBlock}, nil
}

func (d *Drive) Description() string {
	return fmt.Sprintf("tu56:%s blocks=%d words/block=%d (ro, container)", d.source, d.blocks, WordsPerBlock)
}
func (d *Drive) Close() error { d.data = nil; return nil }

func (d *Drive) Sector(n int64) ([]byte, error) {
	if n < 0 || n >= int64(d.blocks) {
		return nil, fmt.Errorf("block %d: out of range", n)
	}
	off := int(n) * BytesPerBlock
	return append([]byte(nil), d.data[off:off+BytesPerBlock]...), nil
}

func (d *Drive) Open(name string) (fs.File, error) {
	name = clean(name)
	if name == "." || name == "blocks" {
		return d.dirFile(name)
	}
	if name == "README.txt" {
		return newFile(info{"README.txt", int64(len(d.readme())), false}, d.readme()), nil
	}
	if strings.HasPrefix(name, "blocks/") {
		base := path.Base(name)
		n, ok, text := parseBlockName(base)
		if !ok || n < 0 || n >= d.blocks {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		raw, _ := d.Sector(int64(n))
		if text {
			return newFile(info{base, int64(len(octalDump(n, raw))), false}, octalDump(n, raw)), nil
		}
		return newFile(info{base, int64(len(raw)), false}, raw), nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (d *Drive) Stat(name string) (fs.FileInfo, error) {
	f, err := d.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func (d *Drive) ReadDir(name string) ([]fs.DirEntry, error) {
	name = clean(name)
	switch name {
	case ".":
		return []fs.DirEntry{info{"README.txt", int64(len(d.readme())), false}, info{"blocks", 0, true}}, nil
	case "blocks":
		entries := make([]fs.DirEntry, 0, d.blocks*2)
		width := len(strconv.Itoa(d.blocks - 1))
		if width < 4 {
			width = 4
		}
		for i := 0; i < d.blocks; i++ {
			entries = append(entries, info{fmt.Sprintf("%0*d.bin", width, i), BytesPerBlock, false})
			entries = append(entries, info{fmt.Sprintf("%0*d.oct", width, i), int64(8 * WordsPerBlock), false})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		return entries, nil
	default:
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
}

func (d *Drive) dirFile(name string) (fs.File, error) {
	entries, err := d.ReadDir(name)
	if err != nil {
		return nil, err
	}
	return &dir{info: info{path.Base(name), 0, true}, entries: entries}, nil
}

func (d *Drive) readme() []byte {
	return []byte(fmt.Sprintf("TU56/DECtape container image\nSource: %s\nBlocks: %d\nBlock format: %d 12-bit words stored as big-endian 16-bit words (%d bytes).\nUse 'sector /MOUNT N' for the original container block, or read blocks/NNNN.bin.\nOS/8 filesystem decoding is not yet implemented.\n", d.source, d.blocks, WordsPerBlock, BytesPerBlock))
}

func clean(name string) string {
	if name == "" {
		return "."
	}
	return strings.TrimPrefix(path.Clean(name), "/")
}

func parseBlockName(base string) (int, bool, bool) {
	text := false
	switch {
	case strings.HasSuffix(base, ".bin"):
		base = strings.TrimSuffix(base, ".bin")
	case strings.HasSuffix(base, ".oct"):
		base = strings.TrimSuffix(base, ".oct")
		text = true
	default:
		return 0, false, false
	}
	n, err := strconv.Atoi(base)
	return n, err == nil, text
}

func octalDump(block int, raw []byte) []byte {
	var b strings.Builder
	for i := 0; i < len(raw); i += 2 {
		if (i/2)%8 == 0 {
			fmt.Fprintf(&b, "%04o:", i/2)
		}
		w := int(raw[i])<<8 | int(raw[i+1])
		fmt.Fprintf(&b, " %04o", w&07777)
		if (i/2)%8 == 7 {
			b.WriteByte('\n')
		}
	}
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
	}
	return []byte(b.String())
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
