# Cordwright Quick Reference

Read-only retro-media workbench. Current support: host folders and experimental
Hawk/Disk6 top-level files, through the Unix personality.

## Start

From `/home/rsbohn/build/cordwright` (Go 1.25+):

```sh
go build -o cordwright ./cmd/cordwright
./cordwright -host h0=/path/to/folder
./cordwright -hawk hawk=out/demo-hawk.img
./cordwright -stride 400 -hawk hawk=out/demo-hawk-400.img
./cordwright -host h0=/path/to/folder -hawk hawk=out/demo-hawk.img
```

Or replace `./cordwright` with `go run ./cmd/cordwright`.

| Flag | Meaning |
| --- | --- |
| `-host NAME=PATH` | Mount a host folder; repeatable |
| `-hawk NAME=PATH` | Mount a Hawk image; repeatable |
| `-stride 400` / `-stride 512` | Record size for all CLI Hawk mounts; default 512 |
| `-personality unix` | Default and currently only personality |
| `-batch` | Read command lines without prompting |
| `-h` | Show CLI flags |

Flags precede command arguments. Supplying a command runs it once and exits:

```sh
./cordwright -hawk hawk=out/demo-hawk.img text /hawk/HELLO
printf 'cd /h0\nls\npwd\n' | ./cordwright -host h0=/path/to/folder -batch
```

Non-terminal stdin also selects batch mode. Batch errors go to stderr; later
commands continue, but the process exits nonzero if any command failed.

## Commands

```text
mount                               List mounted drives
mount HOST-DIRECTORY /NAME           Mount a host folder read-only
mount -t hawk IMAGE /NAME            Mount a Hawk image (512-byte stride)
mount -t hawk -stride 400 IMAGE /NAME Mount a packed Hawk image
umount /NAME                        Unmount; does not alter the source
pwd                                 Show virtual working directory
cd [PATH]                           Change directory; no argument means /
ls [-l] [--] [PATH]                  List directory or file; default cwd
cat [--] FILE...                     Stream exact file payload bytes
hex [--] FILE                       Hex dump file payload bytes
stat [--] PATH                      Show mode, size, timestamp, and path
text [--] FILE                      Decode Hawk high-bit ASCII (lossy)
sectors [--] FILE                   Show Hawk allocation order/provenance
sector /NAME NUMBER                 Hex dump a complete image record
help                                Show command syntax
exit                                Leave the workbench
```

`text`, `sectors`, and `sector` are Hawk capabilities, not host-folder operations.
`sector` accepts decimal or `0x` hexadecimal numbers; leading zeros are not octal.
`ls` includes hidden entries and accepts one path, not globs.

## Paths and mounts

- `/` is a virtual directory containing drives, not the host filesystem root.
  Startup cwd is `/`; no folders are exposed unless mounted.
- `/h0/sub/file` addresses a mounted file. Relative paths use virtual cwd.
- Commands and names are case-sensitive, including Hawk catalog names.
- Mount points are immediate children of `/`. Duplicate mounts fail rather than
  replace an existing drive.
- Host source paths in `mount` are relative to the process's host working
  directory, **not** virtual cwd. Quote paths containing spaces.
- Unmounting the drive containing cwd resets cwd to `/`.
- Mounts are session-only. There is no fstab loader yet.

```text
mount '/path/to/host folder' /archive
cd /archive
ls -l
cat 'notes with spaces.txt'
cat -- -filename
cd ..
umount /archive
```

Quotes, backslash escapes outside single quotes, and `#` comments at word
boundaries work in interactive/batch input. There is no variable/tilde expansion,
globbing, command substitution, external command execution, pipe, or redirection.
Unquoted shell operators are rejected. This is not a POSIX shell.

## Build and browse the demo Hawk image

```sh
mkdir -p out
go run ./cmd/hawk-demo out/demo-hawk.img
# Optional packed version:
go run ./cmd/hawk-demo -stride 400 out/demo-hawk-400.img
./cordwright -hawk hawk=out/demo-hawk.img
```

The generator refuses to overwrite an existing output. The image is deterministic,
synthetic, and **not bootable**. It contains `README`, `HELLO`, `NOTES`, and `NUMBERS`.

```text
ls /hawk
text /hawk/README
text /hawk/HELLO
text /hawk/NOTES
hex /hawk/HELLO
stat /hawk/NOTES
sectors /hawk/NOTES
sector /hawk 0x0E
```

`NOTES` spans sectors **34 → 40 → 35**, exercising fragmented allocation and a map
continuation. Its allocated payload size is 1,200 bytes.

## Know which bytes you are viewing

| View | Result |
| --- | --- |
| Host `cat` / `hex` | Original file bytes |
| Hawk `cat` / `hex` | 400-byte payloads in allocation order, including high bits, EOT, padding, and slack |
| Hawk `text` | Clears high bits, drops NULs, maps CR to LF, stops at EOT; no heuristic page/slack trimming |
| Hawk `sector` | Full 400- or 512-byte image record, including container padding |
| Hawk `sectors` | Allocation positions and decimal/hex sector numbers |

Hawk sizes are allocated sectors × 400, not inferred text lengths. Historical
timestamps are unknown and shown as zero time. Library containers remain raw
files. Simple Hawk libraries are browsable as subdirectories (for example `S`, `P`, and `USAGI` on observed images).

For lossless allocated-payload export, use **host-shell** redirection:

```sh
./cordwright -hawk hawk=out/demo-hawk.img cat /hawk/NOTES > notes.raw
```

Redirection can overwrite the destination. Never use the mounted image as the
output path. Cordwright has no write, deletion, or repair commands.

## Cautions and related docs

- Prefer `hex` for unknown content: `cat` and `text` may emit terminal controls.
- Do not publish personal/business records recovered from historical images.
- Host mounts confine paths and symlinks with `os.Root`, but are live views of
  trusted folders, not an OS sandbox or snapshot. See the README for boundaries.
- Do not modify images externally while mounted. Corrupt metadata and short reads
  fail explicitly; there is no recovery mode yet.

See [README](../README.md) for architecture and host-drive details, and
[Hawk support](hawk.md) for layout assumptions and driver limits.
