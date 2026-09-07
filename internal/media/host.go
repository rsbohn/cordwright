// Package media supplies byte-preserving, read-only filesystem views.
package media

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Drive is the filesystem view of mounted media. Raw sector/record interfaces
// will be separate capabilities; a host directory has neither.
type Drive interface {
	fs.FS
	fs.StatFS
	fs.ReadDirFS
	Description() string
	Close() error
}

// Host confines access with os.Root rather than a lexical path-prefix check.
// It exposes no mutation API. Symlinks may only resolve within the root.
type Host struct {
	root   *os.Root
	source string
}

func OpenHost(path string) (*Host, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, err
	}
	return &Host{root: root, source: absolute}, nil
}

func (h *Host) Description() string                        { return "host:" + h.source + " (ro)" }
func (h *Host) Close() error                               { return h.root.Close() }
func (h *Host) Stat(name string) (fs.FileInfo, error)      { return fs.Stat(h.root.FS(), name) }
func (h *Host) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(h.root.FS(), name) }
func (h *Host) Open(name string) (fs.File, error) {
	// Reject special files before opening (notably FIFOs, which could block).
	info, err := h.Stat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file or directory", name)
	}
	f, err := h.root.FS().Open(name)
	if err != nil {
		return nil, err
	}
	info, err = f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("%s: not a regular file or directory", name)
	}
	return f, nil
}
