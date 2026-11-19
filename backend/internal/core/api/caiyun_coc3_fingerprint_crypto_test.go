package api

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestCoc3SM4PreservesSDKArithmeticRotation(t *testing.T) {
	key, _ := hex.DecodeString("0123456789abcdeffedcba9876543210")
	rounds, err := coc3SM4Keys(key)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, 16)
	coc3SM4Block(out, key, rounds)
	if hex.EncodeToString(out) != "314e6bc74f35ebe04f35f99f7b08d770" {
		t.Fatal("official SDK legacy rotation vector mismatch")
	}
}

func TestCoc3FingerprintCryptoMatchesOfficialSDK(t *testing.T) {
	for _, name := range []string{"bsfit-crypto-reference.json", "bsfit-long-wire-reference.json"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			var reference struct {
				Fields                           []coc3DfpField
				PacketFields                     []coc3DfpField
				Plaintext, Hash, BSK, Ciphertext string
				Murmur                           []struct{ Input, Hash string }
			}
			if json.Unmarshal(data, &reference) != nil {
				t.Fatal("invalid official BSFIT fixture")
			}
			if coc3DfpHash(reference.Fields) != reference.Hash {
				t.Fatal("official fingerprint field hash differs")
			}
			if len(reference.PacketFields) > 0 {
				body, err := coc3FingerprintJSON(reference.PacketFields)
				if err != nil || string(body) != reference.Plaintext {
					t.Fatal("official SDK plaintext string conversion differs")
				}
			}
			for _, vector := range reference.Murmur {
				if coc3Murmur128([]byte(vector.Input)) != vector.Hash {
					t.Fatal("official Fingerprint2 hash differs")
				}
			}
			ciphertext, err := coc3SM4CBC([]byte(reference.Plaintext), reference.BSK)
			if err != nil || ciphertext != reference.Ciphertext {
				t.Fatal("official SDK legacy SM4 CBC/PKCS7 packet differs")
			}
		})
	}
}
