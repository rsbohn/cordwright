package main

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func fixture() []byte {
	raw := make([]byte, 10*2*physicalBytes)
	// Three entries: a zero-length file, a one-block file, and free space.
	words := []uint16{07775, 7, 0, 0, 07777,
		02405, 02324, 0, 02001, 04017, 0,
		00401, 02401, 0, 02001, 04017, 07777,
		0, 07776}
	for i, w := range words {
		binary.LittleEndian.PutUint16(raw[2*physicalBytes+i*2:], w)
	}
	return raw
}

func TestDirectory(t *testing.T) {
	raw := fixture()
	var out bytes.Buffer
	if err := dump(&out, raw); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TEST.PA", "DATA.PA", "<FREE>"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal(out.String())
		}
	}
	blocks, err := logicalBlocks(raw)
	if err != nil {
		t.Fatal(err)
	}
	s, err := parseSegment(blocks, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Entries) != 3 || s.Entries[0].Length != 0 || s.Entries[1].Start != 7 || s.Entries[2].Start != 8 {
		t.Fatalf("%+v", s)
	}
}

func TestMapping(t *testing.T) {
	raw := fixture()
	binary.LittleEndian.PutUint16(raw[3*physicalBytes:], 01234)
	binary.LittleEndian.PutUint16(raw[2*physicalBytes+256:], 07777) // unused word
	blocks, err := logicalBlocks(raw)
	if err != nil {
		t.Fatal(err)
	}
	if blocks[1][128] != 01234 {
		t.Fatal("incorrect physical pairing")
	}
	if six(0520) != "EP" || six(01103) != "IC" || six(02001) != "PA" {
		t.Fatal("six-bit decoding")
	}
}

func TestCorruption(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func([]byte) []byte
	}{
		{"short", func(b []byte) []byte { return b[:len(b)-1] }},
		{"cycle", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[2*physicalBytes+4:], 1); return b }},
		{"extent", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[2*physicalBytes+18*2:], 07000); return b }},
		{"word", func(b []byte) []byte { b[1] = 255; return b }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := dump(&bytes.Buffer{}, tc.change(fixture())); err == nil {
				t.Fatal("accepted corruption")
			}
		})
	}
}
