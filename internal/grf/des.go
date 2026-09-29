package grf

// This file ports the modified DES and the block shuffle that GRF files use
// to hide some entries. The source is rAthena's src/common/des.cpp and
// src/common/grfio.cpp.
//
// The cipher has one round, so decrypting a block twice gives it back.
// That makes the same function usable for encrypting test files.

var bitMask = [8]byte{0x80, 0x40, 0x20, 0x10, 0x08, 0x04, 0x02, 0x01}

var ipTable = [64]byte{
	58, 50, 42, 34, 26, 18, 10, 2,
	60, 52, 44, 36, 28, 20, 12, 4,
	62, 54, 46, 38, 30, 22, 14, 6,
	64, 56, 48, 40, 32, 24, 16, 8,
	57, 49, 41, 33, 25, 17, 9, 1,
	59, 51, 43, 35, 27, 19, 11, 3,
	61, 53, 45, 37, 29, 21, 13, 5,
	63, 55, 47, 39, 31, 23, 15, 7,
}

var fpTable = [64]byte{
	40, 8, 48, 16, 56, 24, 64, 32,
	39, 7, 47, 15, 55, 23, 63, 31,
	38, 6, 46, 14, 54, 22, 62, 30,
	37, 5, 45, 13, 53, 21, 61, 29,
	36, 4, 44, 12, 52, 20, 60, 28,
	35, 3, 43, 11, 51, 19, 59, 27,
	34, 2, 42, 10, 50, 18, 58, 26,
	33, 1, 41, 9, 49, 17, 57, 25,
}

var tpTable = [32]byte{
	16, 7, 20, 21,
	29, 12, 28, 17,
	1, 15, 23, 26,
	5, 18, 31, 10,
	2, 8, 24, 14,
	32, 27, 3, 9,
	19, 13, 30, 6,
	22, 11, 4, 25,
}

// sTable holds the S-boxes, two nibbles per entry.
var sTable = [4][64]byte{
	{
		0xef, 0x03, 0x41, 0xfd, 0xd8, 0x74, 0x1e, 0x47, 0x26, 0xef, 0xfb, 0x22, 0xb3, 0xd8, 0x84, 0x1e,
		0x39, 0xac, 0xa7, 0x60, 0x62, 0xc1, 0xcd, 0xba, 0x5c, 0x96, 0x90, 0x59, 0x05, 0x3b, 0x7a, 0x85,
		0x40, 0xfd, 0x1e, 0xc8, 0xe7, 0x8a, 0x8b, 0x21, 0xda, 0x43, 0x64, 0x9f, 0x2d, 0x14, 0xb1, 0x72,
		0xf5, 0x5b, 0xc8, 0xb6, 0x9c, 0x37, 0x76, 0xec, 0x39, 0xa0, 0xa3, 0x05, 0x52, 0x6e, 0x0f, 0xd9,
	}, {
		0xa7, 0xdd, 0x0d, 0x78, 0x9e, 0x0b, 0xe3, 0x95, 0x60, 0x36, 0x36, 0x4f, 0xf9, 0x60, 0x5a, 0xa3,
		0x11, 0x24, 0xd2, 0x87, 0xc8, 0x52, 0x75, 0xec, 0xbb, 0xc1, 0x4c, 0xba, 0x24, 0xfe, 0x8f, 0x19,
		0xda, 0x13, 0x66, 0xaf, 0x49, 0xd0, 0x90, 0x06, 0x8c, 0x6a, 0xfb, 0x91, 0x37, 0x8d, 0x0d, 0x78,
		0xbf, 0x49, 0x11, 0xf4, 0x23, 0xe5, 0xce, 0x3b, 0x55, 0xbc, 0xa2, 0x57, 0xe8, 0x22, 0x74, 0xce,
	}, {
		0x2c, 0xea, 0xc1, 0xbf, 0x4a, 0x24, 0x1f, 0xc2, 0x79, 0x47, 0xa2, 0x7c, 0xb6, 0xd9, 0x68, 0x15,
		0x80, 0x56, 0x5d, 0x01, 0x33, 0xfd, 0xf4, 0xae, 0xde, 0x30, 0x07, 0x9b, 0xe5, 0x83, 0x9b, 0x68,
		0x49, 0xb4, 0x2e, 0x83, 0x1f, 0xc2, 0xb5, 0x7c, 0xa2, 0x19, 0xd8, 0xe5, 0x7c, 0x2f, 0x83, 0xda,
		0xf7, 0x6b, 0x90, 0xfe, 0xc4, 0x01, 0x5a, 0x97, 0x61, 0xa6, 0x3d, 0x40, 0x0b, 0x58, 0xe6, 0x3d,
	}, {
		0x4d, 0xd1, 0xb2, 0x0f, 0x28, 0xbd, 0xe4, 0x78, 0xf6, 0x4a, 0x0f, 0x93, 0x8b, 0x17, 0xd1, 0xa4,
		0x3a, 0xec, 0xc9, 0x35, 0x93, 0x56, 0x7e, 0xcb, 0x55, 0x20, 0xa0, 0xfe, 0x6c, 0x89, 0x17, 0x62,
		0x17, 0x62, 0x4b, 0xb1, 0xb4, 0xde, 0xd1, 0x87, 0xc9, 0x14, 0x3c, 0x4a, 0x7e, 0xa8, 0xe2, 0x7d,
		0xa0, 0x9f, 0xf6, 0x5c, 0x6a, 0x09, 0x8d, 0xf0, 0x0f, 0xe3, 0x53, 0x25, 0x95, 0x36, 0x28, 0xcb,
	},
}

