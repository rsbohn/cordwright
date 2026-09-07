package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratorRefusesOverwrite(t *testing.T) {
	name := filepath.Join(t.TempDir(), "demo.img")
	var out bytes.Buffer
	if err := run([]string{"-stride", "400", name}, &out); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 12992*400 {
		t.Fatal(len(before))
	}
	if err := run([]string{name}, &out); err == nil {
		t.Fatal("overwrote image")
	}
	after, _ := os.ReadFile(name)
	if !bytes.Equal(before, after) {
		t.Fatal("changed existing image")
	}
	for _, args := range [][]string{nil, {"-stride", "123", name}, {name, "extra"}} {
		if err := run(args, &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
