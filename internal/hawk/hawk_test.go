package hawk

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func saveImage(t *testing.T, image []byte) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "test.img")
	if err := os.WriteFile(name, image, 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestDemoRoundTrip(t *testing.T) {
	for _, stride := range []int{400, 512} {
		image, err := DemoImage(stride)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := DemoImage(stride)
		if !bytes.Equal(image, again) || len(image) != SectorLimit*stride {
			t.Fatal("demo not deterministic or wrong size")
		}
		source := saveImage(t, image)
		d, err := Open(source, stride)
		if err != nil {
			t.Fatal(err)
		}
		if err := fstest.TestFS(d, "README", "HELLO", "NOTES", "NUMBERS"); err != nil {
			t.Fatal(err)
		}
		if text, err := d.Text("HELLO"); err != nil || text != "HELLO FROM CORDWRIGHT!\n" {
			t.Fatalf("%q %v", text, err)
		}
		if text, err := d.Text("NUMBERS"); err != nil || text != "ID,VALUE\n001,10\n002,20\n003,30\n" {
			t.Fatalf("%q %v", text, err)
		}
		notes, err := d.Text("NOTES")
		if err != nil || !strings.Contains(notes, "PAGE 1") || !strings.Contains(notes, "PAGE 2") || !strings.Contains(notes, "PAGE 3") || strings.Index(notes, "PAGE 2") > strings.Index(notes, "PAGE 3") {
			t.Fatalf("%q %v", notes, err)
		}
		sectors, err := d.Sectors("NOTES")
		if err != nil || !reflect.DeepEqual(sectors, []int64{34, 40, 35}) {
			t.Fatalf("%v %v", sectors, err)
		}
		sectors[0] = 999
		sectors, _ = d.Sectors("NOTES")
		if sectors[0] != 34 {
			t.Fatal("caller mutated allocation")
		}
		data, err := fs.ReadFile(d, "NOTES")
		if err != nil || len(data) != 1200 {
			t.Fatalf("len=%d %v", len(data), err)
		}
		for i, n := range sectors {
			if !bytes.Equal(data[i*400:(i+1)*400], image[int(n)*stride:int(n)*stride+400]) {
				t.Fatal("payload altered")
			}
		}
		raw, err := d.Sector(34)
		if err != nil || !bytes.Equal(raw, image[34*stride:35*stride]) {
			t.Fatal("record altered")
		}
		for _, name := range []string{"hello", "../HELLO", "/HELLO", "NOTES/member"} {
			if _, err := d.Open(name); err == nil {
				t.Errorf("opened invalid/missing path %q", name)
			}
		}
		for _, n := range []int64{-1, SectorLimit, SectorLimit + 1} {
			if _, err := d.Sector(n); err == nil {
				t.Fatal("out of range accepted")
			}
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Open("HELLO"); err == nil {
			t.Fatal("opened closed drive")
		}
		unchanged, err := os.ReadFile(source)
		if err != nil || !bytes.Equal(unchanged, image) {
			t.Fatal("source changed")
		}
	}
}

// Independent tiny layout: one file with FSI=2, not produced by DemoImage.
func tinyImage() []byte {
	b := make([]byte, 64*512)
	b[14*512+8], b[14*512+9] = 255, 255
	b[16*512+15] = 18
	copy(b[16*512+16:], []byte{0xc1, 0xa0, 0xa0, 0xa0, 0xa0, 0xa0, 0xa0, 0xa0, 0xa0, 0xa0})
	b[16*512+32], b[16*512+33] = 0x84, 0x8d
	b[18*512+1], b[18*512+4], b[18*512+8] = 1, 1, 32
	return b
}

func TestIndependentFSI(t *testing.T) {
	d, err := Open(saveImage(t, tinyImage()), 512)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	sectors, err := d.Sectors("A")
	if err != nil || !reflect.DeepEqual(sectors, []int64{32, 33}) {
		t.Fatalf("%v %v", sectors, err)
	}
}

func TestCorruptImagesRejected(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func([]byte) []byte
	}{
		{"short image", func(b []byte) []byte { return b[:100] }},
		{"short volume", func(b []byte) []byte { return b[:16*512] }},
		{"bad flag", func(b []byte) []byte { b[14*512+8] = 0; return b }},
		{"bad UAL", func(b []byte) []byte { b[16*512+15] = 200; return b }},
		{"bad index", func(b []byte) []byte { b[16*512+26] = 255; return b }},
		{"bad shift", func(b []byte) []byte { b[18*512+4] = 16; return b }},
		{"bad length", func(b []byte) []byte { b[18*512] = 255; return b }},
		{"bad data sector", func(b []byte) []byte { b[18*512+8] = 200; return b }},
		{"extent overrun", func(b []byte) []byte { b[18*512+8] = 63; return b }},
		{"early end", func(b []byte) []byte { copy(b[18*512+6:], []byte{255, 255, 255}); return b }},
		{"cycle", func(b []byte) []byte { copy(b[18*512+6:], []byte{255, 255, 2}); return b }},
		{"bad continuation", func(b []byte) []byte { copy(b[18*512+6:], []byte{255, 255, 200}); return b }},
		{"duplicate name", func(b []byte) []byte {
			copy(b[16*512+32:], b[16*512+16:16*512+32])
			b[16*512+48], b[16*512+49] = 0x84, 0x8d
			return b
		}},
		{"path name", func(b []byte) []byte { b[16*512+16] = '/'; return b }},
		{"no catalog terminator", func(b []byte) []byte { b[16*512+32], b[16*512+33] = 0, 0; return b[:18*512] }},
		{"duplicate data", func(b []byte) []byte { b[18*512+4] = 0; b[18*512+11] = 32; return b }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Open(saveImage(t, tt.change(tinyImage())), 512)
			if err == nil {
				d.Close()
				t.Fatal("accepted corrupt image")
			}
		})
	}
	if d, err := Open(saveImage(t, tinyImage()), 128); err == nil {
		d.Close()
		t.Fatal("invalid stride")
	}
}

