package hawk

import "fmt"

// DemoImage builds an experimental Disk6 fixture, NOT a formatted/bootable
// CENTOS volume. All contents are synthetic; no historical image is required.
func DemoImage(stride int) ([]byte, error) {
	if stride != 400 && stride != 512 {
		return nil, fmt.Errorf("stride must be 400 or 512")
	}
	image := make([]byte, SectorLimit*stride)
	for n := 0; n < SectorLimit; n++ {
		for i := 400; i < stride; i++ {
			image[n*stride+i] = 0xff
		}
	}
	sector := func(n int) []byte { return image[n*stride : n*stride+400] }
	ctl := sector(14)
	// Observed Disk6 control fields resolving the volume directory to sector 16.
	copy(ctl[4:10], []byte{0xef, 0x2c, 0xe4, 0x54, 0xff, 0xff})
	vol := sector(16)
	putName(vol[:10], "CORDWRIGHT")
	put24(vol[13:16], 18) // UAL base
	files := []struct {
		name    string
		sectors []int
		pages   []string
	}{
		{"README", []int{32}, []string{"CORDWRIGHT SYNTHETIC HAWK DEMO\rExperimental reader fixture, not a bootable CENTOS disk.\rNo historical software or personal records.\r"}},
		{"HELLO", []int{33}, []string{"HELLO FROM CORDWRIGHT!\r"}},
		{"NOTES", []int{34, 40, 35}, []string{
			"NOTES PAGE 1: This file is deliberately fragmented.\rLogical sector payloads are 400 bytes.\r",
			"NOTES PAGE 2: Allocation order is 34, 40, 35.\rThe map uses a continuation in a second UAL sector.\r",
			"NOTES PAGE 3: Raw reads retain high bits, terminators, and slack.\rText decoding is a separate view.\r",
		}},
		{"NUMBERS", []int{36}, []string{"ID,VALUE\r001,10\r002,20\r003,30\r"}},
	}
	for i, file := range files {
		entry := vol[16+i*16 : 32+i*16]
		putName(entry[:10], file.name)
		off := i * 24
		entry[10] = byte(off / 3)
		entry[13] = 0x12 // provisional text/data attribute; driver does not infer decoding
		fmap := sector(18)
		put16(fmap[off:off+2], len(file.sectors)-1)
		put16(fmap[off+2:off+4], 0x8000)
		for j, n := range file.sectors {
			put24(fmap[off+6+j*3:], n)
		}
		end := off + 6 + 3*len(file.sectors)
		put24(fmap[end:], 0xffffff)
		if file.name == "NOTES" {
			// First extent is inline; redirect to UAL-relative sector 1, index 0.
			copy(fmap[off+9:off+12], []byte{0xff, 0xfe, 0})
			next := sector(19)
			put24(next[0:], 40)
			put24(next[3:], 35)
			put24(next[6:], 0xffffff)
		}
		for j, page := range file.pages {
			payload := sector(file.sectors[j])
			for k := range page {
				payload[k] = page[k] | 0x80
			}
			if j == len(file.pages)-1 {
				payload[len(page)] = 0x84
			}
		}
	}
	vol[80], vol[81] = 0x84, 0x8d
	return image, nil
}

func put16(b []byte, n int) { b[0], b[1] = byte(n>>8), byte(n) }
func put24(b []byte, n int) { b[0], b[1], b[2] = byte(n>>16), byte(n>>8), byte(n) }
func putName(b []byte, name string) {
	for i := range b {
		b[i] = 0xa0
	}
	for i := range name {
		b[i] = name[i] | 0x80
	}
}
