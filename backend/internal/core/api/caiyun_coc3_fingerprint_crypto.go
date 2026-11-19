package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/bits"
	"sort"
	"strings"
)

// SM4's standard S-box; only CBC encryption is needed by the official SDK.
var coc3SM4Sbox = [256]byte{214, 144, 233, 254, 204, 225, 61, 183, 22, 182, 20, 194, 40, 251, 44, 5, 43, 103, 154, 118, 42, 190, 4, 195, 170, 68, 19, 38, 73, 134, 6, 153, 156, 66, 80, 244, 145, 239, 152, 122, 51, 84, 11, 67, 237, 207, 172, 98, 228, 179, 28, 169, 201, 8, 232, 149, 128, 223, 148, 250, 117, 143, 63, 166, 71, 7, 167, 252, 243, 115, 23, 186, 131, 89, 60, 25, 230, 133, 79, 168, 104, 107, 129, 178, 113, 100, 218, 139, 248, 235, 15, 75, 112, 86, 157, 53, 30, 36, 14, 94, 99, 88, 209, 162, 37, 34, 124, 59, 1, 33, 120, 135, 212, 0, 70, 87, 159, 211, 39, 82, 76, 54, 2, 231, 160, 196, 200, 158, 234, 191, 138, 210, 64, 199, 56, 181, 163, 247, 242, 206, 249, 97, 21, 161, 224, 174, 93, 164, 155, 52, 26, 85, 173, 147, 50, 48, 245, 140, 177, 227, 29, 246, 226, 46, 130, 102, 202, 96, 192, 41, 35, 171, 13, 83, 78, 111, 213, 219, 55, 69, 222, 253, 142, 47, 3, 255, 106, 114, 109, 108, 91, 81, 141, 27, 175, 146, 187, 221, 188, 127, 17, 217, 92, 65, 31, 16, 90, 216, 10, 193, 49, 136, 165, 205, 123, 189, 45, 116, 208, 18, 184, 229, 180, 176, 137, 105, 151, 74, 12, 150, 119, 126, 101, 185, 241, 9, 197, 110, 198, 132, 24, 240, 125, 236, 58, 220, 77, 32, 121, 238, 95, 62, 215, 203, 57, 72}

func coc3SM4Sub(x uint32) uint32 {
	return uint32(coc3SM4Sbox[byte(x>>24)])<<24 | uint32(coc3SM4Sbox[byte(x>>16)])<<16 | uint32(coc3SM4Sbox[byte(x>>8)])<<8 | uint32(coc3SM4Sbox[byte(x)])
}

// This upstream SDK uses JavaScript arithmetic >> in ROTL, rather than >>>.
// Preserve that legacy wire cipher exactly; it is not standard SM4 encryption.
func coc3SM4Rotate(value uint32, count int) uint32 {
	return value<<uint(count) | uint32(int32(value)>>uint(32-count))
}
func coc3SM4Keys(key []byte) ([32]uint32, error) {
	var rounds [32]uint32
	if len(key) != 16 {
		return rounds, fmt.Errorf("SM4 key length")
	}
	fk := [4]uint32{0xa3b1bac6, 0x56aa3350, 0x677d9197, 0xb27022dc}
	var k [36]uint32
	for i := 0; i < 4; i++ {
		k[i] = binary.BigEndian.Uint32(key[i*4:]) ^ fk[i]
	}
	for i := 0; i < 32; i++ {
		var ck uint32
		for j := 0; j < 4; j++ {
			ck |= uint32(byte((4*i+j)*7)) << uint(24-8*j)
		}
		b := coc3SM4Sub(k[i+1] ^ k[i+2] ^ k[i+3] ^ ck)
		k[i+4] = k[i] ^ b ^ coc3SM4Rotate(b, 13) ^ coc3SM4Rotate(b, 23)
		rounds[i] = k[i+4]
	}
	return rounds, nil
}
func coc3SM4Block(dst, src []byte, rounds [32]uint32) {
	var x [36]uint32
	for i := 0; i < 4; i++ {
		x[i] = binary.BigEndian.Uint32(src[i*4:])
	}
	for i := 0; i < 32; i++ {
		b := coc3SM4Sub(x[i+1] ^ x[i+2] ^ x[i+3] ^ rounds[i])
		x[i+4] = x[i] ^ b ^ coc3SM4Rotate(b, 2) ^ coc3SM4Rotate(b, 10) ^ coc3SM4Rotate(b, 18) ^ coc3SM4Rotate(b, 24)
	}
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(dst[i*4:], x[35-i])
	}
}
func coc3SM4CBC(plaintext []byte, bsk string) (string, error) {
	if len(bsk) != 32 {
		return "", fmt.Errorf("BSFIT encryption key length")
	}
	rounds, err := coc3SM4Keys([]byte(bsk[:16]))
	if err != nil {
		return "", err
	}
	padding := 16 - len(plaintext)%16
	data := make([]byte, len(plaintext)+padding)
	copy(data, plaintext)
	for i := len(plaintext); i < len(data); i++ {
		data[i] = byte(padding)
	}
	result := make([]byte, len(data))
	previous := []byte(bsk[16:])
	var block [16]byte
	for i := 0; i < len(data); i += 16 {
		for j := 0; j < 16; j++ {
			block[j] = data[i+j] ^ previous[j]
		}
		coc3SM4Block(result[i:i+16], block[:], rounds)
		previous = result[i : i+16]
	}
	return base64.StdEncoding.EncodeToString(result), nil
}

