# SIMH OS/8 TU56 support

## Shell interface

```sh
go run ./cmd/cordwright -tu56 t0=/srv/media/os8/al-4691c-sa-os8-v3d-1.1978.tu56
```

Inside the shell:

```text
ls /t0
stat /t0/EPIC.PA
hex /t0/EPIC.PA
sectors /t0/EPIC.PA
mount -t tu56 -o raw /srv/media/os8/al-4691c-sa-os8-v3d-1.1978.tu56 /raw
ls /raw/blocks
cat /raw/blocks/0002.oct
```

`mount -t tu56 IMAGE /NAME` exposes OS/8 files by default. `-o raw` selects
the existing container view instead, without requiring a valid OS/8 directory.
For startup mounts, use `-o raw -tu56 NAME=PATH`; the option applies to **all**
startup TU56 mounts. Mount the image twice at different names to use both views.
Unknown mount options are rejected. No automatic fallback to raw occurs.

Names are case-sensitive and free-space entries are omitted. `ls`, `cd`, `stat`,
`cat`, `hex`, `sectors`, and `sector` use the existing shell interface.
`cat` preserves allocated file words as **little-endian 16-bit bytes**, skipping
the unused 129th word in each physical block: 512 bytes per OS/8 logical block.
Sizes include allocation padding; these are not decoded ASCII bytes. `text` is
not yet supported. Dates remain uninterpreted.

`sector` and `sectors` use **physical** DECtape block numbers (258 bytes each),
so a one-logical-block file has two sector numbers. Raw sectors retain all 129
words. Both views are read-only snapshots; source images are never modified.
Only SIMH PDP-8 OS/8 TU56 layout is supported, not generic SIMH tape containers
or other OS/8 disk geometries. Corrupt directories fail to mount (including
cycles, duplicate filenames, overlapping allocations, and out-of-range extents).

## Directory inspection tool

```sh
go run ./cmd/os8-dir-dump /srv/media/os8/*.tu56
```

This read-only tool follows the directory chain starting at logical block 1.
It prints names, allocation starts and lengths (decimal logical blocks), plus
raw header and extra words (octal). The same directory parser is shared with the shell driver.
The former heuristic scanner's `-min` and `-n` options have been removed.

The layout successfully decodes all seven local distribution images:

- SIMH PDP-8 DECtape: 129 little-endian 16-bit storage words per physical block.
- OS/8 logical block: first 128 words of physical block `2*n`, followed by
  first 128 words of `2*n+1`. The final word of each physical block is skipped
  by this logical view; the raw TU56 view preserves it.
- Five-word directory header: negative entry count, starting allocation block,
  next directory block (zero terminates), uninterpreted word, negative count
  of extra words per file entry. Negative counts use 12-bit two's complement.
- File entry: three name words, one extension word, extra words, negative length.
  Allocation starts accumulate from the header's starting block.
- Free entry: zero marker and negative length, with **no** extra words.
- Filename characters: six-bit ASCII (`01` = A, `20` = P); zero pads names.
  This is not ASCII-minus-040 encoding.
- Zero-length files are valid (including FORT.PA on distribution tape 1).

All seven images have 1,474 physical / 737 logical blocks, and their decoded
file/free allocations end at logical block 737. Extra words are shown without
attempting date interpretation. Filename validation is deliberately limited to
letters, digits, `$`, and `%`; this is an exploratory reader, not full OS/8 support.

Tests use synthetic data, not copies of historical software. The earlier lack
of scanner matches was caused by incorrect header and filename assumptions;
it was not evidence that these images lacked OS/8 filesystems.
