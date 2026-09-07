# Experimental Hawk support

Cordwright's first image driver implements the top-level Hawk/Disk6 catalog and
allocation traversal used by tricorn's CPU6.dos reader. It is **read-only**.
Format knowledge is incomplete: successful browsing is not a claim of complete
CENTOS filesystem or controller compatibility.

## Generate a demo

From the repository root:

```sh
mkdir -p out
go run ./cmd/hawk-demo out/demo-hawk.img
# Optional packed representation:
go run ./cmd/hawk-demo -stride 400 out/demo-hawk-400.img
```

Output must not exist: the generator uses exclusive creation and never overwrites
an existing image. It does not edit or format mounted drives. Output has no
host-dependent timestamps or randomness; repeated generation with the same
stride produces identical bytes. Generated `out/` files are ignored by Git.

The default image is 6,651,904 bytes: 12,992 records of 512 bytes, each containing
400 payload bytes and 112 bytes of `0xff` padding. Packed output is 5,196,800
bytes: the same payloads with a 400-byte stride and no container padding.

All contents are synthetic. There are no historical binaries or personal records.
The fixture is not bootable and omits a complete free-space map, loader, system
files, and controller checksums. Do not use it as a writable CENTOS volume.

### Demo layout

| Sector (decimal) | Purpose |
| --- | --- |
| 14 | Disk6 control fields, flag `0xffff` at offset 8 |
| 16 | `CORDWRIGHT` volume header and four catalog entries |
| 18 | UAL/FAL allocation headers and extent starts |
| 19 | Allocation continuation for `NOTES` |
| 32 | `README`: synthetic-image identification |
| 33 | `HELLO`: short greeting |
| 34, 40, 35 | `NOTES`: three pages, deliberately fragmented |
| 36 | `NUMBERS`: simple ID/value text records |

Catalog entries are 16 bytes; file names occupy ten high-bit ASCII bytes.
Map indices are multiples of three. UAL headers contain sector-count-minus-one,
a provisional load address, and an FSI shift of zero (one sector per extent).
`NOTES` redirects from its first extent to UAL-relative sector 1, offset 0.
Unused logical payload bytes are zero. Text has high bits set, CR line endings,
and EOT at the end of the last page. The reader does not rely on the fixture's
provisional file attributes to decide whether to decode text.

## Mount and browse

```sh
go run ./cmd/cordwright -hawk hawk=out/demo-hawk.img
go run ./cmd/cordwright -stride 400 -hawk hawk=out/demo-hawk-400.img
```

`-hawk NAME=PATH` is repeatable; `-stride` applies to all CLI Hawk mounts, defaults
to 512, and accepts only 400 or 512. Host and Hawk mounts share the same virtual
namespace; duplicate mount points are errors. Use runtime mounts for mixed strides:

```text
mount -t hawk out/demo-hawk.img /hawk
mount -t hawk -stride 400 out/demo-hawk-400.img /packed
ls /hawk
ls -l /hawk
stat /hawk/HELLO
text /hawk/HELLO
text /hawk/NOTES
hex /hawk/HELLO
sectors /hawk/NOTES
sector /hawk 14
sector /hawk 0x10
umount /hawk
```

Runtime source paths are host paths, relative to the process working directory;
quote paths containing spaces. Mount point paths belong to Cordwright's virtual
namespace. Hawk file names are case-sensitive in the Unix personality.

### Three different views

- `cat FILE` / `hex FILE`: concatenated **400-byte logical sector payloads in
  allocation order**, including high bits, EOT markers, zero padding, and slack.
  No decoding or trimming occurs. Reported file size is allocated sectors times
  400, not an inferred text length. This is lossless at the allocated-payload
  level, not a copy of container padding or filesystem metadata.
- `text FILE`: explicitly lossy high-bit ASCII view. Clears high bits, drops NUL
  bytes, maps CR to LF, and stops at the first EOT. Does **not** perform speculative
  CR-CR page/slack trimming or record-format interpretation. Use only for content
  known to be text; arbitrary records may contain control codes or personal data.
- `sector /MOUNT NUMBER`: hex dump of a complete image record, **including padding**.
  Numbers are decimal unless prefixed with `0x`; leading zeroes are not octal.

`sectors FILE` prints zero-based allocation position, decimal sector number, and
hex sector number. For the demo's `NOTES`, the result is `34, 40, 35`, not sorted
physical order. Metadata timestamps are unknown and displayed as Go's zero time;
no historical date encoding is invented.

Lossless allocated-payload export can use host-shell redirection:

```sh
go run ./cmd/cordwright -hawk hawk=out/demo-hawk.img cat /hawk/NOTES > notes.raw
```

The host shell may overwrite `notes.raw`; do not redirect onto the source image.
Cordwright itself never opens mounted images for writing.

## Reader limits and validation

- Explicit Hawk mounting; no automatic type or stride detection.
- Fixed catalog location at sector 16, following observed CPU6.dos images.
  Arbitrary control-derived catalog locations are not supported yet.
- Hawk library files with simple 16-byte catalogs are exposed as subdirectories
  (for example `/hawk/S`, `/hawk/P`, `/hawk/USAGI`). Library members are byte
  ranges in the parent library's allocated payload, using 200-byte page starts;
  the dot-member shortcut is not implemented.
- Positive allocation starts expand by `1 << FSI shift`; continuation locations
  are UAL-relative sector/index references. Allocation order is preserved.
- Images must be regular files made of complete 400- or 512-byte records, at most
  `0x32c0` records. Smaller synthetic fixtures are accepted if metadata is valid.
- Mount validates the Disk6 flag, terminated catalog (bounded at 800 entries),
  names, allocation headers, continuations, and data bounds. Cycles, premature
  terminators, repeated sectors within a file, and invalid maps fail explicitly.
  Cross-file overlaps and complete volume allocation consistency are not checked.
- Metadata/allocation maps are captured at mount; data is read through the open
  image handle. Do not modify images externally while mounted. Short reads fail;
  no partial file is silently returned by the driver.
- File opens currently buffer allocated payloads in memory (bounded by image size).

Tests use synthetic fixtures and exercise both strides, fragmented/continued
allocations, FSI expansion, more than 100 extents, corruption, truncation, fs.FS
contracts, exact bytes, CLI integration, and generator overwrite protection.
The generated default demo was also browsed with tricorn's CPU6.dos reader.
