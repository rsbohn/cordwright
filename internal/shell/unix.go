// Package shell implements Cordwright's Unix-style command personality.
package shell

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/rsbohn/cordwright/internal/media"
)

const Help = `Cordwright — read-only Unix personality
  mount                         List mounted drives
  mount HOST-DIRECTORY /NAME     Mount a host folder read-only
  mount -t hawk [-stride 400|512] IMAGE /NAME
                                Mount an experimental Hawk image read-only
  mount -t tu56 [-o raw] IMAGE /NAME
                                Mount SIMH OS/8 files (raw: block view)
  text [--] FILE                Decode Hawk high-bit ASCII (lossy view)
  sectors [--] FILE             Show allocation-order sector numbers
  sector /NAME NUMBER           Hex dump image record (decimal or 0xHEX)
  umount /NAME                  Unmount a drive
  pwd                           Show virtual working directory
  cd [PATH]                     Change directory (default /)
  ls [-l] [--] [PATH]            List directory or file
  cat [--] FILE...              Stream exact file bytes
  hex [--] FILE                 Hex dump file bytes
  stat [--] PATH                Show file metadata
  help                          Show this reference
  exit                          Leave the workbench
Paths are case-sensitive. / lists drives. Relative paths use the virtual cwd.
Quotes and backslash escapes work; expansion, pipes, and redirection do not.
`

type Unix struct {
	drives map[string]media.Drive
	cwd    string
}

func NewUnix() *Unix        { return &Unix{drives: map[string]media.Drive{}, cwd: "/"} }
func (s *Unix) Pwd() string { return s.cwd }
func (s *Unix) Close() error {
	var errs []error
	for name, drive := range s.drives {
		errs = append(errs, drive.Close())
		delete(s.drives, name)
	}
	s.cwd = "/"
	return errors.Join(errs...)
}

func mountName(point string) (string, error) {
	if !strings.HasPrefix(point, "/") {
		return "", fmt.Errorf("mount point must be /NAME")
	}
	name := strings.TrimPrefix(point, "/")
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return "", fmt.Errorf("mount point must be /NAME")
	}
	return name, nil
}

func (s *Unix) MountHost(source, point string) error {
	return s.mount(point, func() (media.Drive, error) { return media.OpenHost(source) })
}
func (s *Unix) MountHawk(source, point string, stride int) error {
	return s.mount(point, func() (media.Drive, error) { return media.OpenHawk(source, stride) })
}
func (s *Unix) MountTU56(source, point string) error {
	return s.mount(point, func() (media.Drive, error) { return media.OpenTU56(source) })
}
func (s *Unix) MountTU56Raw(source, point string) error {
	return s.mount(point, func() (media.Drive, error) { return media.OpenTU56Raw(source) })
}
func (s *Unix) mount(point string, open func() (media.Drive, error)) error {
	name, err := mountName(point)
	if err != nil {
		return err
	}
	if _, ok := s.drives[name]; ok {
		return fmt.Errorf("%s: already mounted", point)
	}
	drive, err := open()
	if err != nil {
		return err
	}
	s.drives[name] = drive
	return nil
}

func (s *Unix) absolute(p string) string {
	if strings.HasPrefix(p, "/") {
		return path.Clean(p)
	}
	return path.Join(s.cwd, p)
}

func (s *Unix) resolve(p string) (media.Drive, string, error) {
	absolute := s.absolute(p)
	name, rest, _ := strings.Cut(strings.TrimPrefix(absolute, "/"), "/")
	drive, ok := s.drives[name]
	if !ok {
		return nil, "", fmt.Errorf("%s: %w", absolute, fs.ErrNotExist)
	}
	if rest == "" {
		rest = "."
	}
	return drive, rest, nil
}

