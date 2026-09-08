// os8-dir-dump inspects OS/8 directories in SIMH PDP-8 DECtape images.
package os8

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

const physicalBytes = 129 * 2

type entry struct {
	Name          string
	Start, Length int
	Extra         []uint16
}
type segment struct {
	Block, Next, Start, Extra int
	Header                    []uint16
	Entries                   []entry
}

// OS/8 uses the first 128 words of each 129-word DECtape block,
// pairing consecutive physical blocks into a 256-word logical block.
func logicalBlocks(raw []byte) ([][]uint16, error) {
	if len(raw) == 0 || len(raw)%(2*physicalBytes) != 0 {
		return nil, fmt.Errorf("size %d: expected complete pairs of 258-byte SIMH DECtape blocks", len(raw))
	}
	blocks := make([][]uint16, len(raw)/(2*physicalBytes))
	for n := range blocks {
		blocks[n] = make([]uint16, 256)
		for i := 0; i < 256; i++ {
			offset := (n*2+i/128)*physicalBytes + (i%128)*2
			w := binary.LittleEndian.Uint16(raw[offset:])
			if w > 07777 {
				return nil, fmt.Errorf("non-12-bit word at byte %d", offset)
			}
			blocks[n][i] = w
		}
	}
	return blocks, nil
}

func six(w uint16) string {
	var out [2]byte
	for i, c := range []byte{byte(w >> 6), byte(w & 077)} {
		switch {
		case c == 0:
			out[i] = ' '
		case c < 040:
			out[i] = c + 0100
		default:
			out[i] = c
		}
	}
	return string(out[:])
}

func filename(words []uint16) (string, error) {
	name := strings.TrimRight(six(words[0])+six(words[1])+six(words[2]), " ")
	ext := strings.TrimRight(six(words[3]), " ")
	if name == "" {
		return "", fmt.Errorf("empty filename")
	}
	for _, c := range name + ext {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '$' || c == '%') {
			return "", fmt.Errorf("unrecognized filename characters in %q.%q", name, ext)
		}
	}
	if ext != "" {
		name += "." + ext
	}
	return name, nil
}

func negative(w uint16) int { return int((-w) & 07777) }

func parseSegment(blocks [][]uint16, n int) (segment, error) {
	if n < 1 || n > 6 || n >= len(blocks) {
		return segment{}, fmt.Errorf("directory block %d out of range", n)
	}
	b := blocks[n]
	s := segment{Block: n, Start: int(b[1]), Next: int(b[2]), Extra: negative(b[4]), Header: append([]uint16(nil), b[:5]...)}
	count := negative(b[0])
	if count == 0 || count > 125 || s.Extra > 250 || s.Next > 6 || s.Start < 7 || s.Start > len(blocks) {
		return s, fmt.Errorf("invalid directory header at block %d", n)
	}
	// Header word 3 is preserved but not interpreted here.
	p, cur := 5, s.Start
	for i := 0; i < count; i++ {
		if p >= 256 {
			return s, fmt.Errorf("entry %d overruns directory block %d", i, n)
		}
		e := entry{Start: cur}
		if b[p] == 0 {
			if p+2 > 256 {
				return s, fmt.Errorf("truncated free entry")
			}
			e.Name = "<FREE>"
			e.Length = negative(b[p+1])
			p += 2
		} else {
			end := p + 5 + s.Extra
			if end > 256 {
				return s, fmt.Errorf("truncated file entry")
			}
			var err error
			e.Name, err = filename(b[p : p+4])
			if err != nil {
				return s, err
			}
			e.Extra = append([]uint16(nil), b[p+4:end-1]...)
			e.Length = negative(b[end-1])
			p = end
		}
		if e.Length > len(blocks)-cur {
			return s, fmt.Errorf("%s allocation exceeds volume", e.Name)
		}
		cur += e.Length
		s.Entries = append(s.Entries, e)
	}
	return s, nil
}

func Dump(out io.Writer, raw []byte) error {
	blocks, err := logicalBlocks(raw)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "SIMH little-endian DECtape: %d physical blocks, %d OS/8 logical blocks\n", len(blocks)*2, len(blocks))
	seen := map[int]bool{}
	for n := 1; n != 0; {
		if seen[n] {
			return fmt.Errorf("directory chain cycle at block %d", n)
		}
		seen[n] = true
		s, err := parseSegment(blocks, n)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "\nDirectory block %d (physical %d,%d), next=%d extra-words=%d\n", n, n*2, n*2+1, s.Next, s.Extra)
		fmt.Fprintf(out, "  header (octal): %04o %04o %04o %04o %04o\n", s.Header[0], s.Header[1], s.Header[2], s.Header[3], s.Header[4])
		fmt.Fprintln(out, "  NAME       START  BLOCKS  EXTRA (octal)")
		for _, e := range s.Entries {
			fmt.Fprintf(out, "  %-10s %5d %7d  %04o\n", e.Name, e.Start, e.Length, e.Extra)
		}
		n = s.Next
	}
	return nil
}
