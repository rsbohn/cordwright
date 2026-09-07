// Package hawk implements an experimental, read-only Disk6 filesystem view.
// Layout and allocation rules follow tricorn's CPU6.dos reader and its Hawk
// notes. The sector-16 top-level catalog and simple Hawk library catalogs are
// supported read-only.
package hawk

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	PayloadSize = 400
	SectorLimit = 0x32c0
)

type entry struct {
	name    string
	sectors []int64
	parent  string
	start   int
	end     int
	dir     bool
	kids    map[string]bool
}

type Drive struct {
	file    *os.File
	source  string
	stride  int
	count   int64
	entries map[string]entry
}

func Open(source string, stride int) (*Drive, error) {
	if stride != 400 && stride != 512 {
		return nil, fmt.Errorf("Hawk stride must be 400 or 512")
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(absolute)
	if err != nil {
		return nil, err
	}
	d := &Drive{file: f, source: absolute, stride: stride, entries: map[string]entry{}}
	info, err := f.Stat()
	if err == nil && (!info.Mode().IsRegular() || info.Size()%int64(stride) != 0 || info.Size() > int64(SectorLimit*stride)) {
		err = fmt.Errorf("invalid Hawk image size or file type")
	}
	if err == nil {
		d.count = info.Size() / int64(stride)
		err = d.catalog()
		if err == nil {
			err = d.libraries()
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("Hawk %s: %w", source, err)
	}
	return d, nil
}

func (d *Drive) Close() error { return d.file.Close() }
func (d *Drive) Description() string {
	return fmt.Sprintf("hawk:%s stride=%d sectors=%d (ro, experimental)", d.source, d.stride, d.count)
}

// Sector returns a complete image record, including container padding.
func (d *Drive) Sector(n int64) ([]byte, error) {
	if n < 0 || n >= d.count || n >= SectorLimit {
		return nil, fmt.Errorf("sector %d: out of range", n)
	}
	b := make([]byte, d.stride)
	if _, err := d.file.ReadAt(b, n*int64(d.stride)); err != nil {
		return nil, fmt.Errorf("sector %d: %w", n, err)
	}
	return b, nil
}
func (d *Drive) payload(n int64) ([]byte, error) {
	b, err := d.Sector(n)
	if err != nil {
		return nil, err
	}
	return b[:PayloadSize], nil
}

func (d *Drive) catalog() error {
	ctl, err := d.payload(14)
	if err != nil {
		return err
	}
	if u16(ctl[8:]) != 0xffff {
		return fmt.Errorf("missing Disk6 flag")
	}
	vol, err := d.payload(16)
	if err != nil {
		return err
	}
	base := int64(u24(vol[13:]))
	if base >= d.count {
		return fmt.Errorf("UAL base out of range")
	}
	scanned := 0
	for sector := int64(16); sector < d.count && scanned < 800; sector++ {
		b, err := d.payload(sector)
		if err != nil {
			return err
		}
		start := 0
		if sector == 16 {
			start = 16
		}
		for off := start; off+16 <= len(b) && scanned < 800; off += 16 {
			scanned++
			row := b[off : off+16]
			if row[0] == 0x84 && row[1] == 0x8d {
				return nil
			}
			if row[0] == 0 {
				continue
			}
			name, err := decodeName(row[:10])
			if err != nil {
				return err
			}
			if _, exists := d.entries[name]; exists {
				return fmt.Errorf("duplicate catalog name %q", name)
			}
			sectors, err := d.allocation(base, base+int64(u16(row[11:])), int(row[10])*3)
			if err != nil {
				return fmt.Errorf("file %s: %w", name, err)
			}
			d.entries[name] = entry{name: name, sectors: sectors}
		}
	}
	return fmt.Errorf("unterminated Hawk catalog")
}

func (d *Drive) allocation(base, mapSector int64, index int) ([]int64, error) {
	b, err := d.payload(mapSector)
	if err != nil {
		return nil, err
	}
	if index < 0 || index+6 > len(b) {
		return nil, fmt.Errorf("invalid allocation header")
	}
	length := u16(b[index:]) + 1
	shift := b[index+4]
	if shift > 15 || length > int(d.count) {
		return nil, fmt.Errorf("invalid allocation length/shift")
	}
	fsi := 1 << shift
	off := index + 6
	type location struct {
		sector int64
		offset int
	}
	visited := map[location]bool{}
	used := map[int64]bool{}
	sectors := make([]int64, 0, length)
	for len(sectors) < length {
		loc := location{mapSector, off}
		if visited[loc] {
			return nil, fmt.Errorf("cyclic allocation chain")
		}
		visited[loc] = true
		if off < 0 || off+3 > len(b) {
			return nil, fmt.Errorf("incomplete allocation chain")
		}
		value := u24(b[off:])
		if value == 0xffffff {
			return nil, fmt.Errorf("premature allocation terminator")
		}
		if value >= 0x800000 {
			mapSector = base + int64(u16(b[off:])^0xffff)
			off = int(b[off+2]) * 3
			b, err = d.payload(mapSector)
			if err != nil {
				return nil, err
			}
			continue
		}
		for delta := 0; delta < fsi && len(sectors) < length; delta++ {
			n := int64(value + delta)
			if n >= d.count || n >= SectorLimit {
				return nil, fmt.Errorf("data sector out of range")
			}
			if used[n] {
				return nil, fmt.Errorf("duplicate data sector %d", n)
			}
			used[n] = true
			sectors = append(sectors, n)
		}
		off += 3
	}
	return sectors, nil
}

func (d *Drive) libraries() error {
	names := make([]string, 0, len(d.entries))
	for name := range d.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		e := d.entries[name]
		members, ok, err := d.libraryEntries(e)
		if err != nil {
			return fmt.Errorf("library %s: %w", name, err)
		}
		if !ok {
			continue
		}
		e.dir = true
		e.kids = map[string]bool{}
		d.entries[name] = e
		for _, m := range members {
			full := name + "/" + m.name
			if _, exists := d.entries[full]; exists {
				return fmt.Errorf("duplicate library path %q", full)
			}
			m.parent = name
			d.entries[full] = m
			e.kids[m.name] = true
		}
		d.entries[name] = e
	}
	return nil
}

type libraryMember struct {
	name  string
	start int
}

func (d *Drive) libraryEntries(lib entry) ([]entry, bool, error) {
	data, err := d.entryData(lib)
	if err != nil {
		return nil, false, err
	}
	if len(data) < 32 || !blankName(data[:10]) {
		return nil, false, nil
	}
	totalPages := len(data) / 200
	rows := []libraryMember{}
	baseHigh := -1
	for off, scanned := 16, 0; off+16 <= len(data) && scanned < 800; off, scanned = off+16, scanned+1 {
		row := data[off : off+16]
		if row[0] == 0x84 && row[1] == 0x8d || row[0] == 0 || blankName(row[:10]) {
			break
		}
		name, err := decodeName(row[:10])
		if err != nil {
			return nil, false, nil
		}
		if baseHigh < 0 {
			baseHigh = int(row[12])
		}
		start := int(row[10]) + (int(row[12])-baseHigh)*128
		if start < 0 || start >= totalPages || len(rows) > 0 && start < rows[len(rows)-1].start {
			return nil, false, nil
		}
		rows = append(rows, libraryMember{name, start})
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	members := make([]entry, 0, len(rows))
	seen := map[string]bool{}
	for i, r := range rows {
		end := totalPages
		if i+1 < len(rows) {
			end = rows[i+1].start
		}
		if end < r.start || seen[r.name] {
			return nil, false, nil
		}
		seen[r.name] = true
		members = append(members, entry{name: r.name, sectors: lib.sectors, start: r.start * 200, end: end * 200})
	}
	return members, true, nil
}

func blankName(b []byte) bool {
	for _, c := range b {
		if c != 0 && c != 0xa0 && c != ' ' {
			return false
		}
	}
	return true
}

func decodeName(b []byte) (string, error) {
	var name strings.Builder
	for _, raw := range b {
		c := raw & 0x7f
		if c == 0 || c == ' ' {
			break
		}
		if c < 33 || c >= 127 || c == '/' || c == '\\' {
			return "", fmt.Errorf("invalid catalog name")
		}
		name.WriteByte(c)
	}
	s := name.String()
	if s == "" || s == "." || s == ".." {
		return "", fmt.Errorf("invalid catalog name")
	}
	return s, nil
}
func u16(b []byte) int { return int(b[0])<<8 | int(b[1]) }
func u24(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }

func (d *Drive) lookup(name string) (entry, error) {
	if !fs.ValidPath(name) {
		return entry{}, &fs.PathError{Op: "lookup", Path: name, Err: fs.ErrInvalid}
	}
	// The Unix personality stays case-sensitive, including image names.
	e, ok := d.entries[name]
	if !ok {
		return entry{}, &fs.PathError{Op: "lookup", Path: name, Err: fs.ErrNotExist}
	}
	return e, nil
}
func (d *Drive) Stat(name string) (fs.FileInfo, error) {
	if _, err := d.file.Stat(); err != nil {
		return nil, err
	}
	if name == "." {
		return info{".", 0, true}, nil
	}
	e, err := d.lookup(name)
	if err != nil {
		return nil, err
	}
	size := int64(len(e.sectors) * PayloadSize)
	if e.parent != "" {
		size = int64(e.end - e.start)
	}
	return info{e.name, size, e.dir}, nil
}
func (d *Drive) ReadDir(name string) ([]fs.DirEntry, error) {
	if _, err := d.Stat(name); err != nil {
		return nil, err
	}
	var list []entry
	if name == "." {
		for key, e := range d.entries {
			if !strings.Contains(key, "/") {
				list = append(list, e)
			}
		}
	} else {
		dir, err := d.lookup(name)
		if err != nil {
			return nil, err
		}
		if !dir.dir {
			return nil, fmt.Errorf("%s: not a directory", name)
		}
		for child := range dir.kids {
			list = append(list, d.entries[name+"/"+child])
		}
	}
	entries := make([]fs.DirEntry, 0, len(list))
	for _, e := range list {
		size := int64(len(e.sectors) * PayloadSize)
		if e.parent != "" {
			size = int64(e.end - e.start)
		}
		entries = append(entries, fs.FileInfoToDirEntry(info{e.name, size, e.dir}))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// Sectors returns allocation-order provenance, not a sorted physical list.
func (d *Drive) Sectors(name string) ([]int64, error) {
	if _, err := d.Stat(name); err != nil {
		return nil, err
	}
	e, err := d.lookup(name)
	if err != nil {
		return nil, err
	}
	if e.dir {
		return nil, fmt.Errorf("%s: is a directory", name)
	}
	if e.parent == "" {
		return append([]int64(nil), e.sectors...), nil
	}
	first, last := e.start/PayloadSize, (e.end-1)/PayloadSize
	if e.end == e.start {
		return nil, nil
	}
	return append([]int64(nil), e.sectors[first:last+1]...), nil
}
func (d *Drive) Open(name string) (fs.File, error) {
	stat, err := d.Stat(name)
	if err != nil {
		return nil, err
	}
	f := &file{info: stat}
	if stat.IsDir() {
		f.entries, err = d.ReadDir(name)
		return f, err
	}
	e, _ := d.lookup(name)
	data, err := d.entryData(e)
	if err != nil {
		return nil, err
	}
	f.reader = bytes.NewReader(data)
	return f, nil
}

func (d *Drive) entryData(e entry) ([]byte, error) {
	data := make([]byte, 0, len(e.sectors)*PayloadSize)
	for _, n := range e.sectors {
		b, err := d.payload(n)
		if err != nil {
			return nil, err
		}
		data = append(data, b...)
	}
	if e.parent != "" {
		if e.start < 0 || e.end < e.start || e.end > len(data) {
			return nil, fmt.Errorf("library member bounds invalid")
		}
		data = data[e.start:e.end]
	}
	return data, nil
}

// Text is an explicit, lossy high-bit ASCII view: strip high bits and NUL
// padding, map CR to LF, stop at EOT. No heuristic slack/page trimming is done.
func (d *Drive) Text(name string) (string, error) {
	f, err := d.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for _, raw := range b {
		c := raw & 0x7f
		if c == 4 {
			break
		}
		if c == 0 {
			continue
		}
		if c == '\r' {
			c = '\n'
		}
		text.WriteByte(c)
	}
	return text.String(), nil
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
func (i info) ModTime() time.Time { return time.Time{} } // Date encoding is unknown.
func (i info) IsDir() bool        { return i.dir }
func (i info) Sys() any           { return nil }

type file struct {
	info    fs.FileInfo
	reader  *bytes.Reader
	entries []fs.DirEntry
	offset  int
	closed  bool
}

func (f *file) Close() error { f.closed = true; return nil }
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
	if f.info.IsDir() {
		return 0, fmt.Errorf("cannot read directory bytes")
	}
	return f.reader.Read(b)
}
func (f *file) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.closed {
		return nil, fs.ErrClosed
	}
	if !f.info.IsDir() {
		return nil, fmt.Errorf("not a directory")
	}
	if n > 0 && f.offset >= len(f.entries) {
		return nil, io.EOF
	}
	end := len(f.entries)
	if n > 0 && n < end-f.offset {
		end = f.offset + n
	}
	result := append([]fs.DirEntry{}, f.entries[f.offset:end]...)
	f.offset = end
	return result, nil
}