func TestFileHandlesAndTruncation(t *testing.T) {
	path := saveImage(t, tinyImage())
	d, err := Open(path, 512)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	f, err := d.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	rd := f.(fs.ReadDirFile)
	if entries, err := rd.ReadDir(1); err != nil || len(entries) != 1 {
		t.Fatalf("%v %v", entries, err)
	}
	if _, err := rd.ReadDir(1); err != io.EOF {
		t.Fatal(err)
	}
	f.Close()
	if _, err := rd.ReadDir(1); err != fs.ErrClosed {
		t.Fatal(err)
	}
	f, err = d.Open("A")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := f.Read(make([]byte, 1)); err != fs.ErrClosed {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 32*512+100); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Open("A"); err == nil {
		t.Fatal("silently read truncated data")
	}
}

func TestLongAllocationChain(t *testing.T) {
	b := make([]byte, 512*180)
	copy(b, tinyImage())
	b[18*512+1], b[18*512+4] = 139, 0
	for i := 0; i < 130; i++ {
		put24(b[18*512+6+i*3:], 32+i)
	}
	copy(b[18*512+396:], []byte{255, 254, 0})
	for i := 0; i < 10; i++ {
		put24(b[19*512+i*3:], 162+i)
	}
	d, err := Open(saveImage(t, b), 512)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	sectors, err := d.Sectors("A")
	if err != nil || len(sectors) != 140 {
		t.Fatalf("%d %v", len(sectors), err)
	}
	for i, n := range sectors {
		if n != int64(32+i) {
			t.Fatalf("%d: %d", i, n)
		}
	}
}

func FuzzOpen(f *testing.F) {
	f.Add(tinyImage())
	f.Add([]byte("not an image"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 256*512 {
			t.Skip()
		}
		name := saveImage(t, b)
		d, err := Open(name, 512)
		if err != nil {
			return
		}
		defer d.Close()
		entries, err := d.ReadDir(".")
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if _, err := fs.ReadFile(d, e.Name()); err != nil {
				t.Fatal(err)
			}
		}
	})
}