func (s *Unix) names() []string {
	names := make([]string, 0, len(s.drives))
	for name := range s.drives {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Execute accepts already-tokenized arguments, including CLI arguments whose
// quoting has already been handled by the caller's shell.
func (s *Unix) Execute(args []string, out io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	cmd, args := args[0], args[1:]
	usage := func(text string) (bool, error) { return false, fmt.Errorf("usage: %s", text) }
	switch cmd {
	case "exit":
		if len(args) != 0 {
			return usage("exit")
		}
		return true, nil
	case "help":
		if len(args) != 0 {
			return usage("help")
		}
		_, err := io.WriteString(out, Help)
		return false, err
	case "pwd":
		if len(args) != 0 {
			return usage("pwd")
		}
		_, err := fmt.Fprintln(out, s.cwd)
		return false, err
	case "mount":
		if len(args) == 0 {
			for _, name := range s.names() {
				if _, err := fmt.Fprintf(out, "%s on /%s\n", s.drives[name].Description(), name); err != nil {
					return false, err
				}
			}
			return false, nil
		}
		if len(args) >= 2 && args[0] == "-t" {
			typ := args[1]
			args = args[2:]
			switch typ {
			case "hawk":
				stride := 512
				if len(args) >= 2 && args[0] == "-stride" {
					n, err := strconv.Atoi(args[1])
					if err != nil {
						return false, err
					}
					stride = n
					args = args[2:]
				}
				if len(args) != 2 {
					return usage("mount -t hawk [-stride 400|512] IMAGE /NAME")
				}
				return false, s.MountHawk(args[0], args[1], stride)
			case "tu56":
				raw := false
				if len(args) > 0 && args[0] == "-o" {
					if len(args) < 2 || args[1] != "raw" {
						return usage("mount -t tu56 [-o raw] IMAGE /NAME")
					}
					raw = true
					args = args[2:]
				}
				if len(args) != 2 {
					return usage("mount -t tu56 [-o raw] IMAGE /NAME")
				}
				if raw {
					return false, s.MountTU56Raw(args[0], args[1])
				}
				return false, s.MountTU56(args[0], args[1])
			default:
				return false, fmt.Errorf("unsupported mount type %q", typ)
			}
		}
		if len(args) != 2 {
			return usage("mount HOST-DIRECTORY /NAME")
		}
		return false, s.MountHost(args[0], args[1])
	case "umount":
		if len(args) != 1 {
			return usage("umount /NAME")
		}
		point := s.absolute(args[0])
		name, err := mountName(point)
		if err != nil {
			return false, err
		}
		drive, ok := s.drives[name]
		if !ok {
			return false, fmt.Errorf("%s: not mounted", point)
		}
		if err := drive.Close(); err != nil {
			return false, err
		}
		delete(s.drives, name)
		if s.cwd == point || strings.HasPrefix(s.cwd, point+"/") {
			s.cwd = "/"
		}
		return false, nil
	case "cd":
		if len(args) > 1 {
			return usage("cd [PATH]")
		}
		target := "/"
		if len(args) == 1 {
			target = s.absolute(args[0])
		}
		if target != "/" {
			drive, name, err := s.resolve(target)
			if err != nil {
				return false, err
			}
			info, err := drive.Stat(name)
			if err != nil {
				return false, err
			}
			if !info.IsDir() {
				return false, fmt.Errorf("%s: not a directory", target)
			}
		}
		s.cwd = target
		return false, nil
	case "ls":
		long := false
		if len(args) > 0 && args[0] == "-l" {
			long = true
			args = args[1:]
		}
		args, err := operands(args)
		if err != nil {
			return false, err
		}
		if len(args) > 1 {
			return usage("ls [-l] [--] [PATH]")
		}
		target := s.cwd
		if len(args) == 1 {
			target = s.absolute(args[0])
		}
		return false, s.list(target, long, out)
	case "sector":
		if len(args) != 2 {
			return usage("sector /NAME NUMBER")
		}
		point := s.absolute(args[0])
		if _, err := mountName(point); err != nil {
			return false, err
		}
		drive, _, err := s.resolve(point)
		if err != nil {
			return false, err
		}
		reader, ok := drive.(media.SectorReader)
		if !ok {
			return false, fmt.Errorf("drive has no sector view")
		}
		number := args[1]
		base := 10
		if strings.HasPrefix(number, "0x") || strings.HasPrefix(number, "0X") {
			base = 16
			number = number[2:]
		}
		n, err := strconv.ParseInt(number, base, 64)
		if err != nil {
			return false, err
		}
		b, err := reader.Sector(n)
		if err != nil {
			return false, err
		}
		_, err = io.WriteString(out, hex.Dump(b))
		return false, err
	case "text", "sectors":
		args, err := operands(args)
		if err != nil {
			return false, err
		}
		if len(args) != 1 {
			return usage(cmd + " [--] FILE")
		}
		drive, name, err := s.resolve(args[0])
		if err != nil {
			return false, err
		}
		if cmd == "text" {
			reader, ok := drive.(media.TextReader)
			if !ok {
				return false, fmt.Errorf("drive has no text decoder; use cat for raw bytes")
			}
			text, err := reader.Text(name)
			if err != nil {
				return false, err
			}
			_, err = io.WriteString(out, text)
			return false, err
		}
		reader, ok := drive.(media.AllocationReader)
		if !ok {
			return false, fmt.Errorf("drive has no allocation map")
		}
		sectors, err := reader.Sectors(name)
		if err != nil {
			return false, err
		}
		for i, n := range sectors {
			if _, err := fmt.Fprintf(out, "%d: %d (0x%X)\n", i, n, n); err != nil {
				return false, err
			}
		}
		return false, nil
	case "stat", "cat", "hex":
		args, err := operands(args)
		if err != nil {
			return false, err
		}
		if len(args) == 0 || (cmd != "cat" && len(args) != 1) {
			return usage(cmd + " [--] PATH")
		}
		for _, target := range args {
			if cmd == "stat" && s.absolute(target) == "/" {
				if _, err := fmt.Fprintln(out, "dr-xr-xr-x 0 / (virtual mount directory)"); err != nil {
					return false, err
				}
				continue
			}
			drive, name, err := s.resolve(target)
			if err != nil {
				return false, err
			}
			if cmd == "stat" {
				info, err := drive.Stat(name)
				if err != nil {
					return false, err
				}
				if _, err := fmt.Fprintf(out, "%s %d %s %s\n", info.Mode(), info.Size(), info.ModTime().UTC().Format("2006-01-02T15:04:05Z"), s.absolute(target)); err != nil {
					return false, err
				}
			} else if err := stream(drive, name, out, cmd == "hex"); err != nil {
				return false, err
			}
		}
		return false, nil
	default:
		return false, fmt.Errorf("%s: unknown command (try help)", cmd)
	}
}

func operands(args []string) ([]string, error) {
	if len(args) > 0 && args[0] == "--" {
		return args[1:], nil
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("unsupported option %s (use -- for literal paths)", arg)
		}
	}
	return args, nil
}

func stream(drive media.Drive, name string, out io.Writer, dump bool) error {
	f, err := drive.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file", name)
	}
	if dump {
		writer := hex.Dumper(out)
		_, err := io.Copy(writer, f)
		return errors.Join(err, writer.Close())
	}
	_, err = io.Copy(out, f)
	return err
}