type block [8]byte

func permute(src *block, table []byte) block {
	var out block
	for i, t := range table {
		j := t - 1
		if src[(j>>3)&7]&bitMask[j&7] != 0 {
			out[(i>>3)&7] |= bitMask[i&7]
		}
	}
	return out
}

// roundFunction XORs b[0..3] with TP(SBOX(E(b[4..7]))).
func roundFunction(b *block) {
	// Expansion of the right half into eight 6-bit values.
	var e block
	e[0] = ((b[7] << 5) | (b[4] >> 3)) & 0x3f
	e[1] = ((b[4] << 1) | (b[5] >> 7)) & 0x3f
	e[2] = ((b[4] << 5) | (b[5] >> 3)) & 0x3f
	e[3] = ((b[5] << 1) | (b[6] >> 7)) & 0x3f
	e[4] = ((b[5] << 5) | (b[6] >> 3)) & 0x3f
	e[5] = ((b[6] << 1) | (b[7] >> 7)) & 0x3f
	e[6] = ((b[6] << 5) | (b[7] >> 3)) & 0x3f
	e[7] = ((b[7] << 1) | (b[4] >> 7)) & 0x3f

	var s block
	for i := range sTable {
		s[i] = (sTable[i][e[i*2]] & 0xf0) | (sTable[i][e[i*2+1]] & 0x0f)
	}

	// The P-box writes into the right half of its output.
	var tp block
	for i, t := range tpTable {
		j := t - 1
		if s[j>>3]&bitMask[j&7] != 0 {
			tp[(i>>3)+4] |= bitMask[i&7]
		}
	}

	b[0] ^= tp[4]
	b[1] ^= tp[5]
	b[2] ^= tp[6]
	b[3] ^= tp[7]
}

// desBlock decrypts one block in place. It is its own inverse.
func desBlock(p []byte) {
	b := block(p[:8])
	b = permute(&b, ipTable[:])
	roundFunction(&b)
	b = permute(&b, fpTable[:])
	copy(p, b[:])
}

// substitute swaps some byte values for others. It is its own inverse.
func substitute(in byte) byte {
	switch in {
	case 0x00:
		return 0x2B
	case 0x2B:
		return 0x00
	case 0x6C:
		return 0x80
	case 0x80:
		return 0x6C
	case 0x01:
		return 0x68
	case 0x68:
		return 0x01
	case 0x48:
		return 0x77
	case 0x77:
		return 0x48
	case 0x60:
		return 0xFF
	case 0xFF:
		return 0x60
	case 0xB9:
		return 0xC0
	case 0xC0:
		return 0xB9
	case 0xFE:
		return 0xEB
	case 0xEB:
		return 0xFE
	}
	return in
}

func shuffleDecode(p []byte) {
	var out block
	out[0] = p[3]
	out[1] = p[4]
	out[2] = p[6]
	out[3] = p[0]
	out[4] = p[1]
	out[5] = p[2]
	out[6] = p[5]
	out[7] = substitute(p[7])
	copy(p, out[:])
}

func shuffleEncode(p []byte) {
	var out block
	out[0] = p[3]
	out[1] = p[4]
	out[2] = p[5]
	out[3] = p[0]
	out[4] = p[1]
	out[5] = p[6]
	out[6] = p[2]
	out[7] = substitute(p[7])
	copy(p, out[:])
}

// mixedCycle returns how many blocks apart the DES-encrypted blocks are in
// a mixed-encrypted entry, from the number of digits in its compressed size.
func mixedCycle(size uint32) int {
	digits := 1
	for i := uint64(10); i <= uint64(size); i *= 10 {
		digits++
	}
	switch {
	case digits < 3:
		return 1
	case digits < 5:
		return digits + 1
	case digits < 7:
		return digits + 9
	}
	return digits + 15
}

// codeEntry removes GRF encryption from an entry's data in place.
// With encode set, it adds the encryption instead, for writing test files.
func codeEntry(buf []byte, flags uint8, compressedSize uint32, encode bool) {
	nblocks := len(buf) / 8
	switch {
	case flags&FlagMixedCrypt != 0:
		cycle := mixedCycle(compressedSize)
		for i := 0; i < 20 && i < nblocks; i++ {
			desBlock(buf[i*8:])
		}
		j := -1
		for i := 20; i < nblocks; i++ {
			if i%cycle == 0 {
				desBlock(buf[i*8:])
				continue
			}
			j++
			if j%7 == 0 && j != 0 {
				if encode {
					shuffleEncode(buf[i*8:])
				} else {
					shuffleDecode(buf[i*8:])
				}
			}
		}
	case flags&FlagHeaderCrypt != 0:
		for i := 0; i < 20 && i < nblocks; i++ {
			desBlock(buf[i*8:])
		}
	}
}
