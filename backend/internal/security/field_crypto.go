package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	encryptedPrefixRoot             = "enc:"
	defaultEncryptionVersion        = "v1"
	appEnvironmentEnv               = "APP_ENV"
	productionEnvironment           = "production"
	dataEncryptionKeyEnv            = "DATA_ENCRYPTION_KEY"
	dataEncryptionKeysEnv           = "DATA_ENCRYPTION_KEYS"
	dataEncryptionCurrentVersionEnv = "DATA_ENCRYPTION_CURRENT_VERSION"
	dataEncryptionAllowLegacyAADEnv = "FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD"
	fieldCryptoAAD                  = "caiyun:field-crypto:v1"
)

type fieldCryptoConfig struct {
	enabled          bool
	currentVersion   string
	keys             map[string][]byte
	allowLegacyNoAAD bool
	err              error
}

var (
	fieldCryptoOnce sync.Once
	fieldCryptoCfg  fieldCryptoConfig
)

func IsEncryptedValue(value string) bool {
	_, _, ok := encryptedValueParts(value)
	return ok
}

// EncryptionVersion 返回密文字段的版本号；非加密值返回空字符串。
func EncryptionVersion(value string) string {
	version, _, ok := encryptedValueParts(value)
	if !ok {
		return ""
	}
	return version
}

// CurrentEncryptionVersion 返回当前配置下写入新密文时使用的版本号。
func CurrentEncryptionVersion() (string, error) {
	cfg := loadFieldCryptoConfig()
	if cfg.err != nil {
		return "", cfg.err
	}
	if !cfg.enabled {
		return "", nil
	}
	return cfg.currentVersion, nil
}

// ValidateFieldCryptoConfig validates the process-wide field-encryption
// configuration. Startup entrypoints should call this after loading the .env
// file and terminate startup when it returns an error.
//
// Production deliberately requires the versioned keyring configuration. The
// legacy DATA_ENCRYPTION_KEY variable and plaintext passthrough remain
// available outside production for backwards-compatible local development.
func ValidateFieldCryptoConfig() error {
	cfg := loadFieldCryptoConfig()
	if cfg.err != nil {
		return fmt.Errorf("字段加密配置校验失败: %w", cfg.err)
	}
	return nil
}

func EncryptString(value string) (string, error) {
	if value == "" {
		return value, nil
	}
	if IsEncryptedValue(value) {
		if _, err := DecryptString(value); err != nil {
			return "", fmt.Errorf("拒绝保存无效密文字段: %w", err)
		}
		return value, nil
	}

	cfg := loadFieldCryptoConfig()
	if cfg.err != nil {
		return "", cfg.err
	}
	if !cfg.enabled {
		return value, nil
	}

	key, ok := cfg.keys[cfg.currentVersion]
	if !ok || len(key) == 0 {
		return "", fmt.Errorf("未找到当前数据加密版本 %s 的可用密钥", cfg.currentVersion)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建 AES cipher 失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 AES-GCM 失败: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成随机 nonce 失败: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(value), []byte(fieldCryptoAAD))
	payload := append(nonce, ciphertext...)
	return encryptedPrefixForVersion(cfg.currentVersion) + base64.RawStdEncoding.EncodeToString(payload), nil
}

func DecryptString(value string) (string, error) {
	return decryptString(value, !isProductionEnvironment(), false)
}

// DecryptStringAllowPlaintext is reserved for the explicit reencrypt command,
// which must be able to inventory and rotate legacy plaintext rows. Serving
// API/Worker paths use DecryptString and reject plaintext in production.
func DecryptStringAllowPlaintext(value string) (string, error) {
	// The reencrypt command is the sole explicit migration path.  It must read
	// pre-AAD ciphertexts even after serving processes have disabled the
	// temporary compatibility switch.
	return decryptString(value, true, true)
}

