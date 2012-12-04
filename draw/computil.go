package draw

import "image"

// Compressed image file parameters.
const (
	NMATCH  = 3             /* shortest match possible */
	NRUN    = (NMATCH + 31) /* longest match possible */
	NMEM    = 1024          /* window size */
	NDUMP   = 128           /* maximum length of dump */
	NCBLOCK = 6000          /* size of compressed blocks */
)

/*
 * compressed data are sequences of byte codes.
 * if the first byte b has the 0x80 bit set, the next (b^0x80)+1 bytes
 * are data.  otherwise, it's two bytes specifying a previous string to repeat.
 */

func twiddlecompressed(buf []byte) {
	i := 0
	for i < len(buf) {
		c := buf[i]
		i++
		if c >= 0x80 {
			k := int(c) - 0x80 + 1
			for j := 0; j < k && i < len(buf); j++ {
				buf[i] ^= 0xFF
				i++
			}
		} else {
			i++
		}
	}
}

func compblocksize(r image.Rectangle, depth int) int {
	bpl := BytesPerLine(r, depth)
	bpl = 2 * bpl /* add plenty extra for blocking, etc. */
	if bpl < NCBLOCK {
		return NCBLOCK
	}
	return bpl
}
