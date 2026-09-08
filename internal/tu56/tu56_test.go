package tu56

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func image(t *testing.T) string {
	t.Helper()
	b := make([]byte, BytesPerBlock*2)
	for i := range b {
		b[i] = byte(i)
	}
	name := filepath.Join(t.TempDir(), "fixture.tu56")
	if err := os.WriteFile(name, b, 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestOpenListsAndReadsBlocks(t *testing.T) {
	d, err := Open(image(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if !strings.Contains(d.Description(), "blocks=2") {
		t.Fatalf("description = %q", d.Description())
	}
	entries, err := d.ReadDir("blocks")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 || entries[0].Name() != "0000.bin" || entries[1].Name() != "0000.oct" {
		t.Fatalf("entries = %#v", entries)
	}
	sector, err := d.Sector(1)
	if err != nil {
		t.Fatal(err)
	}
	f, err := d.Open("blocks/0001.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, sector) || len(got) != BytesPerBlock {
		t.Fatalf("block read mismatch")
	}
}

func TestRejectsBadSize(t *testing.T) {
	name := filepath.Join(t.TempDir(), "bad.tu56")
	if err := os.WriteFile(name, []byte{1, 2, 3}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(name); err == nil {
		t.Fatal("accepted bad image")
	}
}

func TestMissingBlock(t *testing.T) {
	d, err := Open(image(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Open("blocks/9999.bin"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}
