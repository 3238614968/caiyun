package shumei

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Public field-obfuscation constants from the fp.min.js included in the user's
// HAR. These are protocol constants shipped to every browser, not credentials.
//
//go:embed sdk_config.json
var officialSDKConfigJSON []byte

type sdkField struct {
	Cipher  string `json:"cipher"`
	Encrypt int    `json:"is_encrypt"`
	Key     string `json:"key"`
	Name    string `json:"obfuscated_name"`
}

type sdkConfig struct {
	Protocol      int
	ConfusionInfo struct {
		Data map[string]sdkField `json:"data"`
	}
}

var officialSDKConfig, officialSDKConfigError = loadSDKConfig()

func loadSDKConfig() (sdkConfig, error) {
	var cfg sdkConfig
	err := json.Unmarshal(officialSDKConfigJSON, &cfg)
	if err == nil && (cfg.Protocol != 4 || len(cfg.ConfusionInfo.Data) == 0) {
		err = fmt.Errorf("embedded device SDK configuration is invalid")
	}
	return cfg, err
}

// sdkData reproduces the normal encode=5/compress=2 path: tn checksum,
// field renaming/DES, JSON -> gzip -> base64, then AES-CBC -> hexadecimal.
// The AES key is the first 16 ASCII characters of MD5(uid), not 16 hex bytes.
func sdkData(profile map[string]interface{}, uid string) (string, error) {
	if officialSDKConfigError != nil {
		return "", officialSDKConfigError
	}
	if uid == "" {
		return "", fmt.Errorf("device SDK uid is empty")
	}
	if _, exists := profile["tn"]; exists {
		return "", fmt.Errorf("device SDK profile already contains tn")
	}
	fields := make(map[string]interface{}, len(profile)+1)
	for key, value := range profile {
		fields[key] = value
	}
	fields["tn"] = sdkChecksum(profile)
	obfuscated, err := sdkObfuscate(fields)
	if err != nil {
		return "", err
	}
	var serialized bytes.Buffer
	encoder := json.NewEncoder(&serialized)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(obfuscated); err != nil {
		return "", err
	}
	plain := bytes.TrimSuffix(serialized.Bytes(), []byte{'\n'})
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.DefaultCompression)
	if err != nil {
		return "", err
	}
	writer.Header.OS = 3 // pako's gzip header; no timestamp or file name.
	if _, err := writer.Write(plain); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return sdkAES(base64.StdEncoding.EncodeToString(compressed.Bytes()), md5Hex(uid)[:16])
}

func sdkObfuscate(profile map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(profile))
	for name, value := range profile {
		field, configured := officialSDKConfig.ConfusionInfo.Data[name]
		if !configured {
			result[name] = value
			continue
		}
		if field.Encrypt != 0 && value != nil && sdkString(value) != "" {
			if field.Cipher != "DES" {
				return nil, fmt.Errorf("unsupported SDK cipher %s", field.Cipher)
			}
			encoded, err := sdkDES(field.Key, sdkString(value))
			if err != nil {
				return nil, err
			}
			value = encoded
		}
		result[field.Name] = value
	}
	return result, nil
}

func sdkChecksum(profile map[string]interface{}) string {
	keys := make([]string, 0, len(profile))
	for key := range profile {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var value strings.Builder
	for _, key := range keys {
		if number, ok := sdkNumber(profile[key]); ok {
			value.WriteString(sdkFloatString(number * 10000))
		} else {
			value.WriteString(sdkString(profile[key]))
		}
	}
	return md5Hex(value.String())
}

func sdkNumber(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	default:
		return 0, false
	}
}

func sdkFloatString(value float64) string {
	if value == 0 {
		return "0"
	}
	if math.Abs(value) >= 1e21 || math.Abs(value) < 1e-6 {
		s := strconv.FormatFloat(value, 'e', -1, 64)
		s = strings.Replace(s, "e-0", "e-", 1)
		return strings.Replace(s, "e+0", "e+", 1)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// String(value) semantics used by the SDK, including objects and arrays.
func sdkString(value interface{}) string {
	if value == nil {
		return "null"
	}
	if number, ok := sdkNumber(value); ok {
		return sdkFloatString(number)
	}
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case []string:
		return strings.Join(v, ",")
	case []interface{}:
		parts := make([]string, len(v))
		for i, item := range v {
			if item != nil {
				parts[i] = sdkString(item)
			}
		}
		return strings.Join(parts, ",")
	default:
		return "[object Object]"
	}
}

func sdkDES(key, text string) (string, error) {
	block, err := des.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	// The bundled JS DES packs charCodeAt (UTF-16) into 32-bit words. UTF-8
	// encoding would disagree with it for non-ASCII text, including surrogate pairs.
	units := utf16.Encode([]rune(text))
	data := make([]byte, (len(units)+7)/8*8)
	for i := 0; i < len(data); i += 4 {
		var word uint32
		for j := 0; j < 4; j++ {
			if i+j < len(units) {
				word |= uint32(units[i+j]) << (24 - 8*j)
			}
		}
		binary.BigEndian.PutUint32(data[i:], word)
	}
	for i := 0; i < len(data); i += block.BlockSize() {
		block.Encrypt(data[i:], data[i:])
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func sdkAES(text, key string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	data := []byte(text)
	// CryptoJS ZeroPadding adds no extra block to an already aligned input.
	if remainder := len(data) % aes.BlockSize; remainder != 0 {
		data = append(data, make([]byte, aes.BlockSize-remainder)...)
	}
	cipher.NewCBCEncrypter(block, []byte("0102030405060708")).CryptBlocks(data, data)
	return hex.EncodeToString(data), nil
}
