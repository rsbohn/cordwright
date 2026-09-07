package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
