package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsbohn/cordwright/internal/hawk"
	"github.com/rsbohn/cordwright/internal/tu56"
)

func TestCLI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello world"), []byte{0, 255, 10}, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		args  []string
		input string
		code  int
		want  string
	}{
		{"one shot preserves arguments", []string{"-host", "h0=" + dir, "cat", "/h0/hello world"}, "", 0, string([]byte{0, 255, 10})},
		{"batch", []string{"-host", "h0=" + dir, "-batch"}, "cd /h0\npwd\nexit\n", 0, "/h0\n"},
		{"runtime mount", []string{"-batch"}, "mount '" + dir + "' /h0\nls /\n", 0, "h0/\n"},
		{"invalid mount", []string{"-host", "bad"}, "", 1, ""},
		{"invalid personality", []string{"-personality", "cpu6"}, "", 1, ""},
		{"command error", []string{"missing"}, "", 1, ""},
		{"batch error", []string{"-batch"}, "missing\npwd\n", 1, "/\n"},
		{"help flag", []string{"-h"}, "", 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			code := run(tt.args, strings.NewReader(tt.input), &out, &diagnostics, false)
			if code != tt.code || out.String() != tt.want {
				t.Fatalf("code=%d out=%q diagnostics=%q", code, out.String(), diagnostics.String())
			}
			if code != 0 && diagnostics.Len() == 0 {
				t.Fatal("no diagnostic")
			}
		})
	}
}

func TestHawkCLI(t *testing.T) {
	image, err := hawk.DemoImage(400)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "demo.img")
	if err := os.WriteFile(name, image, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := run([]string{"-hawk", "h2=" + name, "-stride", "400", "text", "/h2/HELLO"}, strings.NewReader(""), &out, &diagnostics, false)
	if code != 0 || out.String() != "HELLO FROM CORDWRIGHT!\n" {
		t.Fatalf("%d %q %q", code, out.String(), diagnostics.String())
	}
}

func TestTU56CLI(t *testing.T) {
	image := make([]byte, tu56.BytesPerBlock)
	name := filepath.Join(t.TempDir(), "demo.tu56")
	if err := os.WriteFile(name, image, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := run([]string{"-tu56", "t0=" + name, "-o", "raw", "ls", "/t0"}, strings.NewReader(""), &out, &diagnostics, false)
	if code != 0 || out.String() != "README.txt\nblocks/\n" {
		t.Fatalf("%d %q %q", code, out.String(), diagnostics.String())
	}
}

func TestOS8Mounts(t *testing.T) {
	image := make([]byte, 8*2*tu56.BytesPerBlock)
	// One file occupying logical block 7: TEST.PA.
	words := []uint16{07777, 7, 0, 0, 07777, 02405, 02324, 0, 02001, 0, 07777}
	for i, w := range words {
		image[2*tu56.BytesPerBlock+i*2] = byte(w)
		image[2*tu56.BytesPerBlock+i*2+1] = byte(w >> 8)
	}
	name := filepath.Join(t.TempDir(), "test.tu56")
	if err := os.WriteFile(name, image, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args        []string
		input, want string
		code        int
	}{
		{[]string{"-tu56", "t=" + name, "ls", "/t"}, "", "TEST.PA\n", 0},
		{[]string{"-batch"}, "mount -t tu56 '" + name + "' /t\ncd /t\nls\nsectors TEST.PA\numount /t\npwd\n", "TEST.PA\n0: 14 (0xE)\n1: 15 (0xF)\n/\n", 0},
		{[]string{"-batch"}, "mount -t tu56 -o raw '" + name + "' /t\nls /t\n", "README.txt\nblocks/\n", 0},
		{[]string{"-o", "bogus", "-tu56", "t=" + name}, "", "", 1},
		{[]string{"-batch"}, "mount -t tu56 -o bogus '" + name + "' /t\n", "", 1},
	} {
		var out, diag bytes.Buffer
		code := run(tc.args, strings.NewReader(tc.input), &out, &diag, false)
		if code != tc.code || out.String() != tc.want {
			t.Fatalf("code=%d out=%q diagnostic=%q", code, out.String(), diag.String())
		}
	}
}
