# Cordwright

> **Note:** This software is built with the assistance of AI tools.

A retro-media workbench for disk and tape images, from sectors, records, and
blocks to filesystems and decoded content.

## Status

Initial Go implementation: read-only host-folder drives, an experimental Hawk
Disk6 driver, a synthetic Hawk image generator, and a Unix-style shell. Hawk
layout knowledge comes from CPU6.dos in [tricorn](https://github.com/rsbohn/tricorn).
Simple Hawk libraries are browsable as directories. TU56/DECtape container images
can be mounted for raw block inspection; OS/8 directory decoding and persistent
mount tables are not yet implemented.

See the [quick reference](docs/quickref.md) for commands and examples.

## Run

Requires Go 1.25 or later. No third-party dependencies.

```sh
go run ./cmd/cordwright -host h0=/path/to/folder
# Or build a standalone executable:
go build -o cordwright ./cmd/cordwright
./cordwright -host h0=/path/to/folder -host h1=/another/folder
```

The default (and currently only) personality is `unix`; it can also be selected
explicitly with `-personality unix`. Startup cwd is the virtual `/`, not the host
working directory. No host folder is exposed until mounted.

```text
mount                         # list mounts
ls /                          # list drives
cd /h0
ls -l
cat 'notes with spaces.txt'
hex data.bin
stat data.bin
cd ..
mount '/another/host folder' /archive
ls /archive
umount /archive
help
exit
```

`mount HOST-DIRECTORY /NAME` opens a read-only drive. Mount points are immediate
children of the virtual root. Duplicate mounts fail; use `umount` first to
replace one. Unmounting the drive containing cwd resets cwd to `/`.
Relative **host source** paths in `mount` resolve from the process's host working
directory; other commands resolve paths within the virtual namespace.

One-shot commands preserve the invoking shell's argument boundaries:

```sh
./cordwright -host h0=/path/to/folder ls -l /h0
./cordwright -host h0=/path/to/folder cat '/h0/notes with spaces.txt'
printf 'cd /h0\nls\npwd\n' | ./cordwright -host h0=/path/to/folder -batch
```

Flags go before the command. Non-terminal stdin automatically uses batch mode.
Batch mode processes commands as lines arrive, continues after command errors,
and exits nonzero if any command failed. Diagnostics go to stderr. One-shot
commands also return nonzero on failure.

### Unix personality scope

Supported commands: `mount`, `umount`, `pwd`, `cd`, `ls [-l]`, `cat`, `hex`,
`stat`, `help`, `exit`. Paths and commands are case-sensitive. `ls` includes
hidden entries; it accepts one path, not globs. `cat` accepts multiple files;
`hex` and `stat` accept one. Use `--` before literal paths beginning with `-`
for `ls`, `cat`, `hex`, or `stat`.

Interactive/batch command parsing supports single/double quotes, backslash
escapes outside single quotes, and `#` comments at word boundaries. This is a
small command interpreter, **not a POSIX shell**: there is no variable or tilde
expansion, globbing, pipe, redirection, command substitution, or external command
execution. Unquoted shell operators are rejected. Use your host shell for
redirection when invoking a one-shot command.

`cat` streams exact bytes without adding newlines or decoding text. Use `hex`
for unknown binary files: `cat` may emit terminal control sequences. File sizes
and modes shown by `stat`/`ls -l` describe the host files, not permission to write
through Cordwright. There are no write or deletion commands.

### Host drive boundaries

Host access uses Go's `os.Root` on native platforms to confine filesystem
operations to the mounted tree, including symlink resolution. Internal relative
symlinks work; links that escape the root or use absolute targets are rejected.
Virtual `..` navigation can reach `/` or another mounted drive, but cannot expose
unmounted host ancestors. Symlink working-directory navigation is lexical.

Use ordinary, trusted data folders. This is not an OS sandbox: bind mounts and
filesystem boundaries inside a mounted folder remain accessible. Special files
are rejected before opening, but a concurrently modified host tree can race that
check. Host folders are live views, not snapshots. Cordwright issues no content
writes; normal host filesystem read effects such as access-time updates may
still occur.

## Try a synthetic Hawk image

```sh
mkdir -p out
go run ./cmd/hawk-demo out/demo-hawk.img
go run ./cmd/cordwright -hawk hawk=out/demo-hawk.img
```

```text
ls /hawk
text /hawk/README
text /hawk/NOTES
hex /hawk/HELLO
sectors /hawk/NOTES
sector /hawk 0x0E
```

`cat` still returns exact allocated payload bytes; `text` is an explicit lossy
high-bit ASCII view. `sector` includes container padding, while `sectors` shows
file allocation order. These capabilities are unavailable on host-folder drives.

See [Hawk support and demo format](docs/hawk.md) for mount syntax, packed images,
format assumptions, and limitations. The demo is a **non-bootable reader fixture**,
not a disk formatter or a reconstruction of CENTOS.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
```

`internal/media` defines the read-only filesystem drive interface, optional
image capabilities, and host adapter. `internal/hawk` implements the Hawk driver
and deterministic demo layout. `internal/shell` implements the Unix personality
separately from the drivers. `cmd/cordwright` wires startup mounts and
CLI/batch/interactive modes; `cmd/hawk-demo` generates synthetic fixtures.
`internal/tu56` implements a raw TU56/DECtape container view (`README.txt` plus
`blocks/NNNN.bin` and `blocks/NNNN.oct`); mount with `-tu56 NAME=PATH` or
`mount -t tu56 IMAGE /NAME`.

## The workbench

Cordwright will provide two connected views of historical media:

- **Media-level inspection:** sectors, blocks, tape records, filemarks, geometry,
  and image-container metadata.
- **Filesystem-level inspection:** volumes, directories, files, libraries,
  allocation maps, and decoded content.

The connection matters: inspect a file and see its backing sectors or records;
inspect a block and identify its owner where the format permits.

## Design

Keep three concerns separate:

1. **Image container:** how a host file represents disk sectors or tape records.
2. **On-media format:** how the original system organizes volumes and files.
3. **Content interpretation:** text encodings, structured records, and executable
   formats.

Format drivers should handle system-specific rules. Shell interfaces can offer
neutral workbench commands or a familiar personality such as CPU6.dos without
embedding filesystem logic in command handlers.

Tape support must preserve record boundaries and filemarks rather than treating
a tape as a disk with different sector sizes. Recognizing a tape container does
not imply recognizing the filesystem or application records inside it.

## Principles

- **Read-only first.** Inspection never modifies source images. Repair and editing
  are future work requiring explicit safeguards.
- **Preserve bytes.** Lossless extraction is distinct from text decoding, padding
  removal, and sector-slack trimming.
- **Show provenance.** Report the sectors or records behind extracted content.
- **Make uncertainty visible.** Format detection should explain its evidence and
  permit explicit format and geometry overrides.
- **Handle corruption explicitly.** Strict extraction should fail clearly;
  recovery modes must label partial results rather than silently truncate them.
- **Do not execute media content.** CPU emulation is a separate integration, not
  a side effect of browsing.
- **Respect historical data.** Use synthetic test fixtures and avoid publishing
  personal or business records recovered from original media.

## Next steps

- Extend Hawk library support with more format evidence and edge cases.
- Add a dedicated extraction command; currently one-shot `cat` can be redirected
  by the host shell to preserve allocated file payloads.
- Add a record-oriented tape container, such as SIMH `.tap`, to test the design
  against a genuinely different medium.
- Extend format support as documentation and synthetic tests become available.

Persistent mount configuration can build on CPU6.dos's manually edited,
read-only `fstab` approach. Current mounts are session-only; Cordwright does not
read CPU6.dos configuration. The broader command set and configuration format
remain under development.
