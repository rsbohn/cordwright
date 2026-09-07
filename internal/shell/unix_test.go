package shell

import (
	"bytes"
	"errors"
	"github.com/rsbohn/cordwright/internal/hawk"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T) (*Unix, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "hello world"), []byte("Hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "raw"), []byte{0, 255, 13, 10}, 0600); err != nil {
		t.Fatal(err)
	}
	sh := NewUnix()
	t.Cleanup(func() { sh.Close() })
	if err := sh.MountHost(dir, "/h0"); err != nil {
		t.Fatal(err)
	}
	return sh, dir
}

func command(t *testing.T, sh *Unix, line string) string {
	t.Helper()
	args, err := Words(line)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := sh.Execute(args, &out); err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	return out.String()
}

func TestUnixNavigationAndReading(t *testing.T) {
	sh, dir := fixture(t)
	if got := command(t, sh, "ls"); got != "h0/\n" {
		t.Fatal(got)
	}
	if got := command(t, sh, "mount"); !strings.Contains(got, dir+" (ro) on /h0") {
		t.Fatal(got)
	}
	command(t, sh, "cd /h0/sub")
	if got := command(t, sh, "pwd"); got != "/h0/sub\n" {
		t.Fatal(got)
	}
	if got := command(t, sh, "cat 'hello world'"); got != "Hello\n" {
		t.Fatal(got)
	}
	if got := command(t, sh, "cat ../raw"); got != string([]byte{0, 255, 13, 10}) {
		t.Fatalf("%q", got)
	}
	if got := command(t, sh, "hex ../raw"); !strings.Contains(got, "00 ff 0d 0a") {
		t.Fatal(got)
	}
	if got := command(t, sh, "stat 'hello world'"); !strings.Contains(got, " 6 ") || !strings.Contains(got, "/h0/sub/hello world") {
		t.Fatal(got)
	}
	if got := command(t, sh, "ls -l"); !strings.Contains(got, " 6 hello world") {
		t.Fatal(got)
	}
	if got := command(t, sh, "ls /h0/raw"); got != "raw\n" {
		t.Fatal(got)
	}
	command(t, sh, "cd ../../../../")
	if sh.Pwd() != "/" {
		t.Fatal(sh.Pwd())
	}
	command(t, sh, "cd /h0/sub")
	command(t, sh, "umount /h0")
	if sh.Pwd() != "/" || command(t, sh, "ls") != "" {
		t.Fatal("unmount did not reset cwd")
	}
	command(t, sh, "mount '"+dir+"' /h0")
	if got := command(t, sh, "cat /h0/raw"); got != string([]byte{0, 255, 13, 10}) {
		t.Fatal("source changed")
	}
}

func TestUnixErrorsDoNotChangeState(t *testing.T) {
	sh, _ := fixture(t)
	command(t, sh, "cd /h0/sub")
	for _, line := range []string{"cd /missing", "cd /h0/raw", "cat /h0/sub", "cat /h0/nope", "ls -x", "stat", "hex", "exit extra", "rm /h0/raw", "mount /missing /other", "umount /other"} {
		args, _ := Words(line)
		if _, err := sh.Execute(args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %q", line)
		}
		if sh.Pwd() != "/h0/sub" {
			t.Fatal("failed command changed cwd")
		}
	}
	for _, point := range []string{"h1", "/", "/..", "/h1/sub", "/h0"} {
		if err := sh.MountHost(t.TempDir(), point); err == nil {
			t.Errorf("accepted mount %q", point)
		}
	}
}

func TestMultipleDrivesAndLiteralPaths(t *testing.T) {
	sh, dir := fixture(t)
	if err := sh.MountHost(dir, "/a"); err != nil {
		t.Fatal(err)
	}
	if got := command(t, sh, "ls /"); got != "a/\nh0/\n" {
		t.Fatal(got)
	}
	if err := os.WriteFile(filepath.Join(dir, "-name"), []byte("literal"), 0600); err != nil {
		t.Fatal(err)
	}
	command(t, sh, "cd /a")
	if got := command(t, sh, "cat -- -name"); got != "literal" {
		t.Fatal(got)
	}
	command(t, sh, "cd ../h0")
	command(t, sh, "umount /a")
	if sh.Pwd() != "/h0" {
		t.Fatal(sh.Pwd())
	}
}