func decryptString(value string, allowPlaintext, forceLegacyNoAAD bool) (string, error) {
	if value == "" {
		return value, nil
	}

	version, encodedPayload, ok := encryptedValueParts(value)
	if !ok {
		if !allowPlaintext {
			return "", fmt.Errorf("生产环境拒绝读取未加密的敏感字段；请先运行 reencrypt")
		}
		return value, nil
	}

	cfg := loadFieldCryptoConfig()
	if cfg.err != nil {
		return "", cfg.err
	}
	if !cfg.enabled {
		return "", fmt.Errorf("检测到已加密字段，但未配置 %s 或 %s", dataEncryptionKeyEnv, dataEncryptionKeysEnv)
	}

	key, keyExists := cfg.keys[version]
	if !keyExists || len(key) == 0 {
		return "", fmt.Errorf("检测到加密字段版本 %s，但未配置对应密钥", version)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建 AES cipher 失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 AES-GCM 失败: %w", err)
	}

	payload, err := base64.RawStdEncoding.DecodeString(encodedPayload)
	if err != nil {
		return "", fmt.Errorf("解码密文字段失败: %w", err)
	}
	if len(payload) < gcm.NonceSize() {
		return "", fmt.Errorf("密文字段长度不足")
	}

	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(fieldCryptoAAD))
	if err != nil && (forceLegacyNoAAD || cfg.allowLegacyNoAAD) {
		// Pre-AAD ciphertexts are accepted only during the explicit reencrypt
		// command or while the temporary cutover flag is enabled.
		plaintext, err = gcm.Open(nil, nonce, ciphertext, nil)
	}
	if err != nil {
		if !forceLegacyNoAAD && !cfg.allowLegacyNoAAD {
			return "", fmt.Errorf("解密字段失败（如为旧版无 AAD 密文，请在重加密窗口临时设置 %s=true）: %w", dataEncryptionAllowLegacyAADEnv, err)
		}
		return "", fmt.Errorf("解密字段失败: %w", err)
	}
	return string(plaintext), nil
}

func loadFieldCryptoConfig() fieldCryptoConfig {
	fieldCryptoOnce.Do(func() {
		rawSingle := strings.TrimSpace(os.Getenv(dataEncryptionKeyEnv))
		rawKeys := strings.TrimSpace(os.Getenv(dataEncryptionKeysEnv))
		rawCurrentVersion := strings.TrimSpace(os.Getenv(dataEncryptionCurrentVersionEnv))
		rawAllowLegacyNoAAD := strings.TrimSpace(os.Getenv(dataEncryptionAllowLegacyAADEnv))
		allowLegacyNoAAD := false
		if rawAllowLegacyNoAAD != "" {
			parsed, err := strconv.ParseBool(rawAllowLegacyNoAAD)
			if err != nil {
				fieldCryptoCfg.err = fmt.Errorf("%s 配置无效: %w", dataEncryptionAllowLegacyAADEnv, err)
				return
			}
			allowLegacyNoAAD = parsed
		}

		if isProductionEnvironment() {
			if rawKeys == "" {
				fieldCryptoCfg.err = fmt.Errorf("%s=%s 时必须配置 %s；%s 仅用于非生产环境兼容",
					appEnvironmentEnv, productionEnvironment, dataEncryptionKeysEnv, dataEncryptionKeyEnv)
				return
			}
			if rawCurrentVersion == "" {
				fieldCryptoCfg.err = fmt.Errorf("%s=%s 时必须配置 %s",
					appEnvironmentEnv, productionEnvironment, dataEncryptionCurrentVersionEnv)
				return
			}
		}

		if rawSingle == "" && rawKeys == "" {
			fieldCryptoCfg = fieldCryptoConfig{}
			return
		}

		keys := make(map[string][]byte)
		if rawKeys != "" {
			parsed, err := parseDataEncryptionKeys(rawKeys)
			if err != nil {
				fieldCryptoCfg.err = fmt.Errorf("%s 配置无效: %w", dataEncryptionKeysEnv, err)
				return
			}
			for version, key := range parsed {
				keys[version] = key
			}
		}
		if isProductionEnvironment() {
			requestedVersion := normalizeEncryptionVersion(rawCurrentVersion)
			if _, exists := keys[requestedVersion]; !exists {
				fieldCryptoCfg.err = fmt.Errorf("%s=%s 但未在 %s 中找到对应密钥",
					dataEncryptionCurrentVersionEnv, requestedVersion, dataEncryptionKeysEnv)
				return
			}
		}
		if rawSingle != "" {
			key, err := parseDataEncryptionKey(rawSingle)
			if err != nil {
				fieldCryptoCfg.err = fmt.Errorf("%s 配置无效: %w", dataEncryptionKeyEnv, err)
				return
			}
			if _, exists := keys[defaultEncryptionVersion]; !exists {
				keys[defaultEncryptionVersion] = key
			}
		}
		if len(keys) == 0 {
			fieldCryptoCfg = fieldCryptoConfig{}
			return
		}

		currentVersion, err := resolveCurrentEncryptionVersion(keys, rawCurrentVersion)
		if err != nil {
			fieldCryptoCfg.err = err
			return
		}
		fieldCryptoCfg = fieldCryptoConfig{
			enabled:          true,
			currentVersion:   currentVersion,
			keys:             keys,
			allowLegacyNoAAD: allowLegacyNoAAD,
		}
	})
	return fieldCryptoCfg
}