func (s *Unix) list(target string, long bool, out io.Writer) error {
	if target == "/" {
		for _, name := range s.names() {
			prefix := ""
			if long {
				prefix = "dr-xr-xr-x 0 "
			}
			if _, err := fmt.Fprintln(out, prefix+name+"/"); err != nil {
				return err
			}
		}
		return nil
	}
	drive, name, err := s.resolve(target)
	if err != nil {
		return err
	}
	info, err := drive.Stat(name)
	if err != nil {
		return err
	}
	printEntry := func(name string, mode fs.FileMode, size int64) error {
		if mode.IsDir() {
			name += "/"
		}
		if long {
			_, err := fmt.Fprintf(out, "%s %d %s\n", mode, size, name)
			return err
		}
		_, err := fmt.Fprintln(out, name)
		return err
	}
	if !info.IsDir() {
		return printEntry(path.Base(target), info.Mode(), info.Size())
	}
	entries, err := drive.ReadDir(name)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		mode, size := entry.Type(), int64(0)
		if long {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			mode, size = info.Mode(), info.Size()
		}
		if err := printEntry(entry.Name(), mode, size); err != nil {
			return err
		}
	}
	return nil
}

// Run streams commands. Batch errors are reported and accumulated while later
// commands remain usable. Interactive callers can continue after errors too.
func (s *Unix) Run(in io.Reader, out, diagnostics io.Writer, interactive bool) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	failed := false
	for {
		if interactive {
			if _, err := fmt.Fprintf(out, "cordwright:%s$ ", s.cwd); err != nil {
				return err
			}
		}
		if !scanner.Scan() {
			break
		}
		args, err := Words(scanner.Text())
		exit := false
		if err == nil {
			exit, err = s.Execute(args, out)
		}
		if err != nil {
			failed = true
			if _, writeErr := fmt.Fprintln(diagnostics, "cordwright:", err); writeErr != nil {
				return writeErr
			}
		}
		if exit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if failed && !interactive {
		return fmt.Errorf("one or more commands failed")
	}
	return nil
}
