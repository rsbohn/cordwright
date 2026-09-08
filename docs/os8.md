# Exploratory OS/8 directory dump

```sh
go run ./cmd/os8-dir-dump /srv/media/os8/*.tu56
```

This read-only tool follows the directory chain starting at logical block 1.
It prints names, allocation starts and lengths (decimal logical blocks), plus
raw header and extra words (octal). It does not yet expose files in the shell.
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