func isProductionEnvironment() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(appEnvironmentEnv)), productionEnvironment)
}

func parseDataEncryptionKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	if key, err := base64.StdEncoding.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	if key, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	if key, err := hex.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	return nil, fmt.Errorf("需要 32 字节原始字符串、Base64 或十六进制密钥")
}

func parseDataEncryptionKeys(raw string) (map[string][]byte, error) {
	normalized := strings.NewReplacer("\r", "\n", ";", "\n", ",", "\n").Replace(raw)
	entries := strings.Split(normalized, "\n")
	keys := make(map[string][]byte)
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("条目 %q 缺少 version=key 格式", entry)
		}
		version := normalizeEncryptionVersion(parts[0])
		if version == "" {
			return nil, fmt.Errorf("条目 %q 缺少有效版本号", entry)
		}
		key, err := parseDataEncryptionKey(parts[1])
		if err != nil {
			return nil, fmt.Errorf("版本 %s 的密钥无效: %w", version, err)
		}
		keys[version] = key
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("未解析到任何可用加密密钥")
	}
	return keys, nil
}

func resolveCurrentEncryptionVersion(keys map[string][]byte, rawCurrentVersion string) (string, error) {
	requested := normalizeEncryptionVersion(rawCurrentVersion)
	if requested != "" {
		if _, exists := keys[requested]; !exists {
			return "", fmt.Errorf("%s=%s 但未在 %s / %s 中找到对应密钥", dataEncryptionCurrentVersionEnv, requested, dataEncryptionKeysEnv, dataEncryptionKeyEnv)
		}
		return requested, nil
	}
	if _, exists := keys[defaultEncryptionVersion]; exists {
		return defaultEncryptionVersion, nil
	}
	versions := make([]string, 0, len(keys))
	for version := range keys {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	return versions[len(versions)-1], nil
}

func normalizeEncryptionVersion(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	if len(value) < 2 || len(value) > 32 {
		return ""
	}
	for index := 1; index < len(value); index++ {
		char := value[index]
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return ""
	}
	return value
}

func encryptedPrefixForVersion(version string) string {
	return encryptedPrefixRoot + normalizeEncryptionVersion(version) + ":"
}

func encryptedValueParts(value string) (string, string, bool) {
	if !strings.HasPrefix(value, encryptedPrefixRoot) {
		return "", "", false
	}
	rest := strings.TrimPrefix(value, encryptedPrefixRoot)
	idx := strings.Index(rest, ":")
	if idx <= 0 || idx >= len(rest)-1 {
		return "", "", false
	}
	version := normalizeEncryptionVersion(rest[:idx])
	if version == "" {
		return "", "", false
	}
	return version, rest[idx+1:], true
}

func ResetFieldCryptoForTests() {
	fieldCryptoOnce = sync.Once{}
	fieldCryptoCfg = fieldCryptoConfig{}
}
