package media

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestHostReadOnlyView(t *testing.T) {
	dir := t.TempDir()
	data := []byte{0, 255, '\r', '\n', 128}
	if err := os.WriteFile(filepath.Join(dir, "Bytes"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	h, err := OpenHost(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	got, err := fs.ReadFile(h, "Bytes")
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("%v %v", got, err)
	}
	entries, err := h.ReadDir(".")
	if err != nil || len(entries) != 2 || entries[0].Name() != "Bytes" || !entries[1].IsDir() {
		t.Fatalf("%v %v", entries, err)
	}
	if _, err := h.Open("bytes"); err == nil {
		t.Fatal("case was folded")
	}
	if _, err := OpenHost(filepath.Join(dir, "Bytes")); err == nil {
		t.Fatal("mounted regular file")
	}
}

func TestHostConfinesPathsAndSymlinks(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"escape": "../secret", "absolute": filepath.Join(parent, "secret"), "safe": "inside"} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	h, err := OpenHost(root)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, name := range []string{"../secret", "/secret", "escape", "absolute", "x/../../secret"} {
		if f, err := h.Open(name); err == nil {
			f.Close()
			t.Errorf("opened %q", name)
		}
		if _, err := h.Stat(name); err == nil {
			t.Errorf("stat escaped via %q", name)
		}
	}
	data, err := fs.ReadFile(h, "safe")
	if err != nil || string(data) != "inside" {
		t.Fatalf("safe symlink: %q %v", data, err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Open("inside"); err == nil {
		t.Fatal("read closed drive")
	}
}