func TestWords(t *testing.T) {
	for _, tt := range []struct {
		line string
		want []string
	}{
		{`cat 'a b' "c d" e\ f`, []string{"cat", "a b", "c d", "e f"}},
		{`cat '' ""`, []string{"cat", "", ""}},
		{`ls # comment`, []string{"ls"}},
		{`cat a#b 'x|y'`, []string{"cat", "a#b", "x|y"}},
		{` # comment`, nil},
	} {
		got, err := Words(tt.line)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%q: %v %v", tt.line, got, err)
		}
	}
	for _, line := range []string{`cat 'bad`, `cat bad\`, `cat a | cat`, `cat a > b`, `ls; pwd`} {
		if _, err := Words(line); err == nil {
			t.Errorf("accepted %q", line)
		}
	}
}

func TestRunStreamingAndErrors(t *testing.T) {
	sh, _ := fixture(t)
	var out, diagnostics bytes.Buffer
	err := sh.Run(strings.NewReader("bad\npwd\nexit\ncat /h0/raw\n"), &out, &diagnostics, false)
	if err == nil || out.String() != "/\n" || !strings.Contains(diagnostics.String(), "unknown command") {
		t.Fatalf("%q %q %v", out.String(), diagnostics.String(), err)
	}
	out.Reset()
	diagnostics.Reset()
	if err := sh.Run(strings.NewReader("bad\npwd\n"), &out, &diagnostics, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "cordwright:/$ ") {
		t.Fatal(out.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output failed") }
func TestOutputErrors(t *testing.T) {
	sh, _ := fixture(t)
	for _, line := range []string{"pwd", "help", "mount", "ls", "ls /h0", "cat /h0/raw", "hex /h0/raw", "stat /h0/raw"} {
		args, _ := Words(line)
		if _, err := sh.Execute(args, failingWriter{}); err == nil {
			t.Errorf("lost output error: %s", line)
		}
	}
}

func TestHawkCommands(t *testing.T) {
	sh, _ := fixture(t)
	image, err := hawk.DemoImage(512)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "demo hawk.img")
	if err := os.WriteFile(source, image, 0600); err != nil {
		t.Fatal(err)
	}
	command(t, sh, "mount -t hawk '"+source+"' /hawk")
	if got := command(t, sh, "ls /hawk"); got != "HELLO\nNOTES\nNUMBERS\nREADME\n" {
		t.Fatal(got)
	}
	if got := command(t, sh, "text /hawk/HELLO"); got != "HELLO FROM CORDWRIGHT!\n" {
		t.Fatal(got)
	}
	if got := command(t, sh, "cat /hawk/HELLO"); !bytes.Equal([]byte(got), image[33*512:33*512+400]) {
		t.Fatal("cat decoded bytes")
	}
	if got := command(t, sh, "sector /hawk 0x21"); !strings.Contains(got, "c8 c5 cc cc cf") || !strings.Contains(got, "000001f0") {
		t.Fatal(got)
	}
	if got := command(t, sh, "sectors /hawk/NOTES"); got != "0: 34 (0x22)\n1: 40 (0x28)\n2: 35 (0x23)\n" {
		t.Fatal(got)
	}
	for _, line := range []string{"sector /hawk -1", "sector /hawk 12992", "sector /hawk/HELLO 0", "sector /h0 0", "sectors /h0/raw", "text /h0/raw", "mount -t other x /x", "mount -t hawk -stride 12 x /x"} {
		args, _ := Words(line)
		if _, err := sh.Execute(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %s", line)
		}
	}
	command(t, sh, "cd /hawk")
	command(t, sh, "umount /hawk")
	if sh.Pwd() != "/" {
		t.Fatal(sh.Pwd())
	}
}
