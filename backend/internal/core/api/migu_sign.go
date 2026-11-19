package api

import (
	"encoding/binary"
	"encoding/hex"
	"math/bits"
	"strings"
)

// Public APK certificate prefix, not an account credential. Ported from the
// reference project's 04_脚本/protocol/mg_sign.py (libmgsign V005).
const miguCertificatePrefix = "308202333082019ca00302010202044d"
const miguSecurityKey = "ccTWaprX2aWmTIgA"

var miguSignConstants = [64]uint32{
	0xd76aa478, 0xe8c7b756, 0x242070db, 0xc1bdceee, 0xf57c0faf, 0x4787c62a, 0xa8304613, 0xfd469501,
	0x698098d8, 0x8b44f7af, 0xffff5bb1, 0x895cd7be, 0x6b901122, 0xfd987193, 0xa679438e, 0x49b40821,
	0xf61e2562, 0xc040b340, 0x265e5a51, 0xe9b6c7aa, 0xd62f105d, 0x02441453, 0xd8a1e681, 0xe7d3fbc8,
	0x21e1cde6, 0xc33707d6, 0xf4d50d87, 0x455a14ed, 0xa9e3e905, 0xfcefa3f8, 0x676f02d9, 0x8d2a4c8a,
	0xfffa3942, 0x8771f681, 0x6d9d6122, 0xfde5380c, 0xa4beea44, 0x4bdecfa9, 0xf6bb4b60, 0xbebfbc70,
	0x289b7ec6, 0xeaa127fa, 0xd4ef3085, 0x04881d05, 0xd9d4d039, 0xe6db99e5, 0x1fa27cf8, 0xc4ac5665,
	0xf4292244, 0x432aff97, 0xab9423a7, 0xfc93a039, 0x655b59c3, 0x8f0ccc92, 0xffeff47d, 0x85845dd1,
	0x6fa87e4f, 0xfe2ce6e0, 0xa3014314, 0x4e0811a1, 0xf7537e82, 0xbd3af235, 0x2ad7d2bb, 0xeb86d391,
}

func miguSign(data []byte, key string) string {
	if len(key) > 32 {
		key = key[:32]
	}
	input := append(append([]byte(nil), data...), []byte(key)...)
	state := [4]uint32{0x67452301, 0x10325476, 0x98badcfe, 0xefcdab89}
	z := byte(0x7a)
	shifts := [4][4]int{{7, 12, 17, 22}, {5, 9, 14, 20}, {4, 11, 16, 23}, {6, 10, 15, 21}}
	for len(input) > 0 {
		n := min(len(input), 64)
		var block [64]byte
		for i, ch := range input[:n] {
			z += 2*ch + 1
			block[i] = z
		}
		if n < 64 {
			block[n] = 0x80 // No bit-length trailer, and padding is not transformed.
		}
		input = input[n:]
		a, b, c, d := state[0], state[1], state[2], state[3]
		for i := 0; i < 64; i++ {
			var f uint32
			var g int
			switch {
			case i < 16:
				f, g = (b&c)|(^b&d), i
			case i < 32:
				f, g = (d&b)|(^d&c), (5*i+1)%16
			case i < 48:
				f, g = b^c^d, (3*i+5)%16
			default:
				f, g = c^(b|^d), (7*i)%16
			}
			f += a + miguSignConstants[i] + binary.LittleEndian.Uint32(block[g*4:])
			a, d, c, b = d, c, b, b+bits.RotateLeft32(f, shifts[i/16][i%4])
		}
		state[0] += a
		state[1] += b
		state[2] += c
		state[3] += d
	}
	var digest [16]byte
	for i, word := range state {
		binary.LittleEndian.PutUint32(digest[i*4:], word)
	}
	return strings.ToUpper(hex.EncodeToString(digest[:]))
}

func miguEncode(plain string) string {
	encoded := []byte(plain)
	for i := range encoded {
		encoded[i] += miguSecurityKey[i%len(miguSecurityKey)]
	}
	return strings.ToUpper(hex.EncodeToString(encoded))
}