func coc3Murmur128(data []byte) string {
	const c1 uint64 = 0x87c37b91114253d5
	const c2 uint64 = 0x4cf5ad432745937f
	h1, h2 := uint64(31), uint64(31)
	blocks := len(data) / 16
	for i := 0; i < blocks; i++ {
		k1, k2 := binary.LittleEndian.Uint64(data[i*16:]), binary.LittleEndian.Uint64(data[i*16+8:])
		k1 *= c1
		k1 = bits.RotateLeft64(k1, 31)
		k1 *= c2
		h1 ^= k1
		h1 = bits.RotateLeft64(h1, 27)
		h1 += h2
		h1 = h1*5 + 0x52dce729
		k2 *= c2
		k2 = bits.RotateLeft64(k2, 33)
		k2 *= c1
		h2 ^= k2
		h2 = bits.RotateLeft64(h2, 31)
		h2 += h1
		h2 = h2*5 + 0x38495ab5
	}
	tail := data[blocks*16:]
	var k1, k2 uint64
	for i, b := range tail {
		if i < 8 {
			k1 |= uint64(b) << uint(8*i)
		} else {
			k2 |= uint64(b) << uint(8*(i-8))
		}
	}
	if len(tail) > 8 {
		k2 *= c2
		k2 = bits.RotateLeft64(k2, 33)
		k2 *= c1
		h2 ^= k2
	}
	if len(tail) > 0 {
		k1 *= c1
		k1 = bits.RotateLeft64(k1, 31)
		k1 *= c2
		h1 ^= k1
	}
	h1 ^= uint64(len(data))
	h2 ^= uint64(len(data))
	h1 += h2
	h2 += h1
	mix := func(x uint64) uint64 {
		x ^= x >> 33
		x *= 0xff51afd7ed558ccd
		x ^= x >> 33
		x *= 0xc4ceb9fe1a85ec53
		x ^= x >> 33
		return x
	}
	h1 = mix(h1)
	h2 = mix(h2)
	h1 += h2
	h2 += h1
	return fmt.Sprintf("%016x%016x", h1, h2)
}

type coc3DfpField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func coc3DfpHash(fields []coc3DfpField) string {
	fields = append([]coc3DfpField(nil), fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Key < fields[j].Key })
	var joined strings.Builder
	for _, field := range fields {
		joined.WriteString(field.Key)
		joined.WriteString(field.Value)
	}
	hash := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return base64.RawURLEncoding.EncodeToString(sum[:])
	}
	rotate := func(s string) string { n := (len(s) + 2) / 3; return s[2*n:] + s[:n] + s[n:2*n] }
	value := []byte(rotate(rotate(hash(joined.String()))))
	for i := 0; i < len(value)/2; i += 2 {
		value[i], value[len(value)-1-i] = value[len(value)-1-i], value[i]
	}
	return hash(hash(string(value)))
}
