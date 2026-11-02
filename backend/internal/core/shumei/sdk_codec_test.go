package shumei

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func decodeSDKData(t *testing.T, data, uid string) map[string]interface{} {
	t.Helper()
	encrypted, err := hex.DecodeString(data)
	if err != nil || len(encrypted) == 0 || len(encrypted)%16 != 0 {
		t.Fatalf("invalid ciphertext: %v", err)
	}
	block, err := aes.NewCipher([]byte(md5Hex(uid)[:16]))
	if err != nil {
		t.Fatal(err)
	}
	cipher.NewCBCDecrypter(block, []byte("0102030405060708")).CryptBlocks(encrypted, encrypted)
	compressed, err := base64.StdEncoding.DecodeString(string(bytes.TrimRight(encrypted, "\x00")))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	plain, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var profile map[string]interface{}
	if err := json.Unmarshal(plain, &profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestCodecMatchesCapturedOfficialSDKFunctions(t *testing.T) {
	data, err := os.ReadFile("testdata/official_codec_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	type vector struct{ Text, Key, Cipher string }
	var vectors struct {
		DES, AES []vector
		Full     []struct {
			UID, Checksum, Data string
			Profile, Obfuscated map[string]interface{}
		}
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors.DES {
		got, err := sdkDES(v.Key, v.Text)
		if err != nil || got != v.Cipher {
			t.Errorf("DES(%q)=%q want %q err=%v", v.Text, got, v.Cipher, err)
		}
	}
	for _, v := range vectors.AES {
		got, err := sdkAES(v.Text, v.Key)
		if err != nil || got != v.Cipher {
			t.Errorf("AES(%q)=%q want %q err=%v", v.Text, got, v.Cipher, err)
		}
	}
	for i, v := range vectors.Full {
		if got := sdkChecksum(v.Profile); got != v.Checksum {
			t.Errorf("vector %d checksum=%s want %s", i, got, v.Checksum)
		}
		encoded, err := sdkData(v.Profile, v.UID)
		if err != nil {
			t.Fatal(err)
		}
		// Go and pako use different legal DEFLATE streams / JSON key ordering.
		// Compare their decoded objects, not coincidental ciphertext equality.
		if got := decodeSDKData(t, encoded, v.UID); !reflect.DeepEqual(got, v.Obfuscated) {
			t.Errorf("vector %d differs from official field mapping: got=%v want=%v", i, got, v.Obfuscated)
		}
		if got := decodeSDKData(t, v.Data, v.UID); !reflect.DeepEqual(got, v.Obfuscated) {
			t.Errorf("official vector %d cannot be decoded", i)
		}
		if _, present := v.Profile["tn"]; present {
			t.Fatal("encoding mutated caller profile")
		}
	}
}

func TestGeneratedProfileUsesNormalSDKEnvelopeAndFreshKeys(t *testing.T) {
	first, err := generateDeviceProfileForPage("fixture-agent", "https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin")
	if err != nil {
		t.Fatal(err)
	}
	second, err := generateDeviceProfileForPage("fixture-agent", "https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin")
	if err != nil {
		t.Fatal(err)
	}
	var a, b deviceProfileRequest
	if err := json.Unmarshal([]byte(first), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(second), &b); err != nil {
		t.Fatal(err)
	}
	if a.Encode != 5 || a.Compress != 2 || a.EP == b.EP || a.Data == b.Data || a.AppID != appID || a.Organization != organization {
		t.Fatal("invalid normal SDK envelope or reused keys")
	}
	if _, err := hex.DecodeString(a.Data); err != nil || strings.Contains(a.Data, "fixture-agent") {
		t.Fatal("profile is not encoded")
	}
	if ep, err := base64.StdEncoding.DecodeString(a.EP); err != nil || len(ep) != 128 {
		t.Fatal("wrong RSA envelope")
	}
}

func BenchmarkGenerateSDKProfile(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := generateDeviceProfile("fixture-agent"); err != nil {
			b.Fatal(err)
		}
	}
}
