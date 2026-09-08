package os8

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func save(t *testing.T, raw []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.tu56")
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestDrive(t *testing.T) {
	raw := fixture()
	for i := 0; i < 256; i++ {
		off := (14+i/128)*physicalBytes + (i%128)*2
		binary.LittleEndian.PutUint16(raw[off:], uint16(i))
	}
	source := save(t, raw)
	d, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := fstest.TestFS(d, "TEST.PA", "DATA.PA"); err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(d, "DATA.PA")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 512 || binary.LittleEndian.Uint16(data[256:]) != 128 {
		t.Fatal("incorrect file data")
	}
	zero, err := fs.ReadFile(d, "TEST.PA")
	if err != nil || len(zero) != 0 {
		t.Fatal("zero-length file", err)
	}
	sectors, err := d.Sectors("DATA.PA")
	if err != nil || len(sectors) != 2 || sectors[0] != 14 || sectors[1] != 15 {
		t.Fatal(sectors, err)
	}
	sector, err := d.Sector(14)
	if err != nil || !bytes.Equal(sector, raw[14*physicalBytes:15*physicalBytes]) {
		t.Fatal("raw sector", err)
	}
	sector[0] ^= 255
	again, _ := d.Sector(14)
	if bytes.Equal(sector, again) {
		t.Fatal("aliased sector")
	}
	for _, n := range []int64{-1, 20} {
		if _, err := d.Sector(n); err == nil {
			t.Fatal("accepted out-of-range block")
		}
	}
	unchanged, _ := os.ReadFile(source)
	if !bytes.Equal(unchanged, raw) {
		t.Fatal("source changed")
	}
	d.Close()
	if _, err := d.Open("DATA.PA"); !errors.Is(err, fs.ErrClosed) {
		t.Fatal(err)
	}
	if _, err := d.Sector(0); !errors.Is(err, fs.ErrClosed) {
		t.Fatal(err)
	}
}

func TestRejectDuplicateAndOverlap(t *testing.T) {
	for _, duplicate := range []bool{true, false} {
		raw := fixture()
		if duplicate {
			copy(raw[2*physicalBytes+11*2:2*physicalBytes+15*2], raw[2*physicalBytes+5*2:2*physicalBytes+9*2])
		} else {
			binary.LittleEndian.PutUint16(raw[2*physicalBytes+4:], 2)
			copy(raw[4*physicalBytes:4*physicalBytes+256], raw[2*physicalBytes:2*physicalBytes+256])
			binary.LittleEndian.PutUint16(raw[4*physicalBytes+4:], 0)
		}
		if d, err := Open(save(t, raw)); err == nil {
			d.Close()
			t.Fatal("accepted corrupt directory")
		}
	}
}
